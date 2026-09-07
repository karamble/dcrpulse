// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dcrpulse/internal/rpc"
)

// Repointing the daemon does not stop the mixer, the autobuyer or a ticket
// purchase. Their deferred calls carry the OLD wallet's account numbers and a
// copy of its passphrase, and the gRPC clients are package vars swapped in
// place, so a defer that fires after the switch acts on the NEXT wallet. These
// pin the refusal and the wallet-identity fallback behind it.

func TestWalletSwitchGuard(t *testing.T) {
	tests := []struct {
		name      string
		mixer     bool
		autobuyer bool
		purchase  bool
		want      error
	}{
		{"idle", false, false, false, nil},
		{"mixer running", true, false, false, ErrSwitchWhileMixing},
		{"autobuyer running", false, true, false, ErrSwitchWhileMixing},

		// A purchase pauses the mixer for its whole run, so the first two
		// conditions read false for the entire window it is exposed in.
		{"ticket purchase in progress", false, false, true, ErrSwitchWhilePurchasing},

		// When both apply, report the one the user can act on.
		{"mixer wins over purchase", true, false, true, ErrSwitchWhileMixing},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer setMixerRunning(tc.mixer)()
			defer setAutobuyerRunning(tc.autobuyer)()
			if tc.purchase {
				if !tryBeginTicketPurchase() {
					t.Fatal("a purchase was already marked active")
				}
				defer endTicketPurchase()
			}

			got := walletSwitchGuard()
			if !errors.Is(got, tc.want) {
				t.Errorf("walletSwitchGuard() = %v, want %v", got, tc.want)
			}
		})
	}
}

// The guard has to refuse before any RPC or state change, or a busy wallet
// reports a daemon error instead of the real reason and has already been
// half-switched by the time it does.
func TestSwitchWalletRefusedWhileMixing(t *testing.T) {
	withHooks(t, "alpha")
	defer setMixerRunning(true)()

	err := SwitchWallet(context.Background(), "beta", "")
	if !errors.Is(err, ErrSwitchWhileMixing) {
		t.Fatalf("SwitchWallet = %v, want ErrSwitchWhileMixing", err)
	}
	if ActiveWalletName() != "alpha" {
		t.Fatalf("the active wallet moved to %q despite the refusal", ActiveWalletName())
	}
}

func TestSwitchWalletRefusedDuringPurchase(t *testing.T) {
	withHooks(t, "alpha")
	if !tryBeginTicketPurchase() {
		t.Fatal("a purchase was already marked active")
	}
	t.Cleanup(endTicketPurchase)

	if err := SwitchWallet(context.Background(), "beta", ""); !errors.Is(err, ErrSwitchWhilePurchasing) {
		t.Fatalf("SwitchWallet = %v, want ErrSwitchWhilePurchasing", err)
	}
}

func TestCloseActiveWalletRefusedDuringPurchase(t *testing.T) {
	withHooks(t, "alpha")
	if !tryBeginTicketPurchase() {
		t.Fatal("a purchase was already marked active")
	}
	t.Cleanup(endTicketPurchase)

	if err := CloseActiveWallet(context.Background()); !errors.Is(err, ErrSwitchWhilePurchasing) {
		t.Fatalf("CloseActiveWallet = %v, want ErrSwitchWhilePurchasing", err)
	}
	if ActiveWalletName() != "alpha" {
		t.Fatalf("the wallet was deselected despite the refusal: %q", ActiveWalletName())
	}
}

// The create and restore paths run the same handshake (SetActiveWallet plus a
// gRPC reconnect), so they carry the same exposure as an explicit switch.
func TestCreatePathRefusedWhileAutobuyerRuns(t *testing.T) {
	withHooks(t, "alpha")
	defer setAutobuyerRunning(true)()

	if err := switchDaemonToNewWallet(context.Background(), "beta", "mainnet"); !errors.Is(err, ErrSwitchWhileMixing) {
		t.Fatalf("switchDaemonToNewWallet = %v, want ErrSwitchWhileMixing", err)
	}
	if ActiveWalletName() != "alpha" {
		t.Fatalf("the active wallet moved to %q despite the refusal", ActiveWalletName())
	}
}

// The guard sits below the same-wallet check on purpose: re-selecting the
// wallet that is already active is a no-op the UI relies on, and turning it
// into a refusal would strand an operator whose mixer is running.
func TestSwitchToSameWalletIsNotRefusedByTheGuard(t *testing.T) {
	withHooks(t, "alpha")
	defer setMixerRunning(true)()

	err := SwitchWallet(context.Background(), "alpha", "")
	if errors.Is(err, ErrSwitchWhileMixing) || errors.Is(err, ErrSwitchWhilePurchasing) {
		t.Fatalf("re-selecting the active wallet was refused by the guard: %v", err)
	}
}

// The purchase captured wallet A's account numbers and passphrase. If the
// active wallet moved while it ran, restarting would mix accounts 1 and 2 of a
// wallet that never asked for it.
func TestRestartMixerAfterPurchaseNoOpsOnAnotherWallet(t *testing.T) {
	withHooks(t, "beta")

	err := restartMixerAfterPurchase("alpha", []byte("x"), 1, 0, 2)
	if err == nil {
		t.Fatal("the mixer restart was allowed after the wallet changed")
	}
	if !strings.Contains(err.Error(), "active wallet changed") {
		t.Fatalf("refusal does not name the reason: %v", err)
	}
	if IsMixerRunning() {
		t.Fatal("the mixer started on the wrong wallet")
	}
	mixerMu.Lock()
	lastErr := mixerLastErr
	mixerMu.Unlock()
	if !strings.Contains(lastErr, "active wallet changed") {
		t.Fatalf("the mixer log does not say why the mixer is off: %q", lastErr)
	}
}

// The regression pin: the normal case must still restart. Passing the guards is
// proven by reaching the gRPC-client check instead of a refusal.
func TestRestartMixerAfterPurchaseStillRunsOnTheSameWallet(t *testing.T) {
	withHooks(t, "alpha")
	prev := rpc.AccountMixerClient
	rpc.AccountMixerClient = nil
	t.Cleanup(func() { rpc.AccountMixerClient = prev })

	err := restartMixerAfterPurchase("alpha", []byte("x"), 1, 0, 2)
	if err == nil || !strings.Contains(err.Error(), "gRPC client unavailable") {
		t.Fatalf("restart did not pass the guards on its own wallet: %v", err)
	}
}

// A relock fires by account NUMBER, so on another wallet it would lock that
// wallet's same-numbered account, which its own mixer or autobuyer may be
// holding open. The fake records every LockAccount it receives, so this asserts
// on the RPC actually reaching the wallet rather than on a decision.
func TestRelockAccountForSkipsAnotherWallet(t *testing.T) {
	f := &fakeAutobuyerWallet{}
	prev := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = f
	t.Cleanup(func() { rpc.WalletGrpcClient = prev })

	withHooks(t, "beta")
	relockAccountFor("alpha", 2, nil)
	if len(f.locked) != 0 {
		t.Fatalf("locked accounts %v on a wallet the worker never ran on", f.locked)
	}

	// The regression pin: the same wallet must still be relocked, or every
	// worker leaves its account unlocked when it stops.
	withHooks(t, "alpha")
	relockAccountFor("alpha", 2, nil)
	if len(f.locked) != 1 || f.locked[0] != 2 {
		t.Fatalf("locked = %v, want exactly [2] on the worker's own wallet", f.locked)
	}
}
