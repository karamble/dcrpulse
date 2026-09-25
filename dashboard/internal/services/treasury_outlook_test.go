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
