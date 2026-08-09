// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"testing"
)

// A save must not erase settings fields the daemon knows and this build does
// not: unknown keys survive, every owned key is overwritten, and the
// reply-only last_denied never rides back.
func TestMergeOwnedMCPSettingsPreservesUnknownFields(t *testing.T) {
	current := json.RawMessage(`{
		"enabled": true,
		"token": "old-token",
		"mode": "auto",
		"per_call_cap_atoms": 1,
		"per_day_cap_atoms": 2,
		"allowed_bots": ["a"],
		"allowed_ips": ["1.2.3.4"],
		"approval_timeout_secs": 5,
		"tip_wait_secs": 6,
		"per_week_cap_atoms": 777,
		"future_flag": {"nested": true},
		"last_denied": {"ip": "9.9.9.9", "at": "then"}
	}`)
	wire := brMCPSettingsWire{
		Enabled:             false,
		Token:               "new-token",
		Mode:                "manual",
		PerCallCapAtoms:     10,
		PerDayCapAtoms:      20,
		AllowedBots:         []string{"b"},
		AllowedIPs:          []string{},
		ApprovalTimeoutSecs: 50,
		TipWaitSecs:         60,
	}

	merged, err := mergeOwnedMCPSettings(current, wire)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if _, ok := merged["last_denied"]; ok {
		t.Error("reply-only last_denied was posted back")
	}
	for key, want := range map[string]string{
		"per_week_cap_atoms":    `777`,
		"future_flag":           `{"nested": true}`,
		"enabled":               `false`,
		"token":                 `"new-token"`,
		"mode":                  `"manual"`,
		"per_call_cap_atoms":    `10`,
		"per_day_cap_atoms":     `20`,
		"allowed_bots":          `["b"]`,
		"allowed_ips":           `[]`,
		"approval_timeout_secs": `50`,
		"tip_wait_secs":         `60`,
	} {
		got, ok := merged[key]
		if !ok {
			t.Errorf("%s missing from merge", key)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %s, want %s", key, got, want)
		}
	}
}

func TestMergeOwnedMCPSettingsRejectsMalformedCurrent(t *testing.T) {
	if _, err := mergeOwnedMCPSettings(json.RawMessage(`not json`), brMCPSettingsWire{}); err == nil {
		t.Fatal("malformed current settings did not error")
	}
}
