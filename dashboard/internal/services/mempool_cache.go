// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/utils"

	"github.com/decred/dcrd/chaincfg/chainhash"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

// A mempool transaction cannot change while it sits in the mempool, but both
// the explorer's mempool page and the node dashboard used to refetch every one
// of them on every poll, separately. The raw reply is cached rather than the
// converted detail: a tspend's voting tally is read live and must not be frozen,
// and the confirmed-transaction path must keep its changing confirmation count.
var mempoolTxCache utils.Memo[string, *chainjson.TxRawResult]

// rawTxSeam is the one dcrd call this file makes. rpc.DcrdClient is a concrete
// client rather than an interface, so a seam is the only way to observe cache
// hits in a test; msig uses the same pattern for its own dependencies.
var rawTxSeam = func(ctx context.Context, hash *chainhash.Hash) (*chainjson.TxRawResult, error) {
	return rpc.DcrdClient.GetRawTransactionVerbose(ctx, hash)
}

// mempoolHashesSeam is the mempool listing call, shared by both pages so there
// is one place dcrd is asked and one place a test can stand in for it.
var mempoolHashesSeam = func(ctx context.Context, txType chainjson.GetRawMempoolTxTypeCmd) ([]*chainhash.Hash, error) {
	return rpc.DcrdClient.GetRawMempool(ctx, txType)
}

// mempoolHashes lists the mempool for one transaction type.
func mempoolHashes(ctx context.Context, txType chainjson.GetRawMempoolTxTypeCmd) ([]*chainhash.Hash, error) {
	return mempoolHashesSeam(ctx, txType)
}

// mempoolRawTx returns a mempool transaction's verbose reply, fetching it only
// the first time this process sees the hash. A failure is not remembered, so a
// transient dcrd error retries on the next poll rather than sticking.
func mempoolRawTx(ctx context.Context, hash *chainhash.Hash) (*chainjson.TxRawResult, error) {
	key := hash.String()
	if tx, ok := mempoolTxCache.Get(key); ok {
		return tx, nil
	}
	tx, err := rawTxSeam(ctx, hash)
	if err != nil {
		return nil, err
	}
	mempoolTxCache.Put(key, tx)
	return tx, nil
}

// retainMempoolTxs drops everything the caller's mempool listing no longer
// names. Callers pass the widest set they hold, so a transaction that was mined
// or evicted stops being cached.
func retainMempoolTxs(hashes ...[]*chainhash.Hash) {
	keep := make(map[string]bool)
	for _, set := range hashes {
		for _, h := range set {
			keep[h.String()] = true
		}
	}
	mempoolTxCache.Retain(keep)
}
