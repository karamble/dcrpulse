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

// reclaimSeams points the gaming state at a writable directory and stages the
// address lookup, so a reclaim can be exercised without a wallet.
func reclaimSeams(t *testing.T) {
	t.Helper()
	origDir, origRequest := GamingStateDir, gamingRequest
	origAddr := reclaimAddress
	GamingStateDir = t.TempDir()
	t.Cleanup(func() {
		GamingStateDir, gamingRequest = origDir, origRequest
		reclaimAddress = origAddr
	})
	reclaimAddress = func(context.Context, string) (string, error) { return "DsBoundAccountAddr", nil }
	if _, err := WriteGamingSettings(spendPolicy(), true); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
}

func okReclaim(txid string) func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
	return func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		return &gamingpb.RespondRequest{
			Ok:     true,
			Result: &gamingpb.RespondRequest_Reclaim{Reclaim: &gamingpb.ReclaimResult{Txid: txid}},
		}, nil
	}
}

// The three kinds reach the game as the kinds the contract names, carrying the
// table they belong to.
func TestReclaimAsksForWhatWasNamed(t *testing.T) {
	for _, c := range []struct {
		kind string
		want gamingpb.Reclaim_Kind
	}{
		{"bond", gamingpb.Reclaim_BOND},
		{"stake", gamingpb.Reclaim_STAKE},
		{"tablebond", gamingpb.Reclaim_TABLE_BOND},
	} {
		reclaimSeams(t)
		var got *gamingpb.Reclaim
		gamingRequest = func(_ context.Context, _ string, req *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
			got = req.GetReclaim()
			return okReclaim("tx-"+c.kind)(context.Background(), "", nil)
		}

		txid, err := ReclaimGamingCoin(t.Context(), "poker", c.kind, "sid-1", "")
		if err != nil {
			t.Fatalf("%s: %v", c.kind, err)
		}
		if txid != "tx-"+c.kind {
			t.Fatalf("%s: the caller got txid %q, not the one the game reported", c.kind, txid)
		}
		if got.GetKind() != c.want {
			t.Fatalf("%s reached the game as %v, so the wrong lock would be swept", c.kind, got.GetKind())
		}
		if got.GetSid() != "sid-1" {
			t.Fatalf("%s: the table was not named, so the game cannot tell which seat", c.kind)
		}
	}
}

// The destination is derived from the bound account and cannot be influenced by
// the caller. A reclaim that paid where the request asked would let anything
// that reaches this route redirect the money.
func TestReclaimPaysOnlyIntoTheBoundAccount(t *testing.T) {
	reclaimSeams(t)
	reclaimAddress = func(_ context.Context, game string) (string, error) {
		if game != "poker" {
			t.Errorf("the address was derived for %q, not the game reclaiming", game)
		}
		return "DsTheBoundOne", nil
	}
	var got *gamingpb.Reclaim
	gamingRequest = func(_ context.Context, _ string, req *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		got = req.GetReclaim()
		return okReclaim("tx")(context.Background(), "", nil)
	}

	if _, err := ReclaimGamingCoin(t.Context(), "poker", "stake", "sid-1", ""); err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if got.GetDestAddr() != "DsTheBoundOne" {
		t.Fatalf("the reclaim was addressed to %q, not the bound account", got.GetDestAddr())
	}
}

// An account that cannot be resolved stops the reclaim before it is asked for:
// a reclaim with nowhere to land is refused by the game anyway, and asking
// would make an unconfigured install look like a broken game.
func TestReclaimWithNoAccountNeverAsks(t *testing.T) {
	reclaimSeams(t)
	reclaimAddress = func(context.Context, string) (string, error) {
		return "", errors.New("no account is bound")
	}
	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		t.Error("a reclaim was asked for with no account to pay it into")
		return nil, nil
	}

	if _, err := ReclaimGamingCoin(t.Context(), "poker", "bond", "", ""); err == nil {
		t.Fatal("a reclaim with no destination was accepted")
	}
}

// The game's refusal is what the operator is shown: it names how many blocks
// are left on the lock, which is the only thing that answers "when".
func TestReclaimCarriesTheGamesRefusal(t *testing.T) {
	reclaimSeams(t)
	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		return &gamingpb.RespondRequest{
			Ok:    false,
			Error: "it is not spendable for another 194 blocks",
		}, nil
	}

	_, err := ReclaimGamingCoin(t.Context(), "poker", "stake", "sid-1", "")
	if err == nil {
		t.Fatal("a refusal was reported as a reclaim")
	}
	if !strings.Contains(err.Error(), "another 194 blocks") {
		t.Fatalf("the game's reason was dropped; the operator was told %q", err)
	}
}

// A reclaim that outran the wait is NOT a failure. The game finishes it
// regardless of the deadline, so reporting failure would invite a second
// attempt at coin that is already moving.
func TestReclaimThatOutranTheWaitIsNotAFailure(t *testing.T) {
	reclaimSeams(t)
	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		return nil, context.DeadlineExceeded
	}

	_, err := ReclaimGamingCoin(t.Context(), "poker", "stake", "sid-1", "")
	if !errors.Is(err, ErrGamingReclaimInFlight) {
		t.Fatalf("a reclaim that timed out returned %v, so the operator would be told it failed "+
			"and could try to spend the same coin twice", err)
	}
}

// A kind that names no lock is refused here rather than sent on.
func TestReclaimRefusesAKindThatIsNotALock(t *testing.T) {
	reclaimSeams(t)
	gamingRequest = func(context.Context, string, *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
		t.Error("a reclaim of nothing in particular was sent to the game")
		return nil, nil
	}

	if _, err := ReclaimGamingCoin(t.Context(), "poker", "everything", "sid-1", ""); !errors.Is(err, ErrGamingReclaimKind) {
		t.Fatalf("an unknown kind returned %v, not ErrGamingReclaimKind", err)
	}
}

// An unregistered game is refused, and a registered one with nothing connected
// is a different answer.
func TestReclaimNeedsARegisteredConnectedGame(t *testing.T) {
	reclaimSeams(t)
	gamingRequest = okReclaim("tx")
	if _, err := ReclaimGamingCoin(t.Context(), "chess", "bond", "", ""); !errors.Is(err, ErrGamingGameNotRegistered) {
		t.Fatalf("an unregistered game returned %v, not ErrGamingGameNotRegistered", err)
	}

	gamingRequest = nil
	if _, err := ReclaimGamingCoin(t.Context(), "poker", "bond", "", ""); !errors.Is(err, ErrGamingGameNotConnected) {
		t.Fatalf("a game with no bridge returned %v, not ErrGamingGameNotConnected", err)
	}
}
