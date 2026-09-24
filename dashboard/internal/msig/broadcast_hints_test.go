// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.

package msig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func hintTestStore(t *testing.T, wallets, peers int) *Store {
	t.Helper()
	s, err := openStore(filepath.Join(t.TempDir(), "msig.json"), "test-wallet")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < wallets; i++ {
		r := &WalletRecord{TempID: fmt.Sprint(i), Address: fmt.Sprintf("address%d", i), Status: StatusActive, Proposals: make(map[string]*Proposal)}
		for j := 0; j < peers; j++ {
			r.Peers = append(r.Peers, &Peer{UID: fmt.Sprint(j), Nick: fmt.Sprint(j)})
		}
		if err := s.PutWallet(r); err != nil {
			t.Fatal(err)
		}
	}
	oldActive, oldLookup, oldGeneration := activeWalletSeam, txLookupSeam, walletGenerationSeam
	activeWalletSeam = func() string { return s.walletName }
	walletGenerationSeam = func() uint64 { return 0 }
	txLookupSeam = func(context.Context, string) (int64, bool, error) { return 0, false, nil }
	t.Cleanup(func() { activeWalletSeam, txLookupSeam, walletGenerationSeam = oldActive, oldLookup, oldGeneration })
	return s
}

func hintTx(i int) string { return fmt.Sprintf("%064x", i+1) }
func admitHint(t *testing.T, s *Store, w string, i int, peer string, now time.Time, want string) {
	t.Helper()
	got, err := s.admitBroadcastHint(w, hintTx(i), peer, "untrusted nickname", now)
	if err != nil || got != want {
		t.Fatalf("admission %s/%d: %s %v, want %s", w, i, got, err, want)
	}
}
func reopenHints(t *testing.T, s *Store) *Store {
	t.Helper()
	fresh, err := openStore(s.path, s.walletName)
	if err != nil {
		t.Fatal(err)
	}
	return fresh
}

func TestBroadcastHintCapacityAndNoDuplicateWrites(t *testing.T) {
	s := hintTestStore(t, 5, 5)
	now := time.Now()
	for w := 0; w < 4; w++ {
		for p := 0; p < 4; p++ {
			for i := 0; i < hintMemberLimit; i++ {
				admitHint(t, s, fmt.Sprint(w), p*hintMemberLimit+i, fmt.Sprint(p), now, "processed")
			}
		}
	}
	admitHint(t, s, "0", 1000, "0", now, "ignored") // member + wallet
	admitHint(t, s, "0", 1000, "4", now, "ignored") // wallet
	admitHint(t, s, "4", 1000, "0", now, "ignored") // store
	before, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		admitHint(t, s, "0", 0, "0", now.Add(24*time.Hour), "duplicate")
	}
	after, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("duplicates rewrote persisted state")
	}
	s = reopenHints(t, s)
	total := 0
	for _, r := range s.Wallets() {
		total += len(r.BroadcastHints)
		if len(r.Proposals) != 0 {
			t.Fatal("unobserved proposal")
		}
	}
	if total != hintStoreLimit || len(s.data.ProcessedMids) != 0 {
		t.Fatalf("unbounded queue or MID journal: %d/%d", total, len(s.data.ProcessedMids))
	}
	// Expiration works even when another underlying wallet is selected.
	activeWalletSeam = func() string { return "other" }
	sweepBroadcastHints(context.Background(), s, now.Add(hintLifetime))
	admitHint(t, s, "4", 1000, "0", now.Add(hintLifetime), "processed")
}

func TestBroadcastHintMemberIsolationAndRollingAdmissions(t *testing.T) {
	s := hintTestStore(t, 1, 2)
	now := time.Now()
	// Successful observations remove pending hints, but never reset admission quotas.
	for i := 0; i < hintMemberDailyLimit; i++ {
		admitHint(t, s, "0", i, "0", now, "processed")
		s.mu.Lock()
		delete(s.data.Wallets["0"].BroadcastHints, hintTx(i))
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	s = reopenHints(t, s)
	admitHint(t, s, "0", 999, "0", now, "ignored")
	admitHint(t, s, "0", 999, "1", now, "processed")
	admitHint(t, s, "0", 1000, "0", now.Add(24*time.Hour), "processed")
}

func TestBroadcastHintLookupBudgetSurvivesRefreshAndRestart(t *testing.T) {
	s := hintTestStore(t, 1, 1)
	now := time.Now()
	calls := 0
	txLookupSeam = func(ctx context.Context, txid string) (int64, bool, error) {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("missing lookup timeout")
		}
		return 0, false, nil
	}
	for i := 0; i < hintMemberLimit; i++ {
		admitHint(t, s, "0", i, "0", now, "processed")
	}
	sweepBroadcastHints(context.Background(), s, now)
	if calls != hintLookupLimit {
		t.Fatalf("calls %d", calls)
	}
	s = reopenHints(t, s)
	for i := 0; i < 3; i++ {
		sweepBroadcastHints(context.Background(), s, now.Add(time.Minute))
	}
	if calls != hintLookupLimit {
		t.Fatal("refresh/restart reset budget")
	}
	sweepBroadcastHints(context.Background(), s, now.Add(hintLookupWindow))
	if calls != 2*hintLookupLimit {
		t.Fatalf("next window calls %d", calls)
	}
	r, _ := s.Wallet("0")
	if len(r.Proposals) != 0 {
		t.Fatal("lookup misses made proposals")
	}
}

