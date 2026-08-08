// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import (
	"fmt"
	"path/filepath"
	"sync"
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

// UpsertUsedVSP and UpsertPoliteiaVote add one entry to a map-valued key. Two
// writers touching different entries must not drop each other's, which merging
// whole keys alone does not give: both would write their own copy of the map.
func TestWalletCfgUpsertMergesEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	bg, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load bg: %v", err)
	}
	ui, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load ui: %v", err)
	}

	if err := bg.UpsertUsedVSP(VSPMetadata{Host: "vsp-a.example.com", LastUsed: 1}); err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	if err := bg.Save(); err != nil {
		t.Fatalf("save bg: %v", err)
	}
	if err := ui.UpsertUsedVSP(VSPMetadata{Host: "vsp-b.example.com", LastUsed: 2}); err != nil {
		t.Fatalf("upsert b: %v", err)
	}
	if err := ui.Save(); err != nil {
		t.Fatalf("save ui: %v", err)
	}

	reread, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	used, err := reread.UsedVSPs()
	if err != nil {
		t.Fatalf("used vsps: %v", err)
	}
	if len(used) != 2 {
		t.Fatalf("used_vsps has %d entries, want 2: %v", len(used), used)
	}
	if used["vsp-a.example.com"].LastUsed != 1 || used["vsp-b.example.com"].LastUsed != 2 {
		t.Fatalf("entries mangled: %v", used)
	}
}

func TestWalletCfgUpsertPoliteiaVoteMergesEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	first, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load first: %v", err)
	}
	second, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load second: %v", err)
	}

	if err := first.UpsertPoliteiaVote("token-a", "yes", 3); err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	if err := first.Save(); err != nil {
		t.Fatalf("save first: %v", err)
	}
	if err := second.UpsertPoliteiaVote("token-b", "no", 5); err != nil {
		t.Fatalf("upsert b: %v", err)
	}
	if err := second.Save(); err != nil {
		t.Fatalf("save second: %v", err)
	}

	reread, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	votes, err := reread.PoliteiaVotes()
	if err != nil {
		t.Fatalf("votes: %v", err)
	}
	counts, err := reread.PoliteiaVoteCounts()
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if votes["token-a"] != "yes" || votes["token-b"] != "no" {
		t.Fatalf("votes lost an entry: %v", votes)
	}
	if counts["token-a"] != 3 || counts["token-b"] != 5 {
		t.Fatalf("counts lost an entry: %v", counts)
	}
}

// A whole-key write must beat entries staged for that key, so replacing a map
// wholesale is not undone by a stale entry.
func TestWalletCfgSetSupersedesStagedEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	c, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := c.UpsertUsedVSP(VSPMetadata{Host: "stale.example.com"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := c.Set(KeyUsedVSPs, map[string]VSPMetadata{
		"fresh.example.com": {Host: "fresh.example.com"},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := c.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	reread, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	used, err := reread.UsedVSPs()
	if err != nil {
		t.Fatalf("used vsps: %v", err)
	}
	if len(used) != 1 || used["fresh.example.com"].Host != "fresh.example.com" {
		t.Fatalf("whole-key set did not supersede the staged entry: %v", used)
	}
}

// The tests above interleave saves by hand. This one runs the writers for
// real, which is what the package-level mutex exists for; run with -race.
func TestConcurrentWritersKeepEveryKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	const writers = 16
	var wg sync.WaitGroup
	errs := make(chan error, writers)

	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := loadWalletCfgAt(path)
			if err != nil {
				errs <- err
				return
			}
			if err := c.Set(fmt.Sprintf("key_%02d", i), i); err != nil {
				errs <- err
				return
			}
			if err := c.UpsertUsedVSP(VSPMetadata{
				Host: fmt.Sprintf("vsp-%02d.example.com", i), LastUsed: int64(i),
			}); err != nil {
				errs <- err
				return
			}
			if err := c.Save(); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("writer: %v", err)
	}

	reread, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	for i := range writers {
		var got int
		key := fmt.Sprintf("key_%02d", i)
		if ok, err := reread.Get(key, &got); err != nil || !ok {
			t.Fatalf("%s missing (ok=%v err=%v)", key, ok, err)
		}
		if got != i {
			t.Fatalf("%s = %d, want %d", key, got, i)
		}
	}
	used, err := reread.UsedVSPs()
	if err != nil {
		t.Fatalf("used vsps: %v", err)
	}
	if len(used) != writers {
		t.Fatalf("used_vsps has %d entries, want %d", len(used), writers)
	}
}
