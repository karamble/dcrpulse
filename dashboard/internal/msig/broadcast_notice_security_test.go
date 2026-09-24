// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.

package msig

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Exercise the public message routing boundary with fresh transaction and
// message ids. Merely receiving member claims must not do wallet RPC work,
// create payment records, or grow the general replay journal.
func TestUnobservedBroadcastFloodDoesNotCreateProposals(t *testing.T) {
	sh, id := newSpendHarness(t, 2, "alice", "bob")
	sh.as("bob")
	s := sh.store("bob")
	rec := sh.record("bob", id)
	alice := sh.nodeByNick("alice")
	calls := 0
	txLookupSeam = func(context.Context, string) (int64, bool, error) { calls++; return 0, false, nil }
	beforeMids := len(s.data.ProcessedMids)
	for i := 0; i < 100; i++ {
		payload, err := EncodeMessage(&Message{Type: TypeBroadcast, WalletID: rec.Address, TxID: fmt.Sprintf("%064x", i+1)})
		if err != nil {
			t.Fatal(err)
		}
		mid, _ := NewID()
		body, err := Encode(payload, mid, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		handleInbound(alice.uid, alice.nick, body, time.Now())
	}
	after, _ := s.Wallet(id)
	if len(after.Proposals) != 0 || len(s.data.ProcessedMids) != beforeMids || calls != 0 {
		t.Fatalf("unverified claims caused work: %d proposals, %d journal additions, %d lookups", len(after.Proposals), len(s.data.ProcessedMids)-beforeMids, calls)
	}
}
