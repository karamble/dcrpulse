// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "msig.json")
	s, err := openStore(path, "w1")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	rec := &WalletRecord{
		TempID: "00aa", Label: "team", M: 2, N: 3, Network: "simnet",
		Role: RoleInitiator, Status: StatusInviting,
		HD:    true,
		OwnHD: &OwnHDKey{Xpub: "spub-test", Account: 7},
		Ext:   &CursorState{Next: 3, ImportedThrough: 20, LastUsed: 1},
		Peers: []*Peer{{UID: "u1", Nick: "bob", State: PeerInvited}},
	}
	if err := s.PutWallet(rec); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := s.UpdateWallet("00aa", func(r *WalletRecord) error {
		r.Address = "SsShared"
		r.Status = StatusActivating
		return nil
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	re, err := openStore(path, "w1")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok := re.Wallet("00aa")
	if !ok {
		t.Fatalf("record lost on reopen")
	}
	if got.Address != "SsShared" || got.Status != StatusActivating || got.OwnHD.Account != 7 || got.Ext.Next != 3 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	// Lookup by address works too.
	if _, ok := re.Wallet("SsShared"); !ok {
		t.Fatalf("address lookup failed")
	}
	// Clones do not alias store state.
	got.Peers[0].State = PeerReady
	again, _ := re.Wallet("00aa")
	if again.Peers[0].State != PeerInvited {
		t.Fatalf("clone aliases store state")
	}
}

func TestStoreJournalAndOutbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "msig.json")
	s, err := openStore(path, "w1")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	now := time.Now()
	fresh, err := s.MarkProcessed("mid1", now)
	if err != nil || !fresh {
		t.Fatalf("first mark: fresh=%v err=%v", fresh, err)
	}
	fresh, err = s.MarkProcessed("mid1", now)
	if err != nil || fresh {
		t.Fatalf("duplicate mark accepted")
	}

	// The invite fans one mid out to several peers; sends complete
	// per recipient.
	for _, uid := range []string{"u1", "u2"} {
		if err := s.AppendOutbox(&OutboxItem{
			MID: "m1", ToUID: uid, Body: "b", State: OutboxSending, Ts: now.Unix(),
		}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := s.MarkOutboxSent("m1", "u1"); err != nil {
		t.Fatalf("mark sent: %v", err)
	}
	pending := s.PendingOutbox()
	if len(pending) != 1 || pending[0].ToUID != "u2" {
		t.Fatalf("pending after partial send: %+v", pending)
	}

	re, err := openStore(path, "w1")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if fresh, _ := re.MarkProcessed("mid1", now); fresh {
		t.Fatalf("journal lost on reopen")
	}
	if len(re.PendingOutbox()) != 1 {
		t.Fatalf("outbox lost on reopen")
	}
}

func TestOpenStoreRefusesFutureSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "msig.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":99,"wallets":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openStore(path, "w1"); err == nil {
		t.Fatalf("future schema accepted; a downgrade would strip its fields")
	}
}
