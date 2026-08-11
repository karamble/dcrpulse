// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
)

// ensureWindowImported reports whether it actually imported, so callers
// can gate follow-up rescans on real work.
func TestEnsureWindowImportedReportsWork(t *testing.T) {
	origImport, origSync := importScriptSeam, syncBranchSeam
	t.Cleanup(func() { importScriptSeam, syncBranchSeam = origImport, origSync })
	imports := 0
	importScriptSeam = func(ctx context.Context, scriptHex string, rescan bool, scanFrom int64) error {
		imports++
		return nil
	}
	syncBranchSeam = func(ctx context.Context, name string, branch, through uint32) error {
		return nil
	}

	params := chaincfg.MainNetParams()
	xpubs := SortXpubs([]string{
		testXpub(t, "alice", 1, params),
		testXpub(t, "bob", 1, params),
	})
	store, err := openStore(filepath.Join(t.TempDir(), "msig.json"), "w")
	if err != nil {
		t.Fatal(err)
	}
	rec := &WalletRecord{
		TempID: "tid", Label: "shared", M: 2, N: 2, Network: "mainnet",
		HD: true, Xpubs: xpubs,
		Ext: &CursorState{ImportedThrough: GapExt},
		Int: &CursorState{ImportedThrough: GapInt},
	}
	if err := store.PutWallet(rec); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	imported, err := ensureWindowImported(ctx, store, "tid")
	if err != nil || imported {
		t.Fatalf("covered window: got (%v, %v), want (false, nil)", imported, err)
	}
	if imports != 0 {
		t.Fatalf("covered window imported %d scripts", imports)
	}

	err = store.UpdateWallet("tid", func(r *WalletRecord) error {
		r.Ext.Next = 1
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	imported, err = ensureWindowImported(ctx, store, "tid")
	if err != nil || !imported {
		t.Fatalf("stale window: got (%v, %v), want (true, nil)", imported, err)
	}
	if imports == 0 {
		t.Fatal("stale window imported no scripts")
	}
}
