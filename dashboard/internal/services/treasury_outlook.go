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

// treasuryOutlook projects the treasury's block reward for the months after
// the tip, with blocks at the target block time and dcrd's subsidy schedule.
func treasuryOutlook(p *chaincfg.Params, tip, tipTime int64) *types.TreasuryOutlook {
	subsidy := standalone.NewSubsidyCache(p)
	target := int64(p.TargetTimePerBlock / time.Second)
	t := time.Unix(tipTime, 0).UTC()
	start := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)

	out := &types.TreasuryOutlook{FromHeight: tip, TargetBlockSeconds: target}
	h := tip + 1
	for m := 0; m < treasuryOutlookMonths; m++ {
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
		out.Months = append(out.Months, month)
	}
	return out
}

// TreasuryOutlook returns the projected block reward for the next twelve
// calendar months.
func TreasuryOutlook(ctx context.Context) (*types.TreasuryOutlook, error) {
	if rpc.DcrdClient == nil {
		return nil, fmt.Errorf("dcrd client not available")
	}
	p, err := chainParams(ctx)
	if err != nil {
		return nil, err
	}
	hash, tip, err := rpc.DcrdClient.GetBestBlock(ctx)
	if err != nil {
		return nil, fmt.Errorf("get best block: %w", err)
	}
	hdr, err := rpc.DcrdClient.GetBlockHeader(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("get block header: %w", err)
	}
	return treasuryOutlook(p, tip, hdr.Timestamp.Unix()), nil
}
