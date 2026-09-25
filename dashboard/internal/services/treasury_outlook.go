// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/v3"
)

// treasuryOutlookMonths is how far the block-reward outlook reaches.
const treasuryOutlookMonths = 12

// treasuryRunwayMaxMonths is how far the runway projection looks.
const treasuryRunwayMaxMonths = 1200

// projectTreasuryBase is the treasury's block reward per calendar month after
// the tip, with blocks at the target block time and dcrd's subsidy schedule.
func projectTreasuryBase(p *chaincfg.Params, tip, tipTime int64, months int) []types.TreasuryOutlookMonth {
	subsidy := standalone.NewSubsidyCache(p)
	target := int64(p.TargetTimePerBlock / time.Second)
	t := time.Unix(tipTime, 0).UTC()
	start := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)

	out := make([]types.TreasuryOutlookMonth, 0, months)
	h := tip + 1
	for m := 0; m < months; m++ {
		from := start.AddDate(0, m, 0).Unix()
		to := start.AddDate(0, m+1, 0).Unix()
		month := types.TreasuryOutlookMonth{Month: start.AddDate(0, m, 0).Format("2006-01")}
		for ; tipTime+(h-tip)*target < to; h++ {
			if tipTime+(h-tip)*target < from {
				continue
			}
			month.Blocks++
			month.TBaseAtoms += subsidy.CalcTreasurySubsidy(h, p.TicketsPerBlock, true)
		}
		out = append(out, month)
	}
	return out
}

// treasuryOutlook projects the treasury's block reward for the months after
// the tip.
func treasuryOutlook(p *chaincfg.Params, tip, tipTime int64) *types.TreasuryOutlook {
	return &types.TreasuryOutlook{
		FromHeight:         tip,
		TargetBlockSeconds: int64(p.TargetTimePerBlock / time.Second),
		Months:             projectTreasuryBase(p, tip, tipTime, treasuryOutlookMonths),
	}
}

// treasuryRunway counts the whole calendar months after the tip the balance
// lasts, each month adding its projected block reward and paying a flat
// monthly spend.
func treasuryRunway(p *chaincfg.Params, tip, tipTime, balance, monthlySpend int64) *types.TreasuryRunway {
	months := projectTreasuryBase(p, tip, tipTime, treasuryRunwayMaxMonths)
	r := &types.TreasuryRunway{
		FromHeight:         tip,
		BalanceAtoms:       balance,
		MonthlySpendAtoms:  monthlySpend,
		FirstMonthNetAtoms: months[0].TBaseAtoms - monthlySpend,
		TargetBlockSeconds: int64(p.TargetTimePerBlock / time.Second),
		ProjectionMonths:   treasuryRunwayMaxMonths,
	}
	left := balance
	for i, m := range months {
		left += m.TBaseAtoms - monthlySpend
		if left < 0 {
			r.Months = i
			r.ExhaustedMonth = m.Month
			return r
		}
	}
	r.Months = len(months)
	r.Beyond = true
	return r
}

// bestTreasuryTip returns the tip height, its time and the treasury balance.
func bestTreasuryTip(ctx context.Context) (*chaincfg.Params, int64, int64, int64, error) {
	if rpc.DcrdClient == nil {
		return nil, 0, 0, 0, fmt.Errorf("dcrd client not available")
	}
	p, err := chainParams(ctx)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	hash, tip, err := rpc.DcrdClient.GetBestBlock(ctx)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("get best block: %w", err)
	}
	hdr, err := rpc.DcrdClient.GetBlockHeader(ctx, hash)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("get block header: %w", err)
	}
	bal, err := rpc.DcrdClient.GetTreasuryBalance(ctx, hash, false)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("gettreasurybalance: %w", err)
	}
	return p, tip, hdr.Timestamp.Unix(), int64(bal.Balance), nil
}

// TreasuryRunway returns how long the current treasury balance lasts at the
// given monthly spend, with the block reward following dcrd's schedule.
func TreasuryRunway(ctx context.Context, monthlySpend int64) (*types.TreasuryRunway, error) {
	p, tip, tipTime, balance, err := bestTreasuryTip(ctx)
	if err != nil {
		return nil, err
	}
	return treasuryRunway(p, tip, tipTime, balance, monthlySpend), nil
}

// TreasuryOutlook returns the projected block reward for the next twelve
// calendar months.
func TreasuryOutlook(ctx context.Context) (*types.TreasuryOutlook, error) {
	p, tip, tipTime, _, err := bestTreasuryTip(ctx)
	if err != nil {
		return nil, err
	}
	return treasuryOutlook(p, tip, tipTime), nil
}
