// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestDexRateSourceSavesKeepEachOther(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	a, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load a: %v", err)
	}
	b, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("load b: %v", err)
	}
	if got, err := a.DexRateSources(); err != nil || len(got) != 0 {
		t.Fatalf("fresh config holds %v (%v)", got, err)
	}

	if err := a.SetDexRateSource("dcrdata", false); err != nil {
		t.Fatalf("set a: %v", err)
	}
	if err := a.Save(); err != nil {
		t.Fatalf("save a: %v", err)
	}
	if err := b.SetDexRateSource("Coinpaprika", true); err != nil {
		t.Fatalf("set b: %v", err)
	}
	if err := b.Save(); err != nil {
		t.Fatalf("save b: %v", err)
	}

	reread, err := loadWalletCfgAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, err := reread.DexRateSources()
	want := map[string]bool{"dcrdata": false, "Coinpaprika": true}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("rate sources %v (%v), want %v", got, err, want)
	}
}
