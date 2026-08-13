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

// A credential is the game's identity and the bridge issues it.
//
// A view that read one back would let whoever can reach the settings route name
// a game's identity - and naming its own is the one thing a game must not be
// able to do either. What goes out is only the fingerprint, so the console can
// say a credential exists and how old it is, and nothing comes home.
func TestThePolicyViewNeverTakesACredentialBack(t *testing.T) {
	body := `{
		"enabled": true,
		"registeredGames": ["poker"],
		"policies": {"poker": {"account": "gaming", "perTableCapDcr": 1, "perDayCapDcr": 5}},
		"gameCredentials": {"poker": {"fingerprint": "a-fingerprint-somebody-chose", "issuedAt": 1}}
	}`

	var in gamingSettingsView
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&in); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The view does carry the fingerprint outward, so the field is populated
	// here - which is exactly why the conversion below has to drop it.
	if in.GameCredentials["poker"].Fingerprint == "" {
		t.Fatal("the test body did not carry a credential, so it proves nothing")
	}

	out, err := gamingFromView(in)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(out.GameCredentials) != 0 {
		t.Fatalf("a credential supplied by the caller was taken back: %v", out.GameCredentials)
	}
}

// The console is never sent a game's certificate or key.
//
// The fingerprint tells two credentials apart, which is all the console needs.
// The certificate is what the allowlist verifies against and the key is the
// secret itself; a settings page that carried either would put them in every
// browser cache and every screenshot.
func TestTheViewCarriesNoCertificate(t *testing.T) {
	stored := types.GamingSettings{
		Enabled:         true,
		RegisteredGames: []string{"poker"},
		GameCredentials: map[string]types.GameCredential{"poker": {
			Fingerprint: "abc123",
			CertPEM:     "-----BEGIN CERTIFICATE-----\nnot really\n-----END CERTIFICATE-----\n",
			IssuedAt:    42,
		}},
	}

	body, err := json.Marshal(gamingToView(stored))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(string(body), "BEGIN CERTIFICATE") {
		t.Fatalf("the settings view carried a certificate to the browser: %s", body)
	}
	if !strings.Contains(string(body), "abc123") {
		t.Fatal("the fingerprint did not reach the console, so it cannot tell an issued credential from none")
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