func TestBroadcastHintObservationAndExpiry(t *testing.T) {
	s := hintTestStore(t, 1, 1)
	now := time.Now()
	admitHint(t, s, "0", 0, "0", now, "processed")
	txLookupSeam = func(context.Context, string) (int64, bool, error) { return 0, true, nil }
	sweepBroadcastHints(context.Background(), s, now)
	r, p, ok := s.Proposal("0", hintTx(0))
	if !ok || p.Status != ProposalBroadcast || !p.NoticeOnly || p.Live() || len(r.BroadcastHints) != 1 {
		t.Fatal("unmined observation not bounded")
	}
	txLookupSeam = func(context.Context, string) (int64, bool, error) { return 1, true, nil }
	sweepBroadcastHints(context.Background(), s, now.Add(15*time.Minute))
	r, p, ok = s.Proposal("0", hintTx(0))
	if !ok || p.Status != ProposalConfirmed || len(r.BroadcastHints) != 0 {
		t.Fatal("confirmation did not retire hint")
	}
	admitHint(t, s, "0", 1, "0", now, "processed")
	txLookupSeam = func(context.Context, string) (int64, bool, error) { return 0, true, nil }
	sweepBroadcastHints(context.Background(), s, now.Add(30*time.Minute))
	sweepBroadcastHints(context.Background(), s, now.Add(hintLifetime))
	r, p, ok = s.Proposal("0", hintTx(1))
	if !ok || p.Status != ProposalBroadcast || len(r.BroadcastHints) != 0 {
		t.Fatal("expiry lost observed history or retained polling")
	}
}

func TestBroadcastHintLookupCannotCrossWalletSwitch(t *testing.T) {
	s := hintTestStore(t, 1, 1)
	now := time.Now()
	var generation uint64
	walletGenerationSeam = func() uint64 { return generation }
	admitHint(t, s, "0", 0, "0", now, "processed")
	txLookupSeam = func(context.Context, string) (int64, bool, error) { generation += 2; return 1, true, nil }
	sweepBroadcastHints(context.Background(), s, now)
	if _, _, ok := s.Proposal("0", hintTx(0)); ok {
		t.Fatal("accepted result across switch away and back")
	}
}

func TestBroadcastHintConcurrentAdmissionAndFailedWrite(t *testing.T) {
	s := hintTestStore(t, 1, 1)
	now := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.admitBroadcastHint("0", hintTx(i), "0", "0", now)
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	r, _ := s.Wallet("0")
	if len(r.BroadcastHints) != hintMemberLimit {
		t.Fatalf("concurrent cap: %d", len(r.BroadcastHints))
	}
	s = hintTestStore(t, 1, 1)
	before := s.cloneForHintsLocked()
	path := s.path
	s.path = filepath.Join(path, "not-a-directory")
	if _, err := s.admitBroadcastHint("0", hintTx(0), "0", "0", now); err == nil {
		t.Fatal("expected failed write")
	}
	if !reflect.DeepEqual(before, s.data) {
		t.Fatal("failed write consumed state")
	}
	s.path = path
	admitHint(t, s, "0", 0, "0", now, "processed")
}

func TestBroadcastHintMigration(t *testing.T) {
	s := hintTestStore(t, 1, 1)
	now := time.Now()
	r := s.data.Wallets["0"]
	for i := 0; i < 100; i++ {
		r.Proposals[hintTx(i)] = &Proposal{TxID: hintTx(i), Status: ProposalBroadcast, FromUID: "0", CreatedAt: now.Unix() - int64(i), Reason: "claimed visible"}
	}
	r.Proposals[hintTx(101)] = &Proposal{TxID: hintTx(101), Status: ProposalConfirmed}
	r.Proposals[hintTx(102)] = &Proposal{TxID: hintTx(102), Status: ProposalReady, RawTx: "substantive"}
	r.Proposals[hintTx(103)] = &Proposal{TxID: hintTx(103), Status: ProposalBroadcast, FromUID: "0", CreatedAt: now.Add(-hintLifetime).Unix()}
	s.data.SchemaVersion = 3
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	s = reopenHints(t, s)
	r, _ = s.Wallet("0")
	if len(r.BroadcastHints) != hintMemberLimit || len(r.Proposals) != 2 {
		t.Fatalf("migration: %d hints %d proposals", len(r.BroadcastHints), len(r.Proposals))
	}
	if r.BroadcastHints[hintTx(0)] == nil || r.BroadcastHints[hintTx(31)] == nil || r.BroadcastHints[hintTx(32)] != nil {
		t.Fatal("migration did not keep newest bounded hints")
	}
	before, _ := os.ReadFile(s.path)
	s = reopenHints(t, s)
	after, _ := os.ReadFile(s.path)
	if string(before) != string(after) {
		t.Fatal("migration not idempotent")
	}
	var f storeFile
	if err := json.Unmarshal(after, &f); err != nil || f.SchemaVersion != storeSchemaVersion {
		t.Fatal("migration not persisted")
	}
}

