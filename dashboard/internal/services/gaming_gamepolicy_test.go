// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"errors"
	"math"
	"strings"
	"testing"

	"dcrpulse/internal/types"
)

// A policy has to survive an unrelated settings write.
//
// Saving one game's account would otherwise reset every other game's caps to
// the defaults, and a cap nobody chose is a cap nobody reviewed. It is the same
// property a token has, for the same reason.
func TestCarryGamePolicies(t *testing.T) {
	poker := types.GamePolicy{
		Name: "Poker Night", Account: "poker",
		PerTableCapAtoms: 1, PerDayCapAtoms: 2, ApprovalTimeoutSecs: 30,
	}
	chess := types.GamePolicy{Account: "chess", PerTableCapAtoms: 7, PerDayCapAtoms: 8, ApprovalTimeoutSecs: 60}
	stored := map[string]types.GamePolicy{"poker": poker, "chess": chess}

	t.Run("a game just registered gets the defaults", func(t *testing.T) {
		out := carryGamePolicies(nil, nil, []string{"poker"})
		if out["poker"] != defaultGamePolicy() {
			t.Fatalf("a new game got %+v, want the defaults", out["poker"])
		}
		// And no money, so it can do nothing until the operator says which.
		if out["poker"].Account != "" {
			t.Errorf("a new game was born funded from %q", out["poker"].Account)
		}
	})

	t.Run("an unnamed game keeps what was stored", func(t *testing.T) {
		out := carryGamePolicies(stored, map[string]types.GamePolicy{
			"poker": {Name: "Poker Night", Account: "elsewhere", PerTableCapAtoms: 1, PerDayCapAtoms: 2, ApprovalTimeoutSecs: 30},
		}, []string{"poker", "chess"})
		if out["chess"] != chess {
			t.Fatalf("editing poker changed chess to %+v, want %+v", out["chess"], chess)
		}
	})

	t.Run("a named game takes the operator's edit", func(t *testing.T) {
		out := carryGamePolicies(stored, map[string]types.GamePolicy{
			"poker": {Account: "moved", PerTableCapAtoms: 5, PerDayCapAtoms: 9, ApprovalTimeoutSecs: 45},
		}, []string{"poker"})
		if out["poker"].Account != "moved" || out["poker"].PerTableCapAtoms != 5 {
			t.Fatalf("the edit was ignored: %+v", out["poker"])
		}
	})

	t.Run("removing a game drops its policy", func(t *testing.T) {
		out := carryGamePolicies(stored, nil, []string{"chess"})
		if _, still := out["poker"]; still {
			t.Fatal("a removed game kept its policy")
		}
	})

	t.Run("a policy cannot register a game", func(t *testing.T) {
		out := carryGamePolicies(nil, map[string]types.GamePolicy{
			"chess": {Account: "chess"},
		}, []string{"poker"})
		if _, there := out["chess"]; there {
			t.Fatal("a policy created an entry for a game nobody registered")
		}
	})
}

// A cap above the whole supply is either a typo or an overflow on its way to
// happening; either way the honest reading is the largest cap that means
// anything. The zero sentinel has to survive the clamp, or "no limit" would
// quietly become the tightest limit of all.
func TestACapBeyondTheSupplyIsClampedToIt(t *testing.T) {
	out := normalizeGamePolicy(types.GamePolicy{
		PerTableCapAtoms: math.MaxInt64, PerDayCapAtoms: math.MaxInt64, ApprovalTimeoutSecs: 120,
	})
	if out.PerTableCapAtoms != maxSpendAtoms || out.PerDayCapAtoms != maxSpendAtoms {
		t.Fatalf("caps clamped to %d and %d, want the supply %d",
			out.PerTableCapAtoms, out.PerDayCapAtoms, maxSpendAtoms)
	}

	out = normalizeGamePolicy(types.GamePolicy{ApprovalTimeoutSecs: 120})
	if out.PerTableCapAtoms != 0 || out.PerDayCapAtoms != 0 {
		t.Fatalf("the no-limit sentinel did not survive the clamp: %d, %d",
			out.PerTableCapAtoms, out.PerDayCapAtoms)
	}
}

