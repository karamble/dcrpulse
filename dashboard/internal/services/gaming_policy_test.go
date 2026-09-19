// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"errors"
	"strings"
	"testing"

	"dcrpulse/internal/types"
)

// A bridge that runs without the App Password is a bridge whose approvals mean
// nothing: the caps and the passphrase prompt are both worth exactly as much as
// the certainty that the person answering is the operator, and with the gate
// off the whole API answers to anyone who can reach the port.
//
// A bridge that runs over a dcrd with no transaction index is worth no more.
// Its approvals are real, and every payout they approve is signed and then
// never broadcast.
func TestTheBridgeCannotBeTurnedOnWithoutAHumanGate(t *testing.T) {
	bound := func() types.GamingSettings {
		return types.GamingSettings{Enabled: true}
	}

	for _, tc := range []struct {
		name       string
		in         types.GamingSettings
		appPass    bool
		txIndex    bool
		wantErr    error
		wantOn     bool
		wantReason string
	}{
		{
			name: "enabling with both preconditions met", in: bound(),
			appPass: true, txIndex: true,
			wantOn: true,
		},
		{
			name: "enabling with the gate off", in: bound(),
			appPass: false, txIndex: true,
			wantErr:    ErrGamingNeedsAppPassword,
			wantReason: "an operator told to fix it must be told what to fix",
		},
		{
			name: "enabling without the transaction index", in: bound(),
			appPass: true, txIndex: false,
			wantErr:    ErrGamingNeedsTxIndex,
			wantReason: "a payout that is signed and never sent is worse than one refused",
		},
		{
			// An unreachable dcrd reads as no index, and refusing is the
			// safe half of the guess.
			name: "enabling with neither", in: bound(),
			appPass: false, txIndex: false,
			wantErr:    ErrGamingNeedsAppPassword,
			wantReason: "the first thing to fix is named first",
		},
		{
			name: "switching off never needs either",
			in:   types.GamingSettings{Enabled: false}, appPass: false, txIndex: false,
			wantOn: false,
		},
		{
			// Routing frames needs no money, so an unfunded bridge runs
			// and refuses every spend. The refusal names the game, which
			// is the thing an operator can act on.
			name: "enabling with nothing funded",
			in: types.GamingSettings{
				Enabled: true, RegisteredGames: []string{"poker"},
			}, appPass: true, txIndex: true,
			wantOn: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := normalizeGamingSettings(tc.in, types.GamingSettings{}, tc.appPass, tc.txIndex)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got %v, want %v", err, tc.wantErr)
				}
				if out.Enabled {
					t.Fatal("a refused write must not come back enabled")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected refusal: %v", err)
			}
			if out.Enabled != tc.wantOn {
				t.Fatalf("enabled=%v, want %v", out.Enabled, tc.wantOn)
			}
		})
	}
}

// The refusal has to say what to do about it, or the bridge simply appears not
// to switch on.
func TestTheRefusalNamesTheAppPassword(t *testing.T) {
	_, err := normalizeGamingSettings(
		types.GamingSettings{Enabled: true}, types.GamingSettings{}, false, true)
	if err == nil {
		t.Fatal("enabling without the gate was allowed")
	}
	if got := err.Error(); !strings.Contains(got, "App Password") {
		t.Fatalf("the refusal does not name the App Password: %q", got)
	}
}

// Same rule for the index. "Turn on txindex" is a one-line config change and a
// restart, but only for an operator who is told that is what is wanted.
func TestTheRefusalNamesTheTransactionIndex(t *testing.T) {
	_, err := normalizeGamingSettings(
		types.GamingSettings{Enabled: true}, types.GamingSettings{}, true, false)
	if err == nil {
		t.Fatal("enabling over a node with no index was allowed")
	}
	got := err.Error()
	if !strings.Contains(got, "txindex") {
		t.Fatalf("the refusal does not name the setting: %q", got)
	}
	if !strings.Contains(got, "restart") {
		t.Fatalf("the refusal does not say a restart is needed: %q", got)
	}
}

// A refused enable must not reach the file, whichever precondition refused it.
func TestAnEnableRefusedForTheIndexIsNotStored(t *testing.T) {
	spendSeams(t)

	if _, err := WriteGamingSettings(
		types.GamingSettings{Enabled: true}, true, false); !errors.Is(err, ErrGamingNeedsTxIndex) {
		t.Fatalf("got %v, want a refusal", err)
	}
	if ReadGamingSettings().Enabled {
		t.Fatal("an enable refused for the index was stored anyway")
	}
}

// The stored policy is what a later read gets, so a refused enable must not
// reach the file at all.
func TestARefusedEnableIsNotStored(t *testing.T) {
	spendSeams(t)

	if _, err := WriteGamingSettings(
		types.GamingSettings{Enabled: true}, false, true); !errors.Is(err, ErrGamingNeedsAppPassword) {
		t.Fatalf("got %v, want a refusal", err)
	}
	if ReadGamingSettings().Enabled {
		t.Fatal("a refused enable was stored anyway")
	}

	if _, err := WriteGamingSettings(
		types.GamingSettings{Enabled: true}, true, true); err != nil {
		t.Fatalf("enabling behind the gate: %v", err)
	}
	if !ReadGamingSettings().Enabled {
		t.Fatal("an allowed enable was not stored")
	}
}
