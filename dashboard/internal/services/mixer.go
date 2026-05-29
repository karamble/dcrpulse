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
	mixerMu          sync.Mutex
	mixerCancel      context.CancelFunc
	mixerLastErr     string
	mixerEvents      []MixerEvent
	mixerSubsMu      sync.Mutex
	mixerSubscribers []chan MixerEvent
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
	mixerMu.Lock()
	defer mixerMu.Unlock()
	if n <= 0 || n > len(mixerEvents) {
		n = len(mixerEvents)
	}
	out := make([]MixerEvent, n)
	copy(out, mixerEvents[len(mixerEvents)-n:])
	return out
}

// SubscribeMixerEvents returns a channel that receives every future mixer
// event plus a cleanup func to call when the subscriber goes away.
func SubscribeMixerEvents() (<-chan MixerEvent, func()) {
	ch := make(chan MixerEvent, 32)
	mixerSubsMu.Lock()
	mixerSubscribers = append(mixerSubscribers, ch)
	mixerSubsMu.Unlock()
	return ch, func() {
		mixerSubsMu.Lock()
		defer mixerSubsMu.Unlock()
		for i, sub := range mixerSubscribers {
			if sub == ch {
				mixerSubscribers = append(mixerSubscribers[:i], mixerSubscribers[i+1:]...)
				close(ch)
				return
			}
		}
	}
}

func recordMixerEvent(level, msg string) {
	ev := MixerEvent{Timestamp: time.Now().UTC(), Level: level, Message: msg}

	mixerMu.Lock()
	mixerEvents = append(mixerEvents, ev)
	if len(mixerEvents) > mixerEventBufferSize {
		mixerEvents = mixerEvents[len(mixerEvents)-mixerEventBufferSize:]
	}
	mixerMu.Unlock()

	mixerSubsMu.Lock()
	for _, sub := range mixerSubscribers {
		select {
		case sub <- ev:
		default:
		}
	}
	mixerSubsMu.Unlock()
}

// StartMixer launches the P2P mixer goroutine. Returns an error if it's
// already running or if the gRPC client isn't wired. The passphrase byte slice
// is owned by this function for the duration of the call.
func StartMixer(passphrase []byte, mixedAccount, mixedBranch, changeAccount uint32) error {
	if rpc.AccountMixerClient == nil {
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

	go runMixer(ctx, passphrase, mixedAccount, mixedBranch, changeAccount)
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

func runMixer(ctx context.Context, passphrase []byte, mixedAccount, mixedBranch, changeAccount uint32) {
	defer func() {
		mixerMu.Lock()
		mixerCancel = nil
		mixerMu.Unlock()
	}()

	recordMixerEvent("info", fmt.Sprintf("Mixer starting (mixed=%d branch=%d change=%d)", mixedAccount, mixedBranch, changeAccount))

	// Mixing spends outputs from the change account, so it must be unlocked
	// for the lifetime of the mixer. dcrwallet's wallet-wide Unlock (driven
	// by the passphrase in RunAccountMixerRequest) does NOT unlock
	// per-account-encrypted accounts. Mirrors Decrediton's
	// unlockAcctAndExecFn(changeAccount, leaveUnlock=true).
	unlockCtx, unlockCancel := context.WithTimeout(ctx, 10*time.Second)
	_, err := rpc.WalletGrpcClient.UnlockAccount(unlockCtx, &pb.UnlockAccountRequest{
		Passphrase:    passphrase,
		AccountNumber: changeAccount,
	})
	unlockCancel()
	if err != nil {
		msg := fmt.Sprintf("Unlock change account failed: %v", err)
		log.Printf("❌ %s", msg)
		mixerMu.Lock()
		mixerLastErr = err.Error()
		mixerMu.Unlock()
		recordMixerEvent("error", msg)
		return
	}
	// Relock the change account when the mixer stops.
	defer func() {
		lockCtx, lockCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer lockCancel()
		_, _ = rpc.WalletGrpcClient.LockAccount(lockCtx, &pb.LockAccountRequest{AccountNumber: changeAccount})
	}()

	// v5 dropped the CsppServer field. Mixing is implicitly enabled when
	// the daemon is launched with --mixing and RunAccountMixer is invoked
	// with a valid mixed/change account pair.
	req := &pb.RunAccountMixerRequest{
		Passphrase:         passphrase,
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
