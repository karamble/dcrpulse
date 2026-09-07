// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

// countingSeam replaces the one dcrd call this cache makes and records how
// often it ran, which is the whole point of the cache.
func countingSeam(t *testing.T, err error) (fetches func() int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	prev := rawTxSeam
	rawTxSeam = func(_ context.Context, h *chainhash.Hash) (*chainjson.TxRawResult, error) {
		mu.Lock()
		n++
		mu.Unlock()
		if err != nil {
			return nil, err
		}
		return &chainjson.TxRawResult{Txid: h.String()}, nil
	}
	t.Cleanup(func() {
		rawTxSeam = prev
		mempoolTxCache.Retain(nil)
	})
	mempoolTxCache.Retain(nil)
	return func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

func hashN(t *testing.T, n int) *chainhash.Hash {
	t.Helper()
	h, err := chainhash.NewHashFromStr(fmt.Sprintf("%064x", n))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// A mempool transaction cannot change while it is in the mempool, so the second
// poll must cost nothing. The count is reported rather than only asserted, so
// the saving is a number in the output instead of a claim.
func TestMempoolRawTxFetchesOncePerHash(t *testing.T) {
	fetches := countingSeam(t, nil)

	const pollSize, polls = 40, 5
	for p := 0; p < polls; p++ {
		for i := 0; i < pollSize; i++ {
			if _, err := mempoolRawTx(context.Background(), hashN(t, i)); err != nil {
				t.Fatalf("poll %d hash %d: %v", p, i, err)
			}
		}
	}

	want := pollSize // one per distinct hash, regardless of how many polls
	if got := fetches(); got != want {
		t.Fatalf("%d fetches for %d hashes over %d polls, want %d", got, pollSize, polls, want)
	}
	t.Logf("%d hashes over %d polls: %d fetches, was %d", pollSize, polls, fetches(), pollSize*polls)
}

// The prune is what bounds the cache, and it has to actually reach the entries:
// a hash that leaves the mempool must be fetched afresh if it comes back.
func TestMempoolRawTxRefetchesAfterEviction(t *testing.T) {
	fetches := countingSeam(t, nil)
	gone, stays := hashN(t, 1), hashN(t, 2)

	for _, h := range []*chainhash.Hash{gone, stays} {
		if _, err := mempoolRawTx(context.Background(), h); err != nil {
			t.Fatal(err)
		}
	}
	if fetches() != 2 {
		t.Fatalf("warm-up did %d fetches, want 2", fetches())
	}

	// Only "stays" is still listed, so "gone" must be dropped.
	retainMempoolTxs([]*chainhash.Hash{stays})

	if _, err := mempoolRawTx(context.Background(), stays); err != nil {
		t.Fatal(err)
	}
	if fetches() != 2 {
		t.Fatalf("a retained hash was refetched: %d fetches, want 2", fetches())
	}
	if _, err := mempoolRawTx(context.Background(), gone); err != nil {
		t.Fatal(err)
	}
	if fetches() != 3 {
		t.Fatalf("an evicted hash was served from cache: %d fetches, want 3", fetches())
	}
}

// A dcrd hiccup must not be remembered, or one bad poll would blank a
// transaction until it leaves the mempool.
func TestMempoolRawTxDoesNotCacheErrors(t *testing.T) {
	fetches := countingSeam(t, fmt.Errorf("dcrd is having a moment"))
	h := hashN(t, 7)

	for i := 0; i < 3; i++ {
		if _, err := mempoolRawTx(context.Background(), h); err == nil {
			t.Fatal("a failing fetch reported success")
		}
	}
	if got := fetches(); got != 3 {
		t.Fatalf("%d fetches after 3 failures, want 3: a failure was cached", got)
	}
	if mempoolTxCache.Len() != 0 {
		t.Fatalf("the cache holds %d entries after only failures", mempoolTxCache.Len())
	}
}

// The prune only bounds the cache if a poll actually calls it. This drives the
// node dashboard's real mempool pass with both dcrd calls stubbed and asserts
// that a transaction which left the mempool is no longer held.
func TestNodeMempoolPassPrunesTheCache(t *testing.T) {
	countingSeam(t, nil)

	stays, gone := hashN(t, 11), hashN(t, 12)
	prev := mempoolHashesSeam
	mempoolHashesSeam = func(_ context.Context, txType chainjson.GetRawMempoolTxTypeCmd) ([]*chainhash.Hash, error) {
		if txType == chainjson.GRMRegular {
			return []*chainhash.Hash{stays}, nil
		}
		return nil, nil
	}
	t.Cleanup(func() { mempoolHashesSeam = prev })

	// Both are cached, as if an earlier poll had seen them.
	for _, h := range []*chainhash.Hash{stays, gone} {
		if _, err := mempoolRawTx(context.Background(), h); err != nil {
			t.Fatal(err)
		}
	}
	if mempoolTxCache.Len() != 2 {
		t.Fatalf("warm-up left %d entries, want 2", mempoolTxCache.Len())
	}

	analyzeMempoolTransactions(context.Background())

	if _, ok := mempoolTxCache.Get(gone.String()); ok {
		t.Error("a transaction no longer in the mempool is still cached: the poll did not prune")
	}
	if _, ok := mempoolTxCache.Get(stays.String()); !ok {
		t.Error("a transaction still in the mempool was dropped")
	}
}
