// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import "testing"

// The gaming policy holds the identity registered for each game, so moving it
// is not a rename: a bridge that looks in a new place finds no registrations,
// every game it knew becomes a stranger, and the person has to issue fresh
// credentials for each one and carry them across by hand.
//
// It used to live on a volume of its own, /control/gaming.json, so that a game
// running here as a container could mount that and nothing else. Nothing runs
// here now, so it sits in the stack control directory with every other
// service's control file.
const gamingPolicyLivesAt = "/app-data/control/gaming.json"

func TestTheGamingPolicyKeepsItsPlace(t *testing.T) {
	if got := GamingSettingsPath(); got != gamingPolicyLivesAt {
		t.Fatalf("the gaming policy moved to %q from %q, which loses every registered game",
			got, gamingPolicyLivesAt)
	}
}

// The spend log is the audit trail a person reads and the record the daily cap
// is counted from, so losing track of it would silently reset both.
func TestTheSpendLogKeepsItsPlace(t *testing.T) {
	if got, want := GamingSpendLogPath(), "/app-data/control/gaming-spends.json"; got != want {
		t.Fatalf("the gaming spend log moved to %q from %q", got, want)
	}
}
