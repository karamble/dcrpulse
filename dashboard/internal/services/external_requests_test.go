// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"os"
	"path/filepath"
	"testing"

	"dcrpulse/internal/config"
)

// pointExternalCfg aims the external-request gate at a scratch config file.
func pointExternalCfg(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	prev := externalCfgPath
	externalCfgPath = func() string { return path }
	t.Cleanup(func() {
		externalCfgPath = prev
		externalGateDegraded.Store(false)
	})
	return path
}

func TestExternalRequestAllowed(t *testing.T) {
	tests := []struct {
		name     string
		doc      string // "" leaves the file absent
		politeia bool
		dcrtime  bool
	}{
		{"no config file", "", true, true},
		{"no allowlist", `{}`, true, true},
		{"empty allowlist", `{"allowed_external_requests":{}}`, true, true},
		{"one service off", `{"allowed_external_requests":{"politeia":false}}`, false, true},
		{"one service on", `{"allowed_external_requests":{"politeia":true,"dcrtime":false}}`, true, false},
		{"allowlist of the wrong type", `{"allowed_external_requests":"politeia"}`, false, false},
		{"config that does not parse", `{"allowed_external_requests":{"politeia":tru`, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := pointExternalCfg(t)
			if tc.doc != "" {
				if err := os.WriteFile(path, []byte(tc.doc), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if got := ExternalRequestAllowed(config.ExternalRequestPoliteia); got != tc.politeia {
				t.Errorf("politeia = %v, want %v", got, tc.politeia)
			}
			if got := ExternalRequestAllowed(config.ExternalRequestDcrtime); got != tc.dcrtime {
				t.Errorf("dcrtime = %v, want %v", got, tc.dcrtime)
			}
		})
	}
}

// Once the settings can be read again the stored toggles apply again.
func TestExternalRequestAllowedRecovers(t *testing.T) {
	path := pointExternalCfg(t)
	if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ExternalRequestAllowed(config.ExternalRequestBrseeder) {
		t.Fatal("an unreadable config allows the request")
	}
	if !externalGateDegraded.Load() {
		t.Fatal("the unreadable config was not recorded")
	}
	if err := os.WriteFile(path, []byte(`{"allowed_external_requests":{"brseeder":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !ExternalRequestAllowed(config.ExternalRequestBrseeder) {
		t.Fatal("a repaired config still refuses the request")
	}
	if externalGateDegraded.Load() {
		t.Fatal("the recovery was not recorded")
	}
}
