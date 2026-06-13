// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package timestamp

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	workerPollEvery = 5 * time.Minute
	verifyBatchSize = 100
)

var workerOnce sync.Once

// StartWorker launches the background poller that advances not-yet-anchored
// records as dcrtime commits them to the chain. Safe to call once at startup;
// subsequent calls no-op. The loop stops when ctx is cancelled.
func StartWorker(ctx context.Context) {
	workerOnce.Do(func() { go workerLoop(ctx) })
}

func workerLoop(ctx context.Context) {
	ticker := time.NewTicker(workerPollEvery)
	defer ticker.Stop()
	RefreshAnchors(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			RefreshAnchors(ctx)
		}
	}
}

// RefreshAnchors verifies every not-yet-anchored digest in one batched dcrtime
// call per chunk and applies the results to the archive. Exposed so a handler
// can trigger an on-demand refresh.
func RefreshAnchors(ctx context.Context) {
	if !Enabled() {
		return
	}
	store, err := Archive()
	if err != nil {
		log.Printf("timestamp worker: open archive: %v", err)
		return
	}
	digests := store.PendingDigests()
	if len(digests) == 0 {
		return
	}
	id := store.ClientID()
	for _, batch := range chunkStrings(digests, verifyBatchSize) {
		results, err := Verify(ctx, id, batch)
		if err != nil {
			log.Printf("timestamp worker: verify batch: %v", err)
			continue
		}
		for digest, res := range results {
			ApplyResult(store, digest, res)
		}
	}
}

// ApplyResult advances a record's status/proof from a dcrtime verify result. It
// is used by both the worker and the submit handler (immediate verify), so the
// state mapping lives in one place.
func ApplyResult(store *Store, digest string, res DigestResult) {
	err := store.Update(digest, func(r *Record) error {
		switch res.State {
		case StateAnchored:
			r.Status = StatusAnchored
			r.AnchorTime = res.AnchorTime
			r.MerkleRoot = res.MerkleRoot
			r.MerklePath = res.MerklePath
			r.TxID = res.TxID
			r.Confirmations = res.Confirmations
			r.MinConfirmations = res.MinConfirmations
			r.FailReason = ""
		case StatePending:
			r.Status = StatusPending
			r.TxID = res.TxID
			r.MerkleRoot = res.MerkleRoot
			r.MerklePath = res.MerklePath
			r.Confirmations = res.Confirmations
			r.MinConfirmations = res.MinConfirmations
			r.FailReason = ""
		case StateAwaiting:
			r.Status = StatusAwaiting
			r.FailReason = ""
		case StateNotFound:
			// dcrtime has not registered the digest yet (propagation delay);
			// leave the record untouched so the next poll retries.
		}
		return nil
	})
	if err != nil && err != ErrNotFound {
		log.Printf("timestamp worker: update %s: %v", digest, err)
	}
}

func chunkStrings(s []string, n int) [][]string {
	if n <= 0 || len(s) <= n {
		return [][]string{s}
	}
	var out [][]string
	for i := 0; i < len(s); i += n {
		end := i + n
		if end > len(s) {
			end = len(s)
		}
		out = append(out, s[i:end])
	}
	return out
}
