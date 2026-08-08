// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import (
	"path/filepath"
	"testing"
)

// Each config writer loads its own instance and rewrites the whole document,
// so a save has to merge against whatever is on disk rather than overwrite it.
// These tests stand in for two goroutines racing: interleaving the calls by
// hand is deterministic where a real race would not be.

func TestGlobalCfgSaveKeepsAnotherWritersKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	// Both writers load before either saves, which is the race window.
	auth, err := loadGlobalCfgAt(path)
	if err != nil {
		t.Fatalf("load auth: %v", err)
	}
	theme, err := loadGlobalCfgAt(path)
	if err != nil {
		t.Fatalf("load theme: %v", err)
	}

	if err := auth.Set("auth_session_secret", "s3cret"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	if err := auth.Save(); err != nil {
		t.Fatalf("save auth: %v", err)
	}

	if err := theme.Set("theme", "midnight"); err != nil {
		t.Fatalf("set theme: %v", err)
	}
	if err := theme.Save(); err != nil {
		t.Fatalf("save theme: %v", err)
	}

	reread, err := loadGlobalCfgAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	var secret, name string
	if ok, err := reread.Get("auth_session_secret", &secret); err != nil || !ok {
		t.Fatalf("auth_session_secret lost to the theme save (ok=%v err=%v)", ok, err)
	}
	if secret != "s3cret" {
		t.Fatalf("secret = %q, want %q", secret, "s3cret")
	}
	if ok, err := reread.Get("theme", &name); err != nil || !ok {
		t.Fatalf("theme missing (ok=%v err=%v)", ok, err)
	}
	if name != "midnight" {
		t.Fatalf("theme = %q, want %q", name, "midnight")
	}
}

func TestWalletCfgSaveKeepsAnotherWritersKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	a, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load a: %v", err)
	}
	b, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load b: %v", err)
	}

	if err := a.Set(KeyLastAccess, int64(1234)); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := a.Save(); err != nil {
		t.Fatalf("save a: %v", err)
	}
	if err := b.SetRememberedVSPHost("vsp.example.com"); err != nil {
		t.Fatalf("set vsp: %v", err)
	}
	if err := b.Save(); err != nil {
		t.Fatalf("save b: %v", err)
	}

	reread, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reread.LastAccess(); got != 1234 {
		t.Fatalf("last_access = %d, want 1234 (lost to the VSP save)", got)
	}
	if got := reread.RememberedVSPHost(); got != "vsp.example.com" {
		t.Fatalf("remembered vsp host = %q, want vsp.example.com", got)
	}
}

// A key another writer added must not be resurrected by a stale instance, and
// a delete must not be undone by one.
func TestWalletCfgDeleteAndMergeInteract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	seed, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	if err := seed.Set("doomed", "value"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := seed.Save(); err != nil {
		t.Fatalf("save seed: %v", err)
	}

	remover, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load remover: %v", err)
	}
	writer, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load writer: %v", err)
	}

	remover.Delete("doomed")
	if err := remover.Save(); err != nil {
		t.Fatalf("save remover: %v", err)
	}

	// writer still holds "doomed" in its loaded snapshot but never touched it,
	// so saving must not bring it back.
	if err := writer.Set("kept", "yes"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := writer.Save(); err != nil {
		t.Fatalf("save writer: %v", err)
	}

	reread, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reread.Has("doomed") {
		t.Fatalf("deleted key resurrected by a stale writer")
	}
	if !reread.Has("kept") {
		t.Fatalf("kept key missing")
	}
}

// Unknown keys must round-trip, which is the property the document shape
// promises, including across another writer's save.
func TestWalletCfgUnknownKeysSurviveConcurrentWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	// Loaded before the foreign key exists, so it cannot know about it.
	stale, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load stale: %v", err)
	}

	foreign, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load foreign: %v", err)
	}
	if err := foreign.Set("dex_account", map[string]string{"host": "dex.example.com"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := foreign.Save(); err != nil {
		t.Fatalf("save foreign: %v", err)
	}

	if err := stale.Set(KeyLastAccess, int64(99)); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := stale.Save(); err != nil {
		t.Fatalf("save stale: %v", err)
	}

	reread, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	var acct map[string]string
	if ok, err := reread.Get("dex_account", &acct); err != nil || !ok {
		t.Fatalf("unknown key dropped (ok=%v err=%v)", ok, err)
	}
	if acct["host"] != "dex.example.com" {
		t.Fatalf("unknown key mangled: %v", acct)
	}
}
