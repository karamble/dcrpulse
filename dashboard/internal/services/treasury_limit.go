// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"sync"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	"github.com/decred/dcrd/chaincfg/v3"
)

// The treasury spend limit mirrors dcrd's internal/blockchain
// maxTreasuryExpenditureDCP0013, calculateTreasuryBalance and
// sumPastTreasuryChanges, which no RPC exposes. Keep it identical to them.

// maxTreasuryExpenditureDCP0013 is dcrd's rule for what the treasury may spend
// in the block after a pre-TVI block, given the treasury balance as of that
// next block and what the policy window already spent.
func maxTreasuryExpenditureDCP0013(treasuryBalance, spentRecent, floor int64) (maxSpendable, allowed int64) {
	maxSpendable = (treasuryBalance + spentRecent) * 4 / 100
	if maxSpendable < floor {
		maxSpendable = floor
	}
	if maxSpendable > spentRecent {
		allowed = maxSpendable - spentRecent
	}
	if allowed > treasuryBalance {
		allowed = treasuryBalance
	}
	return maxSpendable, allowed
}

// treasurySpendLimitFloor is dcrd's treasurySpendLimitFloor.
func treasurySpendLimitFloor(p *chaincfg.Params) int64 {
	return (p.BaseSubsidy / 10) * int64(p.TreasuryVoteInterval*p.TreasuryVoteIntervalMultiplier)
}

// treasuryPolicyWindow is the number of blocks dcrd sums spends over.
func treasuryPolicyWindow(p *chaincfg.Params) uint64 {
	return p.TreasuryVoteInterval * p.TreasuryVoteIntervalMultiplier * p.TreasuryExpenditureWindow
}

// treasuryUpdatesAt returns the treasury updates dcrd recorded for a block.
type treasuryUpdatesAt func(ctx context.Context, height int64) ([]int64, error)

// treasurySpendLimitAt computes the DCP-0013 limit for the block after height n,
// whose own treasury balance is balance.
func treasurySpendLimitAt(ctx context.Context, p *chaincfg.Params, n, balance int64, updatesAt treasuryUpdatesAt) (*types.TreasurySpendLimit, error) {
	window := treasuryPolicyWindow(p)

	// sumPastTreasuryChanges: every debit over the window ending at n, walking
	// back until the window is full or the treasury records end.
	var spent int64
	h := n
	for i := uint64(0); i < window && h >= TreasuryActivationHeight; i++ {
		updates, err := updatesAt(ctx, h)
		if err != nil {
			return nil, fmt.Errorf("treasury updates at %d: %w", h, err)
		}
		for _, u := range updates {
			if u < 0 {
				spent += -u
			}
		}
		h--
	}

	// calculateTreasuryBalance: the balance at n plus everything maturing in
	// the next block, which was recorded CoinbaseMaturity-1 blocks back.
	var treasuryBalance int64
	if want := n - int64(p.CoinbaseMaturity-1); want >= TreasuryActivationHeight {
		updates, err := updatesAt(ctx, want)
		if err != nil {
			return nil, fmt.Errorf("treasury updates at %d: %w", want, err)
		}
		var net int64
		for _, u := range updates {
			net += u
		}
		treasuryBalance = balance + net
	}

	floor := treasurySpendLimitFloor(p)
	maxSpendable, allowed := maxTreasuryExpenditureDCP0013(treasuryBalance, spent, floor)
	tvi := int64(p.TreasuryVoteInterval)
	return &types.TreasurySpendLimit{
		Active:             true,
		Height:             n,
		NextTVI:            (n/tvi + 1) * tvi,
		AtTVI:              (n+1)%tvi == 0,
		PolicyWindowBlocks: int64(window),
		SpentInWindowAtoms: spent,
		BalanceAtoms:       treasuryBalance,
		MaxSpendableAtoms:  maxSpendable,
		FloorAtoms:         floor,
		AllowedAtoms:       allowed,
	}, nil
}

// Treasury updates by height, kept with the block hash they belong to so a
// reorganised block is read again.
var (
	treasuryUpdatesMu    sync.Mutex
	treasuryUpdatesCache = map[int64]cachedTreasuryUpdates{}
)

type cachedTreasuryUpdates struct {
	hash    string
	updates []int64
}

// TreasurySpendLimit returns what dcrd's DCP-0013 rule would let the treasury
// spend if the block after the current tip were a TVI block.
func TreasurySpendLimit(ctx context.Context) (*types.TreasurySpendLimit, error) {
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}
	p, err := chainParams(ctx)
	if err != nil {
		return nil, err
	}
	info, err := rpc.DcrdClient.GetBlockChainInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("get blockchain info: %w", err)
	}
	if info.Deployments[chaincfg.VoteIDMaxTreasurySpend].Status != "active" {
		return &types.TreasurySpendLimit{}, nil
	}
	tipHash, err := rpc.DcrdClient.GetBlockHash(ctx, info.Blocks)
	if err != nil {
		return nil, fmt.Errorf("get block hash: %w", err)
	}
	bal, err := rpc.DcrdClient.GetTreasuryBalance(ctx, tipHash, false)
	if err != nil {
		return nil, fmt.Errorf("gettreasurybalance: %w", err)
	}

	treasuryUpdatesMu.Lock()
	defer treasuryUpdatesMu.Unlock()
	if err := refreshTreasuryUpdates(ctx, info.Blocks, int64(treasuryPolicyWindow(p))); err != nil {
		// A partly refreshed cache would stop the next walk too early.
		treasuryUpdatesCache = map[int64]cachedTreasuryUpdates{}
		return nil, err
	}
	return treasurySpendLimitAt(ctx, p, info.Blocks, int64(bal.Balance),
		func(_ context.Context, h int64) ([]int64, error) {
			c, ok := treasuryUpdatesCache[h]
			if !ok {
				return nil, fmt.Errorf("no treasury updates read for block %d", h)
			}
			return c.updates, nil
		})
}

// refreshTreasuryUpdates reads the updates of the window blocks ending at tip.
// It walks down from the tip and stops at the first block whose hash matches
// the cache, since every block below it is then unchanged too.
func refreshTreasuryUpdates(ctx context.Context, tip, window int64) error {
	low := tip - window + 1
	if low < TreasuryActivationHeight {
		low = TreasuryActivationHeight
	}
	for h := tip; h >= low; h-- {
		hash, err := rpc.DcrdClient.GetBlockHash(ctx, h)
		if err != nil {
			return fmt.Errorf("get block hash %d: %w", h, err)
		}
		if c, ok := treasuryUpdatesCache[h]; ok && c.hash == hash.String() {
			break
		}
		bal, err := rpc.DcrdClient.GetTreasuryBalance(ctx, hash, true)
		if err != nil {
			return fmt.Errorf("gettreasurybalance %d: %w", h, err)
		}
		treasuryUpdatesCache[h] = cachedTreasuryUpdates{hash: hash.String(), updates: bal.Updates}
	}
	for h := range treasuryUpdatesCache {
		if h < low || h > tip {
			delete(treasuryUpdatesCache, h)
		}
	}
	return nil
}
