// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"decred.org/dcrwallet/v5/wallet/udb"
)

// restoreHarness settles a 2-of-2 between alice and bob and returns a twin of
// alice (same seed) ready to restore alice's card, plus the card's JSON.
func restoreHarness(t *testing.T) (*hdHarness, []byte) {
	t.Helper()
	hd := newHDHarness(t, "alice", "bob", "twin")
	hd.masters[hd.nodeByNick("twin").uid] = hd.masters[hd.nodeByNick("alice").uid]
	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()
	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	hd.settle(t, tempID, "alice", "bob")
	hd.as("alice")
	card, err := ExportBackupCard(hd.ctx, tempID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	// The twin already carries the account, so no passphrase is needed.
	hd.accounts[hd.nodeByNick("twin").uid] = []string{"renamed-by-hand"}
	origRescan := rescanSeam
	t.Cleanup(func() { rescanSeam = origRescan })
	rescanSeam = func(int64) {}
	hd.as("twin")
	return hd, raw
}

func cardFrom(t *testing.T, raw []byte) *BackupCard {
	t.Helper()
	var c BackupCard
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

func restoredCount(t *testing.T, hd *hdHarness) int {
	t.Helper()
	_, recs, err := ListForActiveWallet(hd.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return len(recs)
}

// A backup card is refused, with nothing saved, when what the restore keeps
// from it is malformed; before, a huge cursor ran the process out of memory
// from a record already on disk, so every later start crashed the same way.
func TestRestoreRefusesMalformedCard(t *testing.T) {
	hd, raw := restoreHarness(t)
	max := uint32(udb.MaxAddressesPerAccount)
	for name, spoil := range map[string]func(r *WalletRecord){
		"a huge receive cursor":            func(r *WalletRecord) { r.Ext = &CursorState{Next: 3_000_000_000, LastUsed: 3_000_000_000} },
		"a receive cursor past the gap":    func(r *WalletRecord) { r.Ext = &CursorState{Next: 5 + GapExt + 1, LastUsed: 5} },
		"a change cursor past dcrwallet's": func(r *WalletRecord) { r.Int = &CursorState{Next: max} },
		"an empty peer entry":              func(r *WalletRecord) { r.Peers = append(r.Peers, nil) },
		"an unknown role":                  func(r *WalletRecord) { r.Role = "owner" },
		"a bad wallet id":                  func(r *WalletRecord) { r.TempID = "../x" },
		"an overlong label":                func(r *WalletRecord) { r.Label = strings.Repeat("x", MaxLabelLen+1) },
	} {
		t.Run(name, func(t *testing.T) {
			card := cardFrom(t, raw)
			spoil(card.Record)
			_, err := ImportBackupCard(hd.ctx, card, nil)
			if err == nil || !strings.Contains(err.Error(), "malformed") {
				t.Fatalf("got %v, want a malformed-card refusal", err)
			}
			if n := restoredCount(t, hd); n != 0 {
				t.Fatalf("a refused card left %d record(s)", n)
			}
		})
	}

	// In-flight state from the card is dropped, including entries that
	// could not be copied.
	card := cardFrom(t, raw)
	card.Record.Proposals = map[string]*Proposal{"x": {Queue: []*QueueHop{nil}}}
	rec, err := ImportBackupCard(hd.ctx, card, nil)
	if err != nil {
		t.Fatalf("a valid card with stale proposals: %v", err)
	}
	if len(rec.Proposals) != 0 {
		t.Fatalf("restored %d proposal(s) from the card", len(rec.Proposals))
	}
}

// A restore that stops part way, here because the request was cancelled,
// leaves no record behind to repeat the import on the next start.
func TestRestoreCancelledSavesNothing(t *testing.T) {
	hd, raw := restoreHarness(t)
	card := cardFrom(t, raw)
	card.Record.Int = &CursorState{Next: 4 * ladderImportChunk}

	ctx, cancel := context.WithCancel(hd.ctx)
	defer cancel()
	orig := importScriptSeam
	t.Cleanup(func() { importScriptSeam = orig })
	imports := 0
	importScriptSeam = func(c context.Context, scriptHex string, rescan bool, scanFrom int64) error {
		if imports++; imports == ladderImportChunk {
			cancel()
		}
		return orig(c, scriptHex, rescan, scanFrom)
	}
	if _, err := ImportBackupCard(ctx, card, nil); err == nil {
		t.Fatal("a cancelled restore reported success")
	}
	if imports < ladderImportChunk {
		t.Fatalf("the restore stopped before the import (%d scripts)", imports)
	}
	if imports >= 4*ladderImportChunk {
		t.Fatalf("imported %d scripts after the cancel", imports)
	}
	if n := restoredCount(t, hd); n != 0 {
		t.Fatalf("a cancelled restore left %d record(s)", n)
	}
}

func TestWindowEndStopsAtDcrwalletsLastIndex(t *testing.T) {
	if got := windowEnd(&CursorState{Next: math.MaxUint32}, GapExt); got != udb.MaxAddressesPerAccount {
		t.Fatalf("windowEnd = %d, want %d", got, udb.MaxAddressesPerAccount)
	}
	if got := windowEnd(&CursorState{Next: 7, LastUsed: 3, ImportedThrough: 2}, GapExt); got != 7+GapExt {
		t.Fatalf("windowEnd = %d, want %d", got, 7+GapExt)
	}
}
