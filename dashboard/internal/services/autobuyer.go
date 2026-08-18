// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/dcrutil/v4"
)

const (
	autobuyerEventBufferSize = 200
	autobuyerPollInterval    = 30 * time.Second
)

var (
	autobuyerMu      sync.Mutex
	autobuyerCancel  context.CancelFunc
	autobuyerLastErr string

	autobuyerLog = eventRing[types.AutobuyerEvent]{max: autobuyerEventBufferSize}
	autobuyerBus eventBus[types.AutobuyerEvent]
)

// IsAutobuyerRunning reports whether the supervisor currently owns a stream.
func IsAutobuyerRunning() bool {
	autobuyerMu.Lock()
	defer autobuyerMu.Unlock()
	return autobuyerCancel != nil
}

// LastAutobuyerError returns the most recent terminal error, or "".
func LastAutobuyerError() string {
	autobuyerMu.Lock()
	defer autobuyerMu.Unlock()
	return autobuyerLastErr
}

// LastAutobuyerEvents returns up to n most-recent events, oldest first.
func LastAutobuyerEvents(n int) []types.AutobuyerEvent {
	return autobuyerLog.last(n)
}

// SubscribeAutobuyerEvents returns a channel receiving every future event plus
// a cleanup func to call when the subscriber detaches.
func SubscribeAutobuyerEvents() (<-chan types.AutobuyerEvent, func()) {
	return autobuyerBus.subscribe(32)
}

func recordAutobuyerEvent(level, msg string) {
	ev := types.AutobuyerEvent{Timestamp: time.Now().UTC(), Level: level, Message: msg}

	autobuyerLog.add(ev)
	autobuyerBus.publish(ev)
}