func TestBroadcastHintAsyncReplayAndManualRetry(t *testing.T) {
	sh, id := newSpendHarness(t, 2, "alice", "bob")
	sh.as("bob")
	s := sh.store("bob")
	r := sh.record("bob", id)
	alice := sh.nodeByNick("alice")
	now := time.Now()
	for i := 0; i < hintMemberLimit; i++ {
		admitHint(t, s, id, i, alice.uid, now, "processed")
	}
	msg := &Message{Type: TypeBroadcast, WalletID: r.Address, TxID: hintTx(999)}
	payload, err := EncodeMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	mid, _ := NewID()
	body, err := Encode(payload, mid, now.Add(7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// Delayed delivery hits a full queue. It must not consume this MID.
	before, _ := os.ReadFile(s.path)
	handleInbound(alice.uid, alice.nick, body, now.Add(24*time.Hour))
	after, _ := os.ReadFile(s.path)
	if s.SeenMid(mid) || string(before) != string(after) {
		t.Fatal("capacity rejection consumed frame or wrote state")
	}
	// Manual delivery must also ignore the legacy MID journal for notices.
	if err := s.UpdateWallet(id, func(r *WalletRecord) error { r.Transport = TransportManual; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkProcessed(mid, now); err != nil {
		t.Fatal(err)
	}
	result, err := ImportFrame(sh.ctx, body, id, alice.uid, "")
	if err != nil || result.Outcome != "ignored" {
		t.Fatalf("manual full queue: %+v %v", result, err)
	}
	// Simulate completed recovery freeing one pending slot, then reimport exactly
	// the same envelope; its local lifetime starts at this successful admission.
	s.mu.Lock()
	delete(s.data.Wallets[id].BroadcastHints, hintTx(0))
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	result, err = ImportFrame(sh.ctx, body, id, alice.uid, "")
	if err != nil || result.Outcome != "processed" {
		t.Fatalf("manual retry: %+v %v", result, err)
	}
	result, err = ImportFrame(sh.ctx, body, id, alice.uid, "")
	if err != nil || result.Outcome != "duplicate" {
		t.Fatalf("manual duplicate: %+v %v", result, err)
	}
	r, _ = s.Wallet(id)
	h := r.BroadcastHints[msg.TxID]
	if h == nil || h.ReceivedAt < now.Unix() {
		t.Fatal("missing local receipt time")
	}
	// Hundreds of fresh MIDs for the same txid cannot grow the journal/queue.
	before, _ = os.ReadFile(s.path)
	for i := 0; i < 100; i++ {
		mid, _ := NewID()
		frame := &Frame{MID: mid}
		inboundSpend(sh.ctx, manager("simnet"), msg, frame, alice.uid, alice.nick, now)
	}
	after, _ = os.ReadFile(s.path)
	if string(before) != string(after) {
		t.Fatal("fresh-MID duplicates changed state")
	}
}

func TestBroadcastHintKnownProposalAndConcurrentObservation(t *testing.T) {
	s := hintTestStore(t, 1, 1)
	now := time.Now()
	for i, status := range []string{ProposalReady, ProposalInvalid, ProposalAborted, ProposalFailed, ProposalConfirmed} {
		if err := s.UpdateProposal("0", hintTx(i), true, func(_ *WalletRecord, p *Proposal) error { p.Status = status; p.RawTx = "substantive"; return nil }); err != nil {
			t.Fatal(err)
		}
		admitHint(t, s, "0", i, "0", now, "duplicate")
	}
	admitHint(t, s, "0", 100, "0", now, "processed")
	txLookupSeam = func(context.Context, string) (int64, bool, error) {
		if err := s.UpdateProposal("0", hintTx(100), true, func(_ *WalletRecord, p *Proposal) error {
			p.Status = ProposalSigned
			p.RawTx = "concurrent"
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return 1, true, nil
	}
	sweepBroadcastHints(context.Background(), s, now)
	_, p, _ := s.Proposal("0", hintTx(100))
	if p.Status != ProposalSigned || p.RawTx != "concurrent" || p.NoticeOnly {
		t.Fatal("observation overwrote substantive proposal")
	}
}
