// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// pointCfgAt aims the package at a config file and restores both the path and
// the cached oversight state, so one test's degraded cache cannot decide
// whether the next one refuses a spend.
func pointCfgAt(t *testing.T, path string) {
	t.Helper()
	prev := cfgPath
	cfgPath = func() string { return path }
	oversightState.mu.Lock()
	pEn, pCon, pKnown, pDeg := oversightState.enabled, oversightState.contact, oversightState.known, oversightState.degraded
	oversightState.enabled, oversightState.contact = false, ""
	oversightState.known, oversightState.degraded = false, false
	oversightState.mu.Unlock()
	t.Cleanup(func() {
		cfgPath = prev
		oversightState.mu.Lock()
		oversightState.enabled, oversightState.contact = pEn, pCon
		oversightState.known, oversightState.degraded = pKnown, pDeg
		oversightState.mu.Unlock()
	})
}

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	return p
}

const overseerUID = "aa11bb22cc33dd44ee55ff6600112233445566778899aabbccddeeff00112233"

// An absent config legitimately means "not configured" and must stay a clean
// off, not a refusal. This is the case readRawJSON reports separately.
func TestOversightAbsentConfigIsOffAndKnown(t *testing.T) {
	pointCfgAt(t, filepath.Join(t.TempDir(), "does-not-exist.json"))
	enabled, contact, known := oversightConfig()
	if enabled || contact != "" {
		t.Errorf("oversightConfig() = %v, %q, want false and empty", enabled, contact)
	}
	if !known {
		t.Error("known = false for an absent config, want true: absent means not configured")
	}
	if err := gateApproval(context.Background(), "agent", "send 1 DCR"); err != nil {
		t.Errorf("gateApproval() = %v, want nil when oversight is legitimately off", err)
	}
}

// The bug: an unreadable config must not read as "oversight off".
func TestOversightUnreadableConfigIsNotOff(t *testing.T) {
	for name, body := range map[string]string{
		"broken json":     `{"mcp_notify_enabled": true`,
		"wrong typed key": `{"mcp_notify_enabled":"yes"}`,
	} {
		t.Run(name, func(t *testing.T) {
			pointCfgAt(t, writeCfg(t, body))
			_, _, known := oversightConfig()
			if known {
				t.Fatal("known = true for a config that could not be used, want false")
			}
			if !oversightDegradedNow() {
				t.Error("oversightDegradedNow() = false after a failed read, want true")
			}
			// With no successful read ever, the gate must refuse rather than
			// silently let the fund move through.
			err := gateApproval(context.Background(), "agent", "send 1 DCR")
			if !errors.Is(err, errApprovalUnreachable) {
				t.Errorf("gateApproval() = %v, want errApprovalUnreachable", err)
			}
		})
	}
}

// After a good read, a later failure must serve the cached settings rather than
// refusing: the approval DM travels over Bison Relay and does not need the file.
func TestOversightFallsBackToTheCachedSettings(t *testing.T) {
	good := writeCfg(t, `{"mcp_notify_enabled":true,"mcp_notify_contact":"`+overseerUID+`"}`)
	pointCfgAt(t, good)
	enabled, contact, known := oversightConfig()
	if !enabled || contact != overseerUID || !known {
		t.Fatalf("first read = %v, %q, %v; want true, the uid, true", enabled, contact, known)
	}

	// Now make the file unusable and read again.
	if err := os.WriteFile(good, []byte(`{not json`), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	enabled, contact, known = oversightConfig()
	if !enabled || contact != overseerUID {
		t.Errorf("degraded read = %v, %q; want the cached true and uid", enabled, contact)
	}
	if !known {
		t.Error("known = false after a successful read; the cache must still count as known")
	}
	if !oversightDegradedNow() {
		t.Error("oversightDegradedNow() = false, want true so Settings can say so")
	}
	if got := Oversight(); !got.Degraded || !got.Enabled {
		t.Errorf("Oversight() = %+v, want Enabled and Degraded both true", got)
	}
}

// An operator who never enabled oversight must not start seeing refusals just
// because the file broke. This is the row that keeps the fix from regressing
// into blanket denial.
func TestOversightCachedOffStillPermits(t *testing.T) {
	p := writeCfg(t, `{"mcp_notify_enabled":false}`)
	pointCfgAt(t, p)
	if _, _, known := oversightConfig(); !known {
		t.Fatal("first read not known")
	}
	if err := os.WriteFile(p, []byte(`{broken`), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	if _, _, known := oversightConfig(); !known {
		t.Fatal("known = false after a good read, want the cache to hold")
	}
	if err := gateApproval(context.Background(), "agent", "send 1 DCR"); err != nil {
		t.Errorf("gateApproval() = %v, want nil: oversight was known to be off", err)
	}
}

// Reads are cached now, so the Settings toggle has to refresh the cache or it
// would not take effect until a restart.
func TestSetOversightConfigRefreshesTheCache(t *testing.T) {
	pointCfgAt(t, writeCfg(t, `{}`))
	if _, _, known := oversightConfig(); !known {
		t.Fatal("first read not known")
	}
	if err := SetOversightConfig(true, overseerUID); err != nil {
		t.Fatalf("SetOversightConfig() = %v", err)
	}
	oversightState.mu.RLock()
	gotEnabled, gotContact := oversightState.enabled, oversightState.contact
	oversightState.mu.RUnlock()
	if !gotEnabled || gotContact != overseerUID {
		t.Errorf("cache = %v, %q after Set; want true and the uid", gotEnabled, gotContact)
	}
}

// persistedEnabled carries the same absent-vs-unreadable distinction as the
// oversight settings, and it decides whether the surface that mints agent
// tokens comes up at all.
func TestPersistedEnabledDistinguishesUnsetFromUnreadable(t *testing.T) {
	t.Run("absent config falls back to the env default", func(t *testing.T) {
		pointCfgAt(t, filepath.Join(t.TempDir(), "none.json"))
		v, ok, err := persistedEnabled()
		if err != nil {
			t.Fatalf("persistedEnabled() err = %v, want nil for an absent config", err)
		}
		if ok || v {
			t.Errorf("= %v, %v; want false, false so cfg.Enable decides", v, ok)
		}
	})

	t.Run("key unset falls back to the env default", func(t *testing.T) {
		pointCfgAt(t, writeCfg(t, `{"something_else":1}`))
		_, ok, err := persistedEnabled()
		if err != nil || ok {
			t.Errorf("= ok %v, err %v; want false and nil", ok, err)
		}
	})

	t.Run("persisted off is honoured", func(t *testing.T) {
		pointCfgAt(t, writeCfg(t, `{"mcp_enabled":false}`))
		v, ok, err := persistedEnabled()
		if err != nil || !ok || v {
			t.Errorf("= %v, %v, %v; want false, true, nil", v, ok, err)
		}
	})

	// The bug: these must not read as "never toggled", or an MCP_ENABLE=true
	// env default would override a Settings "off" the operator actually set.
	for name, body := range map[string]string{
		"broken json":     `{"mcp_enabled": tru`,
		"wrong typed key": `{"mcp_enabled":"yes"}`,
	} {
		t.Run(name+" is an error, not unset", func(t *testing.T) {
			pointCfgAt(t, writeCfg(t, body))
			_, ok, err := persistedEnabled()
			if err == nil {
				t.Fatal("persistedEnabled() err = nil for a config that could not be used, want an error")
			}
			if ok {
				t.Error("ok = true on an unreadable config, want false")
			}
		})
	}
}
