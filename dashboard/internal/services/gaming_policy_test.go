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
func TestTheBridgeCannotBeTurnedOnWithoutAHumanGate(t *testing.T) {
	bound := func() types.GamingSettings {
		return types.GamingSettings{Enabled: true, Account: "gaming"}
	}

	for _, tc := range []struct {
		name       string
		in         types.GamingSettings
		appPass    bool
		wantErr    error
		wantOn     bool
		wantReason string
	}{
		{
			name: "enabling with the gate on", in: bound(), appPass: true,
			wantOn: true,
		},
		{
			name: "enabling with the gate off", in: bound(), appPass: false,
			wantErr:    ErrGamingNeedsAppPassword,
			wantReason: "an operator told to fix it must be told what to fix",
		},
		{
			name: "switching off never needs the gate",
			in:   types.GamingSettings{Enabled: false, Account: "gaming"}, appPass: false,
			wantOn: false,
		},
		{
			// Not a policy choice: with no account bound there is nothing
			// for a game to spend from, so an enabled bridge would only
			// look active.
			name: "enabling with no account bound",
			in:   types.GamingSettings{Enabled: true, Account: "  "}, appPass: true,
			wantOn: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := normalizeGamingSettings(tc.in, types.GamingSettings{}, tc.appPass)
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
		types.GamingSettings{Enabled: true, Account: "gaming"}, types.GamingSettings{}, false)
	if err == nil {
		t.Fatal("enabling without the gate was allowed")
	}
	if got := err.Error(); !strings.Contains(got, "App Password") {
		t.Fatalf("the refusal does not name the App Password: %q", got)
	}
}

// The stored policy is what a later read gets, so a refused enable must not
// reach the file at all.
func TestARefusedEnableIsNotStored(t *testing.T) {
	spendSeams(t)

	if _, err := WriteGamingSettings(
		types.GamingSettings{Enabled: true, Account: "gaming"}, false); !errors.Is(err, ErrGamingNeedsAppPassword) {
		t.Fatalf("got %v, want a refusal", err)
	}
	if ReadGamingSettings().Enabled {
		t.Fatal("a refused enable was stored anyway")
	}

	if _, err := WriteGamingSettings(
		types.GamingSettings{Enabled: true, Account: "gaming"}, true); err != nil {
		t.Fatalf("enabling behind the gate: %v", err)
	}
	if !ReadGamingSettings().Enabled {
		t.Fatal("an allowed enable was not stored")
	}
}
