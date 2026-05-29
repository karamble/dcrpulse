// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

const (
	autobuyerEventBufferSize = 200
	autobuyerPollInterval    = 30 * time.Second
)

var (
	autobuyerMu      sync.Mutex
	autobuyerCancel  context.CancelFunc
	autobuyerLastErr string
	autobuyerActive  *types.AutobuyerSettings

	autobuyerEventsMu sync.Mutex
	autobuyerEvents   []types.AutobuyerEvent

	autobuyerSubsMu      sync.Mutex
	autobuyerSubscribers []chan types.AutobuyerEvent
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

// AutobuyerActiveSettings returns the settings the supervisor is currently
// running with, or nil when stopped.
func AutobuyerActiveSettings() *types.AutobuyerSettings {
	autobuyerMu.Lock()
	defer autobuyerMu.Unlock()
	if autobuyerActive == nil {
		return nil
	}
	cp := *autobuyerActive
	return &cp
}

// LastAutobuyerEvents returns up to n most-recent events, oldest first.
func LastAutobuyerEvents(n int) []types.AutobuyerEvent {
	autobuyerEventsMu.Lock()
	defer autobuyerEventsMu.Unlock()
	if n <= 0 || n > len(autobuyerEvents) {
		n = len(autobuyerEvents)
	}
	out := make([]types.AutobuyerEvent, n)
	copy(out, autobuyerEvents[len(autobuyerEvents)-n:])
	return out
}

// SubscribeAutobuyerEvents returns a channel receiving every future event plus
// a cleanup func to call when the subscriber detaches.
func SubscribeAutobuyerEvents() (<-chan types.AutobuyerEvent, func()) {
	ch := make(chan types.AutobuyerEvent, 32)
	autobuyerSubsMu.Lock()
	autobuyerSubscribers = append(autobuyerSubscribers, ch)
	autobuyerSubsMu.Unlock()
	return ch, func() {
		autobuyerSubsMu.Lock()
		defer autobuyerSubsMu.Unlock()
		for i, sub := range autobuyerSubscribers {
			if sub == ch {
				autobuyerSubscribers = append(autobuyerSubscribers[:i], autobuyerSubscribers[i+1:]...)
				close(ch)
				return
			}
		}
	}
}

func recordAutobuyerEvent(level, msg string) {
	ev := types.AutobuyerEvent{Timestamp: time.Now().UTC(), Level: level, Message: msg}

	autobuyerEventsMu.Lock()
	autobuyerEvents = append(autobuyerEvents, ev)
	if len(autobuyerEvents) > autobuyerEventBufferSize {
		autobuyerEvents = autobuyerEvents[len(autobuyerEvents)-autobuyerEventBufferSize:]
	}
	autobuyerEventsMu.Unlock()

	autobuyerSubsMu.Lock()
	for _, sub := range autobuyerSubscribers {
		select {
		case sub <- ev:
		default:
		}
	}
	autobuyerSubsMu.Unlock()
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

	autobuyerMu.Lock()
	if autobuyerCancel != nil {
		autobuyerMu.Unlock()
		return fmt.Errorf("autobuyer already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	autobuyerCancel = cancel
	autobuyerLastErr = ""
	sCopy := *settings
	autobuyerActive = &sCopy
	autobuyerMu.Unlock()

	// Remember the VSP for the picker, matching Decrediton's
	// dispatch(updateUsedVSPs(vsp)) in ControlActions.js:519.
	rememberVSPUsed(ctx, sCopy.VspHost, sCopy.VspPubkey)

	// Copy the passphrase: the HTTP handler zeroes its slice when it returns
	// (right after this call), but the autobuyer goroutine uses the passphrase
	// later (after StopMixer/WaitForMixerStop), so it must own its own copy.
	pp := append([]byte(nil), passphrase...)
	go runAutobuyer(ctx, sCopy, pp)
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

func runAutobuyer(ctx context.Context, settings types.AutobuyerSettings, passphrase []byte) {
	// This goroutine owns its passphrase copy; wipe it when it exits.
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()
	defer func() {
		autobuyerMu.Lock()
		autobuyerCancel = nil
		autobuyerActive = nil
		autobuyerMu.Unlock()
	}()

	recordAutobuyerEvent("info", fmt.Sprintf("Autobuyer starting (account=%d vsp=%s balanceToMaintain=%.8f DCR)",
		settings.Account, settings.VspHost, settings.BalanceToMaintain))

	// When privacy is configured, the autobuyer buys mixed tickets: fund + split
	// + mix from the "mixed" account, change to the "unmixed" account, mixing on.
	// Otherwise buy plainly from the configured account. Mirrors Decrediton's
	// startTicketAutoBuyer branch.
	sourceAccount := settings.Account
	mixing, mixed := TicketMixingParams(ctx)
	if mixed {
		sourceAccount = mixing.Mixed
		// The autobuyer mixes inline while it runs, so the standalone continuous
		// mixer must not run alongside it (both spend the mixed account). Stop it
		// if running; the user can restart it from the privacy tab after stopping
		// the autobuyer.
		if IsMixerRunning() {
			StopMixer()
			WaitForMixerStop(5 * time.Second)
			recordAutobuyerEvent("info", "Stopped the account mixer; the autobuyer mixes tickets while it runs")
		}
	}

	// Make the source account usable for signing. Skips the unlock if it's
	// already unlocked (e.g. by a prior mix session) to avoid dcrwallet's
	// already-unlocked "invalid passphrase" hash check; migrates to per-account
	// encryption if needed.
	unlockCtx, unlockCancel := context.WithTimeout(ctx, 10*time.Second)
	err := unlockAccountForSpend(unlockCtx, sourceAccount, passphrase)
	unlockCancel()
	if err != nil {
		setAutobuyerErr(fmt.Sprintf("Unlock source account failed: %v", err))
		return
	}
	defer func() {
		lockCtx, lockCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer lockCancel()
		_, _ = rpc.WalletGrpcClient.LockAccount(lockCtx, &pb.LockAccountRequest{AccountNumber: sourceAccount})
	}()

	// With mixing on, the ticket buyer also runs a per-block account mixer on the
	// change account (dcrwallet sets MixChange when EnableMixing is set), spending
	// the unmixed account to mix it into the mixed account. That account is
	// per-account-encrypted, and the buyer's wallet-wide Unlock does not reach
	// per-account-encrypted accounts, so it must be unlocked explicitly like the
	// standalone mixer does. Without this, dcrwallet logs "TKBY: Account mixing
	// failed: ... account with unique passphrase is locked" every block and the
	// unmixed balance never mixes.
	if mixed && mixing.Change != sourceAccount {
		changeCtx, changeCancel := context.WithTimeout(ctx, 10*time.Second)
		err := unlockAccountForSpend(changeCtx, mixing.Change, passphrase)
		changeCancel()
		if err != nil {
			setAutobuyerErr(fmt.Sprintf("Unlock change account failed: %v", err))
			return
		}
		defer func() {
			lockCtx, lockCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer lockCancel()
			_, _ = rpc.WalletGrpcClient.LockAccount(lockCtx, &pb.LockAccountRequest{AccountNumber: mixing.Change})
		}()
	}

	balanceAtoms := int64(settings.BalanceToMaintain * 1e8)
	req := &pb.RunTicketBuyerRequest{
		Passphrase:        passphrase,
		Account:           sourceAccount,
		VotingAccount:     sourceAccount,
		BalanceToMaintain: balanceAtoms,
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

	// Remember the VSP in the shared used_vsps list (mirrors Decrediton's
	// updateUsedVSPs on autobuyer start), so it appears in the picker's
	// registry-disabled fallback like manually-purchased VSPs do.
	rememberVSPUsed(ctx, settings.VspHost, settings.VspPubkey)

	// Ticket-poller: every autobuyerPollInterval, compare the wallet's ticket
	// hash set to the previous snapshot and emit "purchased" events for diffs.
	// RunTicketBuyerResponse is empty in v4, so this is how we surface activity.
	pollDone := make(chan struct{})
	go pollAutobuyerTickets(ctx, pollDone)

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

	<-pollDone
}

func setAutobuyerErr(msg string) {
	log.Printf("autobuyer: %s", msg)
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
				log.Printf("autobuyer poll: %v", err)
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
