// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.

package msig

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"dcrpulse/internal/services"
	"time"
)

const (
	hintMemberLimit      = 32
	hintWalletLimit      = 128
	hintStoreLimit       = 512
	hintMemberDailyLimit = 256
	hintStoreDailyLimit  = 1024
	hintLookupLimit      = 16
	hintLifetime         = 30 * 24 * time.Hour
	hintLookupWindow     = 15 * time.Minute
)

// BroadcastHint is an untrusted report, not a spend or an input reservation.
// Deadlines use first local receipt, never a peer's clock. Retries and budgets
// survive restarts and manual refreshes.
type BroadcastHint struct {
	TxID        string `json:"txid"`
	FromUID     string `json:"fromUid"`
	FromNick    string `json:"fromNick"`
	ReceivedAt  int64  `json:"receivedAt"`
	NextAttempt int64  `json:"nextAttempt"`
	Attempts    int    `json:"attempts"`
}

type hintBudgets struct {
	Admissions []int64 `json:"admissions,omitempty"`
	Lookups    []int64 `json:"lookups,omitempty"`
}

func recentTimes(times []int64, now int64, window time.Duration) []int64 {
	var out []int64
	for _, ts := range times {
		if ts > now-int64(window/time.Second) {
			out = append(out, ts)
		}
	}
	return out
}

// Clone before writing: a failed disk write must not consume quota or leave
// an in-memory admission that was never made durable.
func (s *Store) cloneForHintsLocked() *storeFile {
	f := *s.data
	f.Wallets = make(map[string]*WalletRecord, len(s.data.Wallets))
	for id, r := range s.data.Wallets {
		f.Wallets[id] = cloneRecord(r)
	}
	f.HintBudgets.Admissions = append([]int64(nil), s.data.HintBudgets.Admissions...)
	f.HintBudgets.Lookups = append([]int64(nil), s.data.HintBudgets.Lookups...)
	return &f
}

func (s *Store) commitHintsLocked(f *storeFile) error {
	if err := atomicWriteJSON(s.path, f); err != nil {
		return err
	}
	s.data = f
	return nil
}

func hintCapacity(f *storeFile, r *WalletRecord, uid string) bool {
	if len(r.BroadcastHints) >= hintWalletLimit {
		return false
	}
	member, total := 0, 0
	for _, h := range r.BroadcastHints {
		if h.FromUID == uid {
			member++
		}
	}
	for _, w := range f.Wallets {
		total += len(w.BroadcastHints)
	}
	return member < hintMemberLimit && total < hintStoreLimit
}

// admitBroadcastHint does not journal MIDs. Rejected notices can be replayed
// after capacity becomes available, including the exact same manual frame.
func (s *Store) admitBroadcastHint(id, txid, uid, nick string, now time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	txid = strings.ToLower(txid)
	r := s.data.Wallets[id]
	if r == nil || r.peerByUID(uid) == nil || !isHexLen(txid, 64) {
		return "ignored", nil
	}
	if r.Proposals[txid] != nil || r.BroadcastHints[txid] != nil {
		return "duplicate", nil
	}
	if !hintCapacity(s.data, r, uid) {
		return "ignored", nil
	}
	ts := now.Unix()
	memberTimes := recentTimes(r.HintAdmissions[uid], ts, 24*time.Hour)
	storeTimes := recentTimes(s.data.HintBudgets.Admissions, ts, 24*time.Hour)
	if len(memberTimes) >= hintMemberDailyLimit || len(storeTimes) >= hintStoreDailyLimit {
		return "ignored", nil
	}
	f := s.cloneForHintsLocked()
	r = f.Wallets[id]
	if r.BroadcastHints == nil {
		r.BroadcastHints = make(map[string]*BroadcastHint)
	}
	if r.HintAdmissions == nil {
		r.HintAdmissions = make(map[string][]int64)
	}
	r.BroadcastHints[txid] = &BroadcastHint{TxID: txid, FromUID: uid, FromNick: r.peerByUID(uid).Nick, ReceivedAt: ts, NextAttempt: ts}
	r.HintAdmissions[uid] = append(memberTimes, ts)
	f.HintBudgets.Admissions = append(storeTimes, ts)
	if err := s.commitHintsLocked(f); err != nil {
		return "ignored", err
	}
	return "processed", nil
}

var hintWorker sync.Mutex
var walletGenerationSeam = services.WalletGeneration

func hintDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 15 * time.Minute
	case 2:
		return time.Hour
	default:
		return 6 * time.Hour
	}
}

type dueHint struct {
	walletID string
	hint     BroadcastHint
}

