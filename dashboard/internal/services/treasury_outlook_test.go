// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package services

import (
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
)

func TestTreasuryOutlookFollowsTheSubsidySchedule(t *testing.T) {
	// The tip five minutes before July 2026, so block 1096768 opens July.
	tipTime := time.Date(2026, time.June, 30, 23, 55, 0, 0, time.UTC).Unix()
	o := treasuryOutlook(chaincfg.MainNetParams(), 1096767, tipTime)
	if len(o.Months) != 12 || o.Months[0].Month != "2026-07" || o.Months[11].Month != "2027-06" || o.TargetBlockSeconds != 300 {
		t.Fatalf("outlook %+v", o)
	}
	// dcrd's getblocksubsidy: 53075235 atoms per block through 1099775, then
	// 52549738 from the reduction at 1099776; July's last block is 1105695.
	jul := o.Months[0]
	if jul.Blocks != 8928 || jul.TBaseAtoms != 3008*53075235+5920*52549738 {
		t.Fatalf("July %+v", jul)
	}
	if aug := o.Months[1]; aug.Blocks != 8928 {
		t.Fatalf("August %+v", aug)
	}
	if feb := o.Months[7]; feb.Month != "2027-02" || feb.Blocks != 8064 {
		t.Fatalf("February %+v", feb)
	}
}

func TestTreasuryRunway(t *testing.T) {
	p := chaincfg.MainNetParams()
	tipTime := time.Date(2026, time.June, 30, 23, 55, 0, 0, time.UTC).Unix()
	// July's block reward from dcrd's getblocksubsidy, as in the outlook test.
	const july = 3008*53075235 + 5920*52549738

	// A month's spend one atom above July's block reward empties an empty
	// treasury in July.
	r := treasuryRunway(p, 1096767, tipTime, 0, july+1)
	if r.Months != 0 || r.ExhaustedMonth != "2026-07" || r.Beyond || r.FirstMonthNetAtoms != -1 {
		t.Fatalf("runway %+v", r)
	}
	// Spending exactly July's block reward leaves nothing, but not less than
	// nothing; August's smaller reward then falls short.
	r = treasuryRunway(p, 1096767, tipTime, 0, july)
	if r.Months != 1 || r.ExhaustedMonth != "2026-08" {
		t.Fatalf("runway %+v", r)
	}
	// Ten million DCR a month from 25 million lasts July and August.
	r = treasuryRunway(p, 1096767, tipTime, 2_500_000_000_000_000, 1_000_000_000_000_000)
	if r.Months != 2 || r.ExhaustedMonth != "2026-09" {
		t.Fatalf("runway %+v", r)
	}
	// One atom a month never runs out within the projection.
	r = treasuryRunway(p, 1096767, tipTime, 0, 1)
	if !r.Beyond || r.Months != 1200 || r.ProjectionMonths != 1200 || r.ExhaustedMonth != "" {
		t.Fatalf("runway %+v", r)
	}
}
