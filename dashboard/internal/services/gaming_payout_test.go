// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dcrpulse/internal/gamingpb"
)

// payoutSeams points the gaming state at a writable directory and stages the
// address lookup and the reported state.
func payoutSeams(t *testing.T, reported *gamingpb.GameState) {
	t.Helper()
	origDir, origRequest := GamingStateDir, gamingRequest
	origAddr, origState := payoutAddress, gamingState
	GamingStateDir = t.TempDir()
	t.Cleanup(func() {
		GamingStateDir, gamingRequest = origDir, origRequest
		payoutAddress, gamingState = origAddr, origState
	})
	payoutAddress = func(context.Context, string) (string, error) { return "DsBoundAccountAddr", nil }
	gamingState = func(string) *gamingpb.GameState { return reported }
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
}

func okPayout() func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
	return func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		return &gamingpb.RespondRequest{Ok: true}, nil
	}
}

// The address comes from the bound account and is what reaches the game.
//
// A game that chose where its own winnings landed could choose itself, which is
// the same rule a reclaim's destination follows.
func TestPayoutIsDerivedFromTheBoundAccount(t *testing.T) {
	payoutSeams(t, &gamingpb.GameState{})
	payoutAddress = func(_ context.Context, game string) (string, error) {
		if game != "poker" {
			t.Errorf("the address was derived for %q, not the game being paid", game)
		}
		return "DsTheBoundOne", nil
	}
	var sent *gamingpb.SetPayoutAddress
	gamingRequest = func(_ context.Context, _ string, req *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		sent = req.GetSetPayout()
		return okPayout()(context.Background(), "", nil)
	}

	got, err := PinGamingPayout(t.Context(), "poker")
	if err != nil {
		t.Fatalf("pin: %v", err)
	}
	if got != "DsTheBoundOne" {
		t.Fatalf("the caller was told %q was pinned", got)
	}
	if sent.GetAddress() != "DsTheBoundOne" {
		t.Fatalf("the game was sent %q, not the address from the bound account", sent.GetAddress())
	}
}

// A game that already has an address is left alone.
//
// Changing it under a live table makes the seats build different settlement
// drafts and all refuse, which is a table wedged rather than paid.
func TestPayoutDoesNotChangeOneAlreadySet(t *testing.T) {
	payoutSeams(t, &gamingpb.GameState{PayoutAddress: "DsAlreadyPinned"})
	payoutAddress = func(context.Context, string) (string, error) {
		t.Error("a fresh address was derived for a game that already had one")
		return "DsSomethingElse", nil
	}
	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		t.Error("a game that already had an address was told to change it")
		return nil, nil
	}

	got, err := PinGamingPayout(t.Context(), "poker")
	if err != nil {
		t.Fatalf("pin: %v", err)
	}
	if got != "DsAlreadyPinned" {
		t.Fatalf("the caller was told %q, not the address the game already holds", got)
	}
}

// The game's refusal is what the operator is shown. It refuses an address it
// cannot decode on this network, and every claim built to pay it would fail.
func TestPayoutCarriesTheGamesRefusal(t *testing.T) {
	payoutSeams(t, &gamingpb.GameState{})
	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		return &gamingpb.RespondRequest{Ok: false, Error: "that is not an address on mainnet"}, nil
	}

	_, err := PinGamingPayout(t.Context(), "poker")
	if err == nil {
		t.Fatal("a refusal was reported as an address pinned")
	}
	if !strings.Contains(err.Error(), "not an address on mainnet") {
		t.Fatalf("the game's reason was dropped; the operator was told %q", err)
	}
}

// An unregistered game is refused, and a registered one with nothing connected
// is a different answer.
func TestPayoutNeedsARegisteredConnectedGame(t *testing.T) {
	payoutSeams(t, &gamingpb.GameState{})
	gamingRequest = okPayout()
	if _, err := PinGamingPayout(t.Context(), "chess"); !errors.Is(err, ErrGamingGameNotRegistered) {
		t.Fatalf("an unregistered game returned %v, not ErrGamingGameNotRegistered", err)
	}

	gamingRequest = nil
	if _, err := PinGamingPayout(t.Context(), "poker"); !errors.Is(err, ErrGamingGameNotConnected) {
		t.Fatalf("a game with no bridge returned %v, not ErrGamingGameNotConnected", err)
	}
}

// The automatic pin fires once and not on every report.
//
// A game reports itself every few seconds; asking the wallet for a fresh
// address each time would be a new address per report, and a game that cannot
// take one would have it asked forever.
func TestTheAutomaticPinFiresOnce(t *testing.T) {
	payoutSeams(t, &gamingpb.GameState{})
	tried.Lock()
	tried.m = map[string]bool{}
	tried.Unlock()

	var asks int
	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		asks++
		return okPayout()(context.Background(), "", nil)
	}

	for range 5 {
		PinGamingPayoutOnce("poker")
	}
	if asks != 1 {
		t.Fatalf("a game reporting five times was told where to be paid %d times", asks)
	}
}

// A game that never reports is never asked: there is nothing to answer.
func TestTheAutomaticPinSkipsAGameThatHasNotReported(t *testing.T) {
	payoutSeams(t, nil)
	tried.Lock()
	tried.m = map[string]bool{}
	tried.Unlock()

	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		t.Error("a game that has never reported was sent a payout address")
		return nil, nil
	}
	PinGamingPayoutOnce("poker")
}