// reserveHint first removes expired hints, including for inactive wallets.
// It commits a lookup reservation before returning any work to the caller.
func (s *Store) reserveHint(now time.Time, active bool) (*dueHint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts := now.Unix()
	f := s.cloneForHintsLocked()
	changed := false
	var due []dueHint
	for id, r := range f.Wallets {
		for txid, h := range r.BroadcastHints {
			p := r.Proposals[txid]
			if h.ReceivedAt <= 0 || ts >= h.ReceivedAt+int64(hintLifetime/time.Second) || (p != nil && (!p.NoticeOnly || p.Terminal())) {
				delete(r.BroadcastHints, txid)
				changed = true
				continue
			}
			if active && r.Status == StatusActive && h.NextAttempt <= ts {
				due = append(due, dueHint{id, *h})
			}
		}
	}
	sort.Slice(due, func(i, j int) bool {
		a, b := due[i], due[j]
		if a.hint.NextAttempt != b.hint.NextAttempt {
			return a.hint.NextAttempt < b.hint.NextAttempt
		}
		if a.walletID != b.walletID {
			return a.walletID < b.walletID
		}
		return a.hint.TxID < b.hint.TxID
	})
	lookups := recentTimes(f.HintBudgets.Lookups, ts, hintLookupWindow)
	var work *dueHint
	if len(due) > 0 && len(lookups) < hintLookupLimit {
		work = &due[0]
		h := f.Wallets[work.walletID].BroadcastHints[work.hint.TxID]
		h.Attempts++
		h.NextAttempt = ts + int64(hintDelay(h.Attempts)/time.Second)
		work.hint = *h
		f.HintBudgets.Lookups = append(lookups, ts)
		changed = true
	}
	if changed {
		if err := s.commitHintsLocked(f); err != nil {
			return nil, err
		}
	}
	return work, nil
}

func (s *Store) observeHint(work *dueHint, conf int64, now time.Time, generation uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.data.Wallets[work.walletID]
	if r == nil || s.walletName != activeWalletSeam() || generation != walletGenerationSeam() {
		return nil
	}
	h := r.BroadcastHints[work.hint.TxID]
	if h == nil || *h != work.hint || now.Unix() >= h.ReceivedAt+int64(hintLifetime/time.Second) {
		return nil
	}
	existing := r.Proposals[h.TxID]
	if existing != nil && (!existing.NoticeOnly || existing.Terminal()) {
		return nil
	}
	f := s.cloneForHintsLocked()
	r = f.Wallets[work.walletID]
	if r.Proposals == nil {
		r.Proposals = make(map[string]*Proposal)
	}
	p := &Proposal{TxID: h.TxID, Role: RoleCosigner, Status: ProposalBroadcast, NoticeOnly: true,
		FromUID: h.FromUID, FromNick: h.FromNick, CreatedAt: h.ReceivedAt, UpdatedAt: now.Unix()}
	if conf > 0 {
		p.Status = ProposalConfirmed
		delete(r.BroadcastHints, h.TxID)
	}
	r.Proposals[h.TxID] = p
	r.UpdatedAt = now.Unix()
	if err := s.commitHintsLocked(f); err != nil {
		return err
	}
	notifyRecordChanged(r)
	return nil
}

func sweepBroadcastHints(ctx context.Context, store *Store, now time.Time) {
	if !hintWorker.TryLock() {
		return
	}
	defer hintWorker.Unlock()
	for i := 0; i < hintLookupLimit; i++ {
		generation := walletGenerationSeam()
		work, err := store.reserveHint(now, store.WalletName() == activeWalletSeam())
		if err != nil {
			msigLog.Warnf("broadcast hints: %v", err)
			return
		}
		if work == nil {
			return
		}
		if store.WalletName() != activeWalletSeam() || generation != walletGenerationSeam() {
			return
		}
		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		conf, seen, err := txLookupSeam(lookupCtx, work.hint.TxID)
		cancel()
		if err == nil && seen && store.WalletName() == activeWalletSeam() {
			if err := store.observeHint(work, conf, time.Now(), generation); err != nil {
				msigLog.Warnf("broadcast observation: %v", err)
			}
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// Old inputless notices were live proposals forever. Keep only bounded,
// unexpired recovery hints, newest first. A peer's old Reason text is not
// evidence of wallet visibility. Confirmed and substantive records survive.
func migrateBroadcastHints(f *storeFile, now time.Time) {
	var old []dueHint
	for id, r := range f.Wallets {
		for txid, p := range r.Proposals {
			if p.Terminal() || p.RawTx != "" || len(p.Inputs) > 0 {
				continue
			}
			delete(r.Proposals, txid)
			if p.CreatedAt <= 0 || p.CreatedAt > now.Unix() || now.Unix() >= p.CreatedAt+int64(hintLifetime/time.Second) || r.peerByUID(p.FromUID) == nil || !isHexLen(txid, 64) {
				continue
			}
			old = append(old, dueHint{id, BroadcastHint{TxID: txid, FromUID: p.FromUID, FromNick: p.FromNick, ReceivedAt: p.CreatedAt, NextAttempt: now.Unix()}})
		}
	}
	sort.Slice(old, func(i, j int) bool {
		if old[i].hint.ReceivedAt != old[j].hint.ReceivedAt {
			return old[i].hint.ReceivedAt > old[j].hint.ReceivedAt
		}
		return fmt.Sprint(old[i].walletID, old[i].hint.TxID) < fmt.Sprint(old[j].walletID, old[j].hint.TxID)
	})
	for _, item := range old {
		r := f.Wallets[item.walletID]
		if !hintCapacity(f, r, item.hint.FromUID) {
			continue
		}
		if r.BroadcastHints == nil {
			r.BroadcastHints = make(map[string]*BroadcastHint)
		}
		h := item.hint
		r.BroadcastHints[h.TxID] = &h
	}
}

func isHexLen(s string, n int) bool {
	if len(s) != n {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
