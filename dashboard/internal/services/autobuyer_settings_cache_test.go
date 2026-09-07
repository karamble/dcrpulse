// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"dcrpulse/internal/config"
)

// The settings are read from disk and the wallet once per wallet, until they
// are saved, a VSP is recorded, an account is renamed, or the wallet changes.
func TestAutobuyerSettingsAreReadOnceUntilChanged(t *testing.T) {
	withWallet(t, &privacyWallet{}) // has "default" at 0 for the name lookup
	reads := 0
	raw := &config.AutobuyerSettings{Account: "default", BalanceToMaintain: 5e8}
	prev := readAutobuyerCfg
	readAutobuyerCfg = func(context.Context) (*config.AutobuyerSettings, string, string, error) {
		reads++
		return raw, "vsp.test", "k", nil
	}
	t.Cleanup(func() { readAutobuyerCfg = prev; invalidateAutobuyerSettings() })
	invalidateAutobuyerSettings()

	load := func() *config.AutobuyerSettings {
		t.Helper()
		s, err := LoadAutobuyerSettings(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if s == nil {
			return nil
		}
		return &config.AutobuyerSettings{Account: "default", BalanceToMaintain: int64(s.BalanceToMaintain * 1e8)}
	}
	first, second := load(), load()
	if reads != 1 {
		t.Fatalf("two loads read the disk %d times, want 1", reads)
	}
	if first == nil || second == nil || *first != *second {
		t.Fatalf("the two loads disagree: %+v vs %+v", first, second)
	}

	invalidateAutobuyerSettings()
	load()
	if reads != 2 {
		t.Fatalf("a load after a change read the disk %d times in total, want 2", reads)
	}

	// Another wallet is another file.
	activeWalletMu.Lock()
	prevWallet := activeWallet
	activeWallet = "another-wallet"
	activeWalletMu.Unlock()
	t.Cleanup(func() {
		activeWalletMu.Lock()
		activeWallet = prevWallet
		activeWalletMu.Unlock()
	})
	load()
	if reads != 3 {
		t.Fatalf("a load on another wallet read the disk %d times in total, want 3", reads)
	}

	// An account that does not resolve is not remembered: the wallet may
	// just be unreachable for a moment.
	raw = &config.AutobuyerSettings{Account: "ghost", BalanceToMaintain: 5e8}
	invalidateAutobuyerSettings()
	if s := load(); s != nil {
		t.Fatalf("an unresolvable account produced settings: %+v", s)
	}
	load()
	if reads != 5 {
		t.Fatalf("a failed lookup was cached: %d reads, want 5", reads)
	}

	// "Nothing configured" is the common case and is remembered.
	raw = nil
	invalidateAutobuyerSettings()
	load()
	load()
	if reads != 6 {
		t.Fatalf("an unconfigured wallet is re-read every poll: %d reads, want 6", reads)
	}
}
