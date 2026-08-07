// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"dcrpulse/internal/rpc"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

var mixerDebugEnabled atomic.Bool

// MixerDebugEnabled reports whether MIXC + TKBY debug logging is currently on.
func MixerDebugEnabled() bool {
	return mixerDebugEnabled.Load()
}

// SetMixerDebug calls dcrwallet's debuglevel JSON-RPC to toggle MIXC + TKBY
// between debug and info, and tracks the resulting state locally.
func SetMixerDebug(ctx context.Context, enabled bool) error {
	if rpc.WalletClient == nil {
		return fmt.Errorf("wallet client not initialized")
	}
	levelSpec := "MIXC=info,TKBY=info"
	if enabled {
		levelSpec = "MIXC=debug,TKBY=debug"
	}
	raw, _ := json.Marshal(levelSpec)
	if _, err := rpc.WalletClient.RawRequest(ctx, "debuglevel", []json.RawMessage{raw}); err != nil {
		return fmt.Errorf("debuglevel RPC: %w", err)
	}
	mixerDebugEnabled.Store(enabled)
	return nil
}

// MixerEvent is a structured log entry emitted by the privacy mixer goroutine.
type MixerEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

const mixerEventBufferSize = 200

var (
	mixerMu      sync.Mutex
	mixerCancel  context.CancelFunc
	mixerLastErr string
	mixerLog     = eventRing[MixerEvent]{max: mixerEventBufferSize}
	mixerBus     eventBus[MixerEvent]
)

// IsMixerRunning reports whether the mixer goroutine currently holds a stream.
func IsMixerRunning() bool {
	mixerMu.Lock()
	defer mixerMu.Unlock()
	return mixerCancel != nil
}

// WaitForMixerStop blocks until the mixer goroutine has fully stopped (its stop
// path relocks the change account) or the timeout elapses. Call after StopMixer
// before an operation that needs exclusive use of the mixed account.
func WaitForMixerStop(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for IsMixerRunning() {
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// LastMixerError returns the most recent terminal error from the mixer, or "".
func LastMixerError() string {
	mixerMu.Lock()
	defer mixerMu.Unlock()
	return mixerLastErr
}

// LastMixerEvents returns up to n most-recent events, oldest first.
func LastMixerEvents(n int) []MixerEvent {
	return mixerLog.last(n)
}

// SubscribeMixerEvents returns a channel that receives every future mixer
// event plus a cleanup func to call when the subscriber goes away.
func SubscribeMixerEvents() (<-chan MixerEvent, func()) {
	return mixerBus.subscribe(32)
}

func recordMixerEvent(level, msg string) {
	ev := MixerEvent{Timestamp: time.Now().UTC(), Level: level, Message: msg}
	mixerLog.add(ev)
	mixerBus.publish(ev)
}

func setMixerErr(msg string) {
	log.Printf("mixer: %s", msg)
	mixerMu.Lock()
	mixerLastErr = msg
	mixerMu.Unlock()
	recordMixerEvent("error", msg)
}

// StartMixer launches the P2P mixer goroutine. Returns an error if it's
// already running or if the gRPC client isn't wired. The passphrase byte slice
// is owned by this function for the duration of the call.
func StartMixer(passphrase []byte, mixedAccount, mixedBranch, changeAccount uint32) error {
	if rpc.AccountMixerClient == nil || rpc.WalletGrpcClient == nil {
		return fmt.Errorf("mixer gRPC client unavailable")
	}

	mixerMu.Lock()
	if mixerCancel != nil {
		mixerMu.Unlock()
		return fmt.Errorf("mixer already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	mixerCancel = cancel
	mixerLastErr = ""
	mixerMu.Unlock()

	// Unlock before the goroutine starts so a wrong passphrase is reported to
	// the caller instead of surfacing later as a mixer event. Mixing spends
	// outputs from the change account, so it stays unlocked for the lifetime of
	// the run; dcrwallet's wallet-wide Unlock (driven by the passphrase in
	// RunAccountMixerRequest) does NOT unlock per-account-encrypted accounts.
	// Mirrors Decrediton's unlockAcctAndExecFn(changeAccount, leaveUnlock=true).
	unlockCtx, unlockCancel := context.WithTimeout(ctx, 10*time.Second)
	_, err := rpc.WalletGrpcClient.UnlockAccount(unlockCtx, &pb.UnlockAccountRequest{
		Passphrase:    passphrase,
		AccountNumber: changeAccount,
	})
	unlockCancel()
	if err != nil {
		cancel()
		mixerMu.Lock()
		mixerCancel = nil
		mixerLastErr = err.Error()
		mixerMu.Unlock()
		return fmt.Errorf("unlock change account: %w", err)
	}

	// The passphrase is not needed past this point: the mixer signs with the
	// account key unlocked above and the RPC below carries no passphrase.
	go runMixer(ctx, mixedAccount, mixedBranch, changeAccount)
	return nil
}

// StopMixer cancels the running mixer goroutine. Safe to call when not
// running — no-op.
func StopMixer() {
	mixerMu.Lock()
	cancel := mixerCancel
	mixerMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func runMixer(ctx context.Context, mixedAccount, mixedBranch, changeAccount uint32) {
	defer func() {
		mixerMu.Lock()
		mixerCancel = nil
		mixerMu.Unlock()
	}()
	// Relock the change account StartMixer unlocked when the mixer stops.
	defer relockAccount(changeAccount, setMixerErr)

	recordMixerEvent("info", fmt.Sprintf("Mixer starting (mixed=%d branch=%d change=%d)", mixedAccount, mixedBranch, changeAccount))

	// Don't pass the passphrase: dcrwallet's RunAccountMixer would otherwise do a
	// wallet-wide Unlock, unlocking every account for the mixer's lifetime. We
	// already unlocked the change account per-account above (relocked on stop),
	// which is all the mixer needs to sign. Mirrors Decrediton, which unlocks only
	// the change account and omits the passphrase from this RPC. v5 also dropped
	// the CsppServer field; mixing is implicit with --mixing + a valid account pair.
	req := &pb.RunAccountMixerRequest{
		MixedAccount:       mixedAccount,
		MixedAccountBranch: mixedBranch,
		ChangeAccount:      changeAccount,
	}

	stream, err := rpc.AccountMixerClient.RunAccountMixer(ctx, req)
	if err != nil {
		msg := fmt.Sprintf("RunAccountMixer call failed: %v", err)
		log.Printf("❌ %s", msg)
		mixerMu.Lock()
		mixerLastErr = err.Error()
		mixerMu.Unlock()
		recordMixerEvent("error", msg)
		return
	}

	recordMixerEvent("info", "Mixer connected; awaiting peers")

	for {
		_, err := stream.Recv()
		if err == io.EOF {
			recordMixerEvent("info", "Mixer stream closed by daemon")
			return
		}
		if err != nil {
			if ctx.Err() != nil {
				recordMixerEvent("info", "Mixer stopped")
				return
			}
			mixerMu.Lock()
			mixerLastErr = err.Error()
			mixerMu.Unlock()
			recordMixerEvent("error", fmt.Sprintf("Mixer stream error: %v", err))
			return
		}
		recordMixerEvent("info", "Mix cycle event")
	}
}
