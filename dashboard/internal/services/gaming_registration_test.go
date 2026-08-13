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

// The id an operator registers is the routing key the wire carries.
//
// It becomes the `game=` key in the envelope and the host part of a gaming://
// invitation, so an id this host accepts but the wire cannot carry is a game
// whose frames are dropped forever with nothing anywhere to say why. The rule
// belongs to the wire and to the game on the other side of it, not to this
// build's taste.
func TestARegisteredGameIdIsTheRoutingKeyTheWireAccepts(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string // "" means refused
		why  string
	}{
		{"poker", "poker", "the ordinary case"},
		{"dcr_poker", "dcr_poker", "underscores are on the wire's list"},
		{"x-1", "x-1", "so are hyphens and digits"},
		{"a", "a", "one character is an id"},
		{strings.Repeat("a", 32), strings.Repeat("a", 32), "thirty-two is the limit"},
		{"Poker", "poker", "case is folded, not refused"},
		{"  poker  ", "poker", "surrounding space is tidying"},
		{"-poker", "", "an id starts with a letter or a digit"},
		{"_poker", "", "and an underscore is not one"},
		{strings.Repeat("a", 33), "", "thirty-three is past the limit"},
		{"pok er", "", "a space inside is not a routing key"},
		{"poker!", "", "nor is punctuation"},
		{"póker", "", "nor is anything outside ascii"},
	} {
		got, err := sanitizeRegisteredGames([]string{tc.in})
		if tc.want == "" {
			if !errors.Is(err, ErrGamingBadGameID) {
				t.Errorf("%q was accepted, but %s", tc.in, tc.why)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q was refused, but %s: %v", tc.in, tc.why, err)
			continue
		}
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("%q became %v, want %q - %s", tc.in, got, tc.want, tc.why)
		}
	}
}

// An id the wire cannot carry comes back as an error rather than vanishing.
//
// The operator types this now, so a dropped typo leaves them looking at a list
// that did not grow, with nothing to read and nothing to correct.
func TestAnUnroutableGameIdIsRefusedRatherThanDropped(t *testing.T) {
	spendSeams(t)

	_, err := WriteGamingSettings(types.GamingSettings{
		RegisteredGames: []string{"poker", "not a game id"},
	}, true)
	if !errors.Is(err, ErrGamingBadGameID) {
		t.Fatalf("got %v, want a refusal naming the rule", err)
	}
	if !strings.Contains(err.Error(), "not a game id") {
		t.Errorf("the refusal does not say which id was wrong: %v", err)
	}
	if got := ReadGamingSettings().RegisteredGames; len(got) != 0 {
		t.Fatalf("a refused write registered %v anyway", got)
	}
}

// Duplicates and ordering are tidied rather than refused: nobody meant anything
// by them, and the stored list is what the routing table is read from.
func TestRegisteringFoldsDuplicatesAndOrders(t *testing.T) {
	got, err := sanitizeRegisteredGames([]string{"poker", "chess", "POKER", " chess "})
	if err != nil {
		t.Fatalf("tidying was refused: %v", err)
	}
	if len(got) != 2 || got[0] != "chess" || got[1] != "poker" {
		t.Fatalf("registered %v, want [chess poker]", got)
	}
}

// The list is what the operator registered, and nothing this build was built
// knowing. A game has to be able to appear without the bridge being taught its
// name, or a second game needs a release before it can play at all.
func TestTheGamesListReportsOnlyWhatWasRegistered(t *testing.T) {
	spendSeams(t)

	if _, err := WriteGamingSettings(types.GamingSettings{
		RegisteredGames: []string{"backgammon", "poker"},
		Policies:        map[string]types.GamePolicy{"poker": {Name: "Poker Night"}},
	}, true); err != nil {
		t.Fatalf("register: %v", err)
	}

	games := GamingGames()
	if len(games) != 2 {
		t.Fatalf("listed %d games, want 2: %+v", len(games), games)
	}
	if games[0].ID != "backgammon" || games[1].ID != "poker" {
		t.Fatalf("listed %v, want them sorted", []string{games[0].ID, games[1].ID})
	}
	// A game the operator labelled is shown by that label; one they did not
	// is shown by its id, which is the only name the wire carries.
	if games[0].Name != "backgammon" {
		t.Errorf("an unlabelled game is called %q, want its id", games[0].Name)
	}
	if games[1].Name != "Poker Night" {
		t.Errorf("a labelled game is called %q, want the operator's label", games[1].Name)
	}
	for _, g := range games {
		if g.Ready {
			t.Errorf("%s is reported connected, but nothing has connected", g.ID)
		}
	}
}
