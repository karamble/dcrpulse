// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"dcrpulse/internal/types"
)

// A token is the game's identity and the bridge issues it.
//
// A view that read one back would let whoever can reach the settings route name
// a game's identity - and naming its own is the one thing a game must not be
// able to do either. It goes out so the operator can copy it into the game, and
// it never comes home.
func TestThePolicyViewNeverTakesATokenBack(t *testing.T) {
	body := `{
		"enabled": true,
		"registeredGames": ["poker"],
		"policies": {"poker": {"account": "gaming", "perTableCapDcr": 1, "perDayCapDcr": 5}},
		"gameTokens": {"poker": "a-token-somebody-chose"}
	}`

	var in gamingSettingsView
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&in); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The view does carry tokens outward, so the field is populated here -
	// which is exactly why the conversion below has to drop them.
	if in.GameTokens["poker"] == "" {
		t.Fatal("the test body did not carry a token, so it proves nothing")
	}

	out, err := gamingFromView(in)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(out.GameTokens) != 0 {
		t.Fatalf("a token supplied by the caller was taken back: %v", out.GameTokens)
	}
}

// Caps cross the wire in DCR and are stored in atoms; a round trip must not
// quietly move the decimal point, because both ends of that conversion are a
// limit on real money.
func TestPolicyCapsSurviveTheRoundTrip(t *testing.T) {
	stored := types.GamingSettings{
		Enabled:         true,
		RegisteredGames: []string{"poker"},
		Policies: map[string]types.GamePolicy{"poker": {
			Name: "Poker", Account: "gaming",
			PerTableCapAtoms: 100_000_000, PerDayCapAtoms: 512_345_678,
			ApprovalTimeoutSecs: 120,
		}},
	}

	back, err := gamingFromView(gamingToView(stored))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	got, want := back.Policies["poker"], stored.Policies["poker"]
	if got != want {
		t.Fatalf("the policy came back as %+v, want %+v", got, want)
	}
}
