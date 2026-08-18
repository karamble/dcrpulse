// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"strings"
	"testing"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

// The mixer, the autobuyer and a ticket purchase all spend the mixed account,
// so no two may run together. These pin the guard triangle - including the
// one subtlety that makes the naive version wrong: the purchase's own mixer
// restart runs while the purchase flag is still set.

func TestStartMixerRefusedWhileAutobuyerRuns(t *testing.T) {
	restore := setAutobuyerRunning(true)
	t.Cleanup(restore)
	err := StartMixer([]byte("x"), 1, 0, 2)
	if err == nil || !strings.Contains(err.Error(), "autobuyer") {
		t.Fatalf("mixer start not refused for the autobuyer: %v", err)
	}
}

func TestStartMixerRefusedDuringTicketPurchase(t *testing.T) {
	if !tryBeginTicketPurchase() {
		t.Fatal("could not mark a purchase active")
	}
	t.Cleanup(endTicketPurchase)
	err := StartMixer([]byte("x"), 1, 0, 2)
	if err == nil || !strings.Contains(err.Error(), "purchase") {
		t.Fatalf("mixer start not refused during a purchase: %v", err)
	}
}

func TestRestartAfterPurchaseYieldsToTheAutobuyer(t *testing.T) {
	restore := setAutobuyerRunning(true)
	t.Cleanup(restore)
	err := restartMixerAfterPurchase([]byte("x"), 1, 0, 2)
	if err == nil || !strings.Contains(err.Error(), "autobuyer") {
		t.Fatalf("restart did not yield to the autobuyer: %v", err)
	}
	if IsMixerRunning() {
		t.Fatal("the mixer started anyway")
	}
	mixerMu.Lock()
	lastErr := mixerLastErr
	mixerMu.Unlock()
	if !strings.Contains(lastErr, "autobuyer") {
		t.Fatalf("the mixer log does not say why the mixer is off: %q", lastErr)
	}
}

// TestRestartAfterPurchaseIgnoresThePurchaseFlag is the regression pin against
// the naive fix: the restart is the purchase's own last step and runs while
// the purchase flag is still set, so the flag must not block it. Passing the
// guards is proven by reaching the gRPC-client check instead of a refusal.
func TestRestartAfterPurchaseIgnoresThePurchaseFlag(t *testing.T) {
	if !tryBeginTicketPurchase() {
		t.Fatal("could not mark a purchase active")
	}
	t.Cleanup(endTicketPurchase)
	prev := rpc.AccountMixerClient
	rpc.AccountMixerClient = nil
	t.Cleanup(func() { rpc.AccountMixerClient = prev })

	err := restartMixerAfterPurchase([]byte("x"), 1, 0, 2)
	if err == nil || !strings.Contains(err.Error(), "gRPC client unavailable") {
		t.Fatalf("restart did not pass the guards during its own purchase: %v", err)
	}
}

func TestStartAutobuyerRefusedDuringTicketPurchase(t *testing.T) {
	// Fakes only satisfy the nil-client checks; the guard fires before any call.
	prevTB := rpc.TicketBuyerClient
	rpc.TicketBuyerClient = struct{ pb.TicketBuyerServiceClient }{}
	t.Cleanup(func() { rpc.TicketBuyerClient = prevTB })
	withFakeVSPWallet(t, &fakeVSPWallet{})

	if !tryBeginTicketPurchase() {
		t.Fatal("could not mark a purchase active")
	}
	t.Cleanup(endTicketPurchase)
	err := StartAutobuyer(&types.AutobuyerSettings{VspHost: "vsp.example.org", VspPubkey: "pk"}, []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "purchase") {
		t.Fatalf("autobuyer start not refused during a purchase: %v", err)
	}
}
