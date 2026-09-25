// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"slices"
	"time"

	"dcrpulse/internal/types"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/rpcclient/v8"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
)

// treasuryFlows is what one block paid into and out of the treasury.
type treasuryFlows struct {
	time    int64
	tbase   int64
	tspends []types.TSpendHistory
	tadds   []types.TreasuryTAdd
}

// treasuryBlockFlows splits a block's treasury updates, as dcrd's
// gettreasurybalance reports them, into block reward, contributions and
// spends. A block with more than one update must come with its transactions,
// which are classified with dcrd's own stake rules and must reproduce the
// updates exactly.
func treasuryBlockFlows(height int64, hash string, blockTime int64, updates []int64,
	block *wire.MsgBlock, params stdaddr.AddressParamsV0) (*treasuryFlows, error) {

	f := &treasuryFlows{time: blockTime}
	if len(updates) == 1 && block == nil {
		if updates[0] <= 0 {
			return nil, fmt.Errorf("block %d: single treasury update %d is not a treasurybase", height, updates[0])
		}
		f.tbase = updates[0]
		return f, nil
	}
	if block == nil {
		return nil, fmt.Errorf("block %d: %d treasury updates need the block's transactions", height, len(updates))
	}

	at := time.Unix(blockTime, 0)
	var want []int64
	for _, tx := range block.STransactions {
		switch {
		case stake.IsTAdd(tx):
			want = append(want, tx.TxOut[0].Value)
			f.tadds = append(f.tadds, types.TreasuryTAdd{
				TxHash:      tx.TxHash().String(),
				AmountAtoms: tx.TxOut[0].Value,
				BlockHeight: height,
				BlockHash:   hash,
				Timestamp:   at,
			})
		case stake.IsTreasuryBase(tx):
			want = append(want, tx.TxOut[0].Value)
			f.tbase += tx.TxOut[0].Value
		case stake.IsTSpend(tx):
			var paid int64
			payee := ""
			for _, out := range tx.TxOut[1:] {
				want = append(want, -out.Value)
				paid += out.Value
				if _, addrs := stdscript.ExtractAddrs(out.Version, out.PkScript, params); len(addrs) > 0 {
					payee = addrs[0].String()
				}
			}
			fee := tx.TxIn[0].ValueIn - paid
			want = append(want, -fee)
			f.tspends = append(f.tspends, types.TSpendHistory{
				TxHash:      tx.TxHash().String(),
				Amount:      dcrutil.Amount(paid).ToCoin(),
				AmountAtoms: paid,
				FeeAtoms:    fee,
				Payee:       payee,
				BlockHeight: height,
				BlockHash:   hash,
				Timestamp:   at,
				VoteResult:  "approved",
			})
		}
	}
	if !slices.Equal(want, updates) {
		return nil, fmt.Errorf("block %d: its transactions give treasury updates %v, dcrd reports %v", height, want, updates)
	}
	return f, nil
}

// readTreasuryFlows reads one block's treasury updates and, when there is more
// than the treasurybase, the block itself.
func readTreasuryFlows(ctx context.Context, client *rpcclient.Client, height int64,
	params stdaddr.AddressParamsV0) (*treasuryFlows, error) {

	hash, err := client.GetBlockHash(ctx, height)
	if err != nil {
		return nil, fmt.Errorf("get block hash: %w", err)
	}
	bal, err := client.GetTreasuryBalance(ctx, hash, true)
	if err != nil {
		return nil, fmt.Errorf("gettreasurybalance %s: %w", hash, err)
	}
	if len(bal.Updates) == 1 {
		hdr, err := client.GetBlockHeader(ctx, hash)
		if err != nil {
			return nil, fmt.Errorf("getblockheader %s: %w", hash, err)
		}
		return treasuryBlockFlows(height, hash.String(), hdr.Timestamp.Unix(), bal.Updates, nil, params)
	}
	block, err := client.GetBlock(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("getblock %s: %w", hash, err)
	}
	return treasuryBlockFlows(height, hash.String(), block.Header.Timestamp.Unix(), bal.Updates, block, params)
}

// readTreasuryFlowsRetry retries readTreasuryFlows, so that a transient dcrd
// failure does not end the scan.
func readTreasuryFlowsRetry(ctx context.Context, client *rpcclient.Client, height int64,
	params stdaddr.AddressParamsV0) (*treasuryFlows, error) {

	var err error
	for attempt := 1; attempt <= scanBlockAttempts; attempt++ {
		var f *treasuryFlows
		if f, err = readTreasuryFlows(ctx, client, height, params); err == nil {
			return f, nil
		}
		govnLog.Warnf("Failed to read treasury flows of block %d (attempt %d/%d): %v", height, attempt, scanBlockAttempts, err)
		if attempt < scanBlockAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(scanBlockRetryDelay):
			}
		}
	}
	return nil, err
}

// flowMonth is the UTC month a block's flows count towards.
func flowMonth(blockTime int64) string {
	return time.Unix(blockTime, 0).UTC().Format("2006-01")
}