// Which account a game's money comes from is the most consequential answer in
// the spend path.
//
// Resolved from anything global, a spend one game asked for under its own caps
// would be paid out of an account it was never allowed to touch.
func TestTheAccountAGameSpendsFromIsItsOwn(t *testing.T) {
	s := types.GamingSettings{
		Enabled:         true,
		RegisteredGames: []string{"poker", "chess"},
		Policies: map[string]types.GamePolicy{
			"poker": {Account: "poker-money"},
			"chess": {Account: "chess-money"},
			// Registered, funded by nobody.
			"backgammon": {Account: "  "},
		},
	}

	for game, want := range map[string]string{"poker": "poker-money", "chess": "chess-money"} {
		got, err := gamingAccountFor(s, game)
		if err != nil {
			t.Errorf("%s: %v", game, err)
			continue
		}
		if got != want {
			t.Errorf("%s would be paid from %q, want %q", game, got, want)
		}
	}

	if _, err := gamingAccountFor(s, "backgammon"); err == nil {
		t.Error("a game with no account bound resolved one anyway")
	} else if !strings.Contains(err.Error(), "backgammon") {
		t.Errorf("the refusal does not name the game: %v", err)
	}

	if _, err := gamingAccountFor(s, "nobody"); err == nil {
		t.Error("an unregistered game resolved an account")
	}
}

// The bridge no longer switches itself off for want of an account: routing
// frames costs nothing. What replaces that is per game - the bridge runs, the
// traffic flows, and the unfunded game is refused where money moves, with a
// message naming it.
func TestAGameWithNoAccountBoundCanStakeNothing(t *testing.T) {
	s, err := normalizeGamingSettings(types.GamingSettings{
		Enabled: true, RegisteredGames: []string{"poker"},
	}, types.GamingSettings{}, true)
	if err != nil {
		t.Fatalf("registering an unfunded game was refused: %v", err)
	}
	if !s.Enabled {
		t.Fatal("the bridge switched itself off because nothing was funded")
	}

	_, err = checkSpendRequest(s, "poker", "Tsaddr", 1_000_000)
	if !errors.Is(err, ErrGamingSpendRefused) {
		t.Fatalf("an unfunded game was carried: %v", err)
	}
	if !strings.Contains(err.Error(), "poker") {
		t.Errorf("the refusal does not name the game: %v", err)
	}
}

// The dropdown does not offer the mixer, Lightning, DEX or imported accounts,
// but a request that skips the dropdown must be refused too: a game bound to
// one of them would spend funds another part of the stack is relying on.
func TestAGameCannotBeFundedFromAReservedAccount(t *testing.T) {
	for _, name := range []string{"lightning", "dex", "mixed", "unmixed", "imported"} {
		in := types.GamingSettings{
			Enabled:         true,
			RegisteredGames: []string{"poker"},
			Policies:        map[string]types.GamePolicy{"poker": {Account: name}},
		}

		_, err := normalizeGamingSettings(in, types.GamingSettings{}, true)
		if err == nil {
			t.Errorf("%q was accepted as a game's spending account", name)
			continue
		}
		if !errors.Is(err, ErrGamingReservedAccount) {
			t.Errorf("%q was refused for the wrong reason: %v", name, err)
		}
		// The operator has to be able to tell which game and which account.
		for _, want := range []string{"poker", name} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal for %q does not mention %q: %v", name, want, err)
			}
		}
	}
}

// The check lowercases, because the name is matched case-insensitively
// everywhere else a daemon binds to it.
func TestAReservedAccountIsRefusedWhateverItsCase(t *testing.T) {
	in := types.GamingSettings{
		Enabled:         true,
		RegisteredGames: []string{"poker"},
		Policies:        map[string]types.GamePolicy{"poker": {Account: "  Lightning "}},
	}
	if _, err := normalizeGamingSettings(in, types.GamingSettings{}, true); !errors.Is(err, ErrGamingReservedAccount) {
		t.Errorf("a differently-cased reserved account slipped through: %v", err)
	}
}

// An ordinary account is still the point of the feature.
func TestAnOrdinaryAccountStillFundsAGame(t *testing.T) {
	in := types.GamingSettings{
		Enabled:         true,
		RegisteredGames: []string{"poker"},
		Policies:        map[string]types.GamePolicy{"poker": {Account: "poker-money"}},
	}
	out, err := normalizeGamingSettings(in, types.GamingSettings{}, true)
	if err != nil {
		t.Fatalf("an ordinary account was refused: %v", err)
	}
	if got := out.Policies["poker"].Account; got != "poker-money" {
		t.Errorf("the account came back as %q, want poker-money", got)
	}
}
