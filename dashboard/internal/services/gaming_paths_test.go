// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

// Where the gaming section keeps its state, pinned because moving either file
// is not a rename.
//
// The policy holds the identity registered for each game: a bridge that looks
// somewhere new finds no registrations, every game it knew becomes a stranger,
// and the person has to issue fresh credentials and carry each one across by
// hand. The spend log is the audit trail a person reads and the record the
// daily cap counts from, so losing track of it silently resets both.
//
// The directory is a variable so tests can point it at somewhere writable.
// These assert what production actually gets.
func TestTheGamingStateKeepsItsPlace(t *testing.T) {
	for _, tc := range []struct {
		what string
		got  string
		want string
	}{
		{"policy", gamingSettingsPath(), "/app-data/control/gaming.json"},
		{"spend log", gamingSpendLogPath(), "/app-data/control/gaming-spends.json"},
	} {
		if tc.got != tc.want {
			t.Errorf("the gaming %s moved to %q from %q", tc.what, tc.got, tc.want)
		}
	}
}