// StartAutobuyer launches the ticket-autobuyer goroutine.
func StartAutobuyer(settings *types.AutobuyerSettings, passphrase []byte) error {
	if rpc.TicketBuyerClient == nil || rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet gRPC clients unavailable")
	}
	if settings == nil {
		return fmt.Errorf("settings required")
	}
	if settings.VspHost == "" || settings.VspPubkey == "" {
		return fmt.Errorf("vspHost and vspPubkey are required")
	}
	if settings.BalanceToMaintain < 0 {
		return fmt.Errorf("balanceToMaintain must be >= 0")
	}
	// The autobuyer mixes inline while it runs, so the standalone continuous
	// mixer must be stopped first (both spend the mixed account).
	if IsMixerRunning() {
		return fmt.Errorf("stop the account mixer before starting the ticket autobuyer")
	}
	// A purchase may have paused the mixer and will restart it as its last
	// step, still under its own flag - starting the autobuyer in that window
	// would put both on the mixed account the moment the restart lands.
	if IsTicketPurchaseInProgress() {
		return fmt.Errorf("a ticket purchase is in progress; wait for it to finish before starting the ticket autobuyer")
	}

	autobuyerMu.Lock()
	if autobuyerCancel != nil {
		autobuyerMu.Unlock()
		return fmt.Errorf("autobuyer already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	autobuyerCancel = cancel
	autobuyerLastErr = ""
	sCopy := *settings
	autobuyerMu.Unlock()

	// Remember the VSP for the picker, matching Decrediton's
	// dispatch(updateUsedVSPs(vsp)) in ControlActions.js:519.
	rememberVSPUsed(ctx, sCopy.VspHost, sCopy.VspPubkey)

	// When privacy is configured, the autobuyer buys mixed tickets: fund + split
	// + mix from the "mixed" account, change to the "unmixed" account, mixing on.
	// Otherwise buy plainly from the configured account. Mirrors Decrediton's
	// startTicketAutoBuyer branch.
	sourceAccount := sCopy.Account
	mixing, mixed := TicketMixingParams(ctx)
	if mixed {
		sourceAccount = mixing.Mixed
	}

	// Unlock before the goroutine starts so a wrong passphrase reaches the
	// caller instead of surfacing later as an autobuyer event. The accounts stay
	// unlocked for the buyer's lifetime and are re-locked when it stops.
	abort := func(err error, what string) error {
		cancel()
		autobuyerMu.Lock()
		autobuyerCancel = nil
		autobuyerLastErr = err.Error()
		autobuyerMu.Unlock()
		return fmt.Errorf("%s: %w", what, err)
	}
	unlockCtx, unlockCancel := context.WithTimeout(ctx, 10*time.Second)
	didUnlockSource, err := unlockAccountForSpend(unlockCtx, sourceAccount, passphrase)
	unlockCancel()
	if err != nil {
		return abort(err, "unlock source account")
	}
	// With mixing on, the ticket buyer also runs a per-block account mixer on the
	// change account (dcrwallet sets MixChange when EnableMixing is set), spending
	// the unmixed account to mix it into the mixed account. That account is
	// per-account-encrypted, and the buyer's wallet-wide Unlock does not reach
	// per-account-encrypted accounts, so it must be unlocked explicitly like the
	// standalone mixer does. Without this, dcrwallet logs "TKBY: Account mixing
	// failed: ... account with unique passphrase is locked" every block and the
	// unmixed balance never mixes.
	var didUnlockChange bool
	if mixed && mixing.Change != sourceAccount {
		changeCtx, changeCancel := context.WithTimeout(ctx, 10*time.Second)
		didUnlockChange, err = unlockAccountForSpend(changeCtx, mixing.Change, passphrase)
		changeCancel()
		if err != nil {
			if didUnlockSource {
				relockAccount(sourceAccount, setAutobuyerErr)
			}
			return abort(err, "unlock change account")
		}
	}

	// The passphrase is not needed past this point: the buyer signs with the
	// account keys unlocked above and its RPC carries no passphrase.
	go runAutobuyer(ctx, sCopy, sourceAccount, mixing, mixed, didUnlockSource, didUnlockChange)
	return nil
}

// StopAutobuyer cancels the supervisor. Safe to call when not running.
func StopAutobuyer() {
	autobuyerMu.Lock()
	cancel := autobuyerCancel
	autobuyerMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// AutobuyerStatusSnapshot returns the current status for the status handler.
func AutobuyerStatusSnapshot(ctx context.Context) types.AutobuyerStatus {
	settings, _ := LoadAutobuyerSettings(ctx)
	return types.AutobuyerStatus{
		Running:   IsAutobuyerRunning(),
		LastError: LastAutobuyerError(),
		Settings:  settings,
	}
}

func runAutobuyer(ctx context.Context, settings types.AutobuyerSettings, sourceAccount uint32, mixing TicketMixing, mixed, didUnlockSource, didUnlockChange bool) {
	defer func() {
		autobuyerMu.Lock()
		autobuyerCancel = nil
		autobuyerMu.Unlock()
	}()
	// Re-lock whatever StartAutobuyer opened once the buyer stops.
	if didUnlockSource {
		defer relockAccount(sourceAccount, setAutobuyerErr)
	}
	if didUnlockChange {
		defer relockAccount(mixing.Change, setAutobuyerErr)
	}

	recordAutobuyerEvent("info", fmt.Sprintf("Autobuyer starting (account=%d vsp=%s balanceToMaintain=%.8f DCR)",
		settings.Account, settings.VspHost, settings.BalanceToMaintain))

	balanceAtoms, err := dcrutil.NewAmount(settings.BalanceToMaintain)
	if err != nil {
		msg := fmt.Sprintf("invalid balance to maintain %v: %v", settings.BalanceToMaintain, err)
		recordAutobuyerEvent("error", msg)
		setAutobuyerErr(msg)
		return
	}
	// Do not set Passphrase. A non-empty passphrase makes dcrwallet unlock the
	// whole wallet (the RunTicketBuyer handler and ticketbuyer.Run/buy), and the
	// ticketbuyer's own unlock uses a nil lock channel that is never relocked
	// when the buyer stops, leaving the wallet spendable until the next relaunch.
	// The accounts the buyer signs with are already unlocked per-account above
	// (relocked on stop). Mirrors Decrediton's startTicketAutoBuyer, which omits
	// the passphrase from this request.
	req := &pb.RunTicketBuyerRequest{
		Account:           sourceAccount,
		VotingAccount:     sourceAccount,
		BalanceToMaintain: int64(balanceAtoms),
		VspHost:           "https://" + strings.TrimPrefix(strings.TrimPrefix(settings.VspHost, "https://"), "http://"),
		VspPubkey:         settings.VspPubkey,
		Limit:             1,
	}
	if mixed {
		req.EnableMixing = true
		req.MixedAccount = mixing.Mixed
		req.MixedSplitAccount = mixing.Mixed
		req.ChangeAccount = mixing.Change
		req.MixedAccountBranch = privacyMixedAccountBranch
	}

	stream, err := rpc.TicketBuyerClient.RunTicketBuyer(ctx, req)
	if err != nil {
		setAutobuyerErr(fmt.Sprintf("RunTicketBuyer call failed: %v", err))
		return
	}

	recordAutobuyerEvent("info", "Autobuyer connected; waiting for purchase opportunities")

	// Ticket-poller: every autobuyerPollInterval, compare the wallet's ticket
	// hash set to the previous snapshot and emit "purchased" events for diffs.
	// RunTicketBuyerResponse is empty in v4, so this is how we surface activity.
	// The poller only returns when its context ends; run it on a child context
	// cancelled on Recv-loop exit so <-pollDone below never blocks forever.
	pollCtx, pollCancel := context.WithCancel(ctx)
	defer pollCancel()
	pollDone := make(chan struct{})
	go pollAutobuyerTickets(pollCtx, pollDone)

	// Stream Recv loop. Empty responses are expected; we only react to errors.
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			recordAutobuyerEvent("info", "Autobuyer stream closed by daemon")
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				recordAutobuyerEvent("info", "Autobuyer stopped")
				break
			}
			setAutobuyerErr(fmt.Sprintf("Autobuyer stream error: %v", err))
			break
		}
	}

	pollCancel()
	<-pollDone
}

func setAutobuyerErr(msg string) {
	stkeLog.Infof("autobuyer: %s", msg)
	autobuyerMu.Lock()
	autobuyerLastErr = msg
	autobuyerMu.Unlock()
	recordAutobuyerEvent("error", msg)
}

// pollAutobuyerTickets emits an event for each new ticket purchase tx the
// wallet observes while the autobuyer is running.
func pollAutobuyerTickets(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	seen := make(map[string]struct{})
	primed := false

	tick := func() {
		listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		tickets, err := ListTickets(listCtx)
		if err != nil {
			if ctx.Err() == nil {
				stkeLog.Warnf("autobuyer poll: %v", err)
			}
			return
		}
		next := make(map[string]struct{}, len(tickets))
		for _, t := range tickets {
			next[t.Hash] = struct{}{}
			if !primed {
				continue
			}
			if _, ok := seen[t.Hash]; ok {
				continue
			}
			height := "unmined"
			if t.BlockHeight > 0 {
				height = fmt.Sprintf("%d", t.BlockHeight)
			}
			recordAutobuyerEvent("info", fmt.Sprintf("Autobuyer purchased ticket %s (height %s)", t.Hash, height))
		}
		seen = next
		primed = true
	}

	// Prime immediately so the first new purchase produces an event.
	tick()

	ticker := time.NewTicker(autobuyerPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}
