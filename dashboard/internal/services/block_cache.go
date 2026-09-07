// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
	"dcrpulse/internal/utils"

	"github.com/decred/dcrd/chaincfg/chainhash"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

// A block below the tip cannot change, but the explorer landing used to refetch
// every block it lists on every poll. Summaries are cached by hash, which is
// what makes a reorg safe: a block that left the main chain is never reached by
// a walk from the tip and simply ages out. Confirmations are not stored; they
// are the tip's distance and are set by the reader.
var blockSummaryCache utils.Memo[string, types.BlockSummary]

// blockCacheDepth bounds the cache to the most recent blocks; deeper history
// is fetched again if it is browsed again.
const blockCacheDepth = 1000

// The dcrd calls this file makes, as seams: rpc.DcrdClient is a concrete
// client, so a seam is the only way a test can count fetches (see mempool_cache).
var (
	blockCountSeam = func(ctx context.Context) (int64, error) { return rpc.DcrdClient.GetBlockCount(ctx) }
	blockHashSeam  = func(ctx context.Context, height int64) (*chainhash.Hash, error) {
		return rpc.DcrdClient.GetBlockHash(ctx, height)
	}
	// verbosetx stays off: a count does not need the transactions themselves.
	blockVerboseSeam = func(ctx context.Context, hash *chainhash.Hash) (*chainjson.GetBlockVerboseResult, error) {
		return rpc.DcrdClient.GetBlockVerbose(ctx, hash, false)
	}
)

// blockSummaryByHash returns a block's summary, fetching it only the first time
// this process sees the hash. A failure is not remembered.
func blockSummaryByHash(ctx context.Context, hash *chainhash.Hash) (types.BlockSummary, error) {
	key := hash.String()
	if s, ok := blockSummaryCache.Get(key); ok {
		return s, nil
	}
	block, err := blockVerboseSeam(ctx, hash)
	if err != nil {
		return types.BlockSummary{}, err
	}
	s := types.BlockSummary{
		Height:       block.Height,
		Hash:         block.Hash,
		PreviousHash: block.PreviousHash,
		Timestamp:    time.Unix(block.Time, 0),
		TxCount:      len(block.Tx) + len(block.STx),
		Size:         int64(block.Size),
		Difficulty:   block.Difficulty,
	}
	blockSummaryCache.Put(key, s)
	return s, nil
}

// retainRecentBlocks drops summaries deeper than blockCacheDepth below tip.
func retainRecentBlocks(tip int64) {
	blockSummaryCache.RetainWhere(func(_ string, s types.BlockSummary) bool {
		return s.Height > tip-blockCacheDepth
	})
}
