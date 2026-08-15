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
	"dcrpulse/internal/rpc"
)

// testTableGCID is a real group chat id: CreateGamingTable checks the shape
// before it does anything with an effect.
const testTableGCID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// inviteSeams points the gaming state at a writable directory and puts the
// staged calls back the way they were.
func inviteSeams(t *testing.T) {
	t.Helper()
	origDir, origRequest := GamingStateDir, gamingRequest
	origTip, origMsg := tableChainTip, tableGCMessage
	GamingStateDir = t.TempDir()
	t.Cleanup(func() {
		GamingStateDir, gamingRequest = origDir, origRequest
		tableChainTip, tableGCMessage = origTip, origMsg
	})
	tableChainTip = func(context.Context) (GamingChainTip, error) {
		return GamingChainTip{Height: 900_000, Hash: strings.Repeat("ab", 32)}, nil
	}
	tableGCMessage = func(context.Context, rpc.ShortIDHex, string, int) error { return nil }
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
}

// An invitation goes to the game as an AcceptInvite, and the seat it reports
// comes back.
func TestAcceptingAnInviteReturnsTheSeatTheGameTook(t *testing.T) {
	inviteSeams(t)

	var got *gamingpb.AcceptInvite
	var toGame string
	gamingRequest = func(_ context.Context, game string, req *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		toGame = game
		got = req.GetAcceptInvite()
		return &gamingpb.RespondRequest{
			Ok:     true,
			Result: &gamingpb.RespondRequest_AcceptInvite{AcceptInvite: &gamingpb.AcceptInviteResult{Sid: "seat-9"}},
		}, nil
	}

	sid, err := AcceptGamingInvite(t.Context(), "poker", "gaming://poker/table?sid=abc", "gc-1")
	if err != nil {
		t.Fatalf("accepting a registered game's invitation failed: %v", err)
	}
	if sid != "seat-9" {
		t.Fatalf("the caller was told the seat is %q, not what the game reported", sid)
	}
	if toGame != "poker" {
		t.Fatalf("the request went to %q, so another game would have been asked to take the seat", toGame)
	}
	if got.GetInvite() != "gaming://poker/table?sid=abc" || got.GetGcid() != "gc-1" {
		t.Fatalf("the game was handed invite %q in %q, which is not what was accepted",
			got.GetInvite(), got.GetGcid())
	}
}

// A game the operator never registered is refused before anything is asked of
// the bridge.
func TestAcceptingRefusesAnUnregisteredGame(t *testing.T) {
	inviteSeams(t)
	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		t.Error("an unregistered game was asked to take a seat")
		return nil, nil
	}

	_, err := AcceptGamingInvite(t.Context(), "chess", "gaming://chess/table?sid=abc", "gc-1")
	if !errors.Is(err, ErrGamingGameNotRegistered) {
		t.Fatalf("accepting for an unregistered game returned %v, not ErrGamingGameNotRegistered", err)
	}
}

// The game's own refusal is what the person is told: it knows why the
// invitation was not one it could act on.
func TestAcceptingCarriesTheGamesRefusal(t *testing.T) {
	inviteSeams(t)
	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		return &gamingpb.RespondRequest{Ok: false, Error: "registration closed at block 900017"}, nil
	}

	_, err := AcceptGamingInvite(t.Context(), "poker", "gaming://poker/table?sid=abc", "gc-1")
	if err == nil {
		t.Fatal("a refusal from the game was reported as a seat taken")
	}
	if !strings.Contains(err.Error(), "registration closed at block 900017") {
		t.Fatalf("the game's reason was dropped; the person was told %q", err)
	}
}

// With no listener there is nothing holding a stream, which is a different
// answer from a game that refused.
func TestAcceptingWithNoBridgeSaysSo(t *testing.T) {
	inviteSeams(t)
	gamingRequest = nil

	_, err := AcceptGamingInvite(t.Context(), "poker", "gaming://poker/table?sid=abc", "gc-1")
	if !errors.Is(err, ErrGamingGameNotConnected) {
		t.Fatalf("accepting with no bridge returned %v, not ErrGamingGameNotConnected", err)
	}
}

// The seat is taken before the invitation is posted, and a seat that could not
// be taken stops the post: an invitation nobody is at is one others pay to join
// and can never fill.
func TestCreatingSeatsBeforeItAnnounces(t *testing.T) {
	inviteSeams(t)

	var order []string
	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		order = append(order, "seat")
		return &gamingpb.RespondRequest{
			Ok:     true,
			Result: &gamingpb.RespondRequest_AcceptInvite{AcceptInvite: &gamingpb.AcceptInviteResult{Sid: "seat-1"}},
		}, nil
	}
	tableGCMessage = func(context.Context, rpc.ShortIDHex, string, int) error {
		order = append(order, "announce")
		return nil
	}

	table, err := CreateGamingTable(t.Context(), "poker", testTableGCID, 10_000_000, 2, 1)
	if err != nil {
		t.Fatalf("create a table: %v", err)
	}
	if strings.Join(order, ",") != "seat,announce" {
		t.Fatalf("the table ran %v; announcing before seating invites people to a table nobody is at", order)
	}
	if table.Until != 900_001 {
		t.Fatalf("registration closes at %d, not one block past the tip", table.Until)
	}
}

// A seat the game refused must stop the invitation being posted at all.
func TestCreatingDoesNotAnnounceATableItCouldNotJoin(t *testing.T) {
	inviteSeams(t)
	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		return &gamingpb.RespondRequest{Ok: false, Error: "no funds"}, nil
	}
	tableGCMessage = func(context.Context, rpc.ShortIDHex, string, int) error {
		t.Error("a table was announced although the creator never took a seat")
		return nil
	}

	_, err := CreateGamingTable(t.Context(), "poker", testTableGCID, 10_000_000, 2, 1)
	if err == nil {
		t.Fatal("a table was created although the creator never took a seat")
	}
	if !strings.Contains(err.Error(), "no funds") {
		t.Fatalf("the refusal that stopped the table was lost; got %q", err)
	}
}

// A send that fails leaves a seated player, and saying otherwise would send
// them looking for money that is committed.
func TestCreatingSaysYouAreSeatedWhenTheChatRefuses(t *testing.T) {
	inviteSeams(t)
	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		return &gamingpb.RespondRequest{
			Ok:     true,
			Result: &gamingpb.RespondRequest_AcceptInvite{AcceptInvite: &gamingpb.AcceptInviteResult{Sid: "seat-1"}},
		}, nil
	}
	tableGCMessage = func(context.Context, rpc.ShortIDHex, string, int) error {
		return errors.New("group chat unreachable")
	}

	table, err := CreateGamingTable(t.Context(), "poker", testTableGCID, 10_000_000, 2, 1)
	if err == nil {
		t.Fatal("an invitation that was never sent was reported as posted")
	}
	if !strings.Contains(err.Error(), "you are seated") {
		t.Fatalf("a seated player was not told their stake is committed; got %q", err)
	}
	if table.SID == "" {
		t.Fatal("the table they are seated at was not returned, so nothing can point at the seat")
	}
}
