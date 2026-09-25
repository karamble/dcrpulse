// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package services

import (
	"context"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
)

func TestTreasurySpendLimitFloorAndWindowOnMainnet(t *testing.T) {
	p := chaincfg.MainNetParams()
	// (3119582664 / 10) * (288 * 12)
	if got := treasurySpendLimitFloor(p); got != 1_078_127_767_296 {
		t.Fatalf("floor %d", got)
	}
	// 288 * 12 * 2
	if got := treasuryPolicyWindow(p); got != 6912 {
		t.Fatalf("window %d", got)
	}
}

func TestMaxTreasuryExpenditureDCP0013(t *testing.T) {
	const floor = 1_078_127_767_296
	for _, tc := range []struct {
		name                  string
		balance, spent, floor int64
		max, allowed          int64
	}{
		{"four percent", 88_000_000_000_000, 0, floor, 3_520_000_000_000, 3_520_000_000_000},
		{"less what the window spent", 85_000_000_000_000, 3_000_000_000_000, floor, 3_520_000_000_000, 520_000_000_000},
		{"floor", 10_000_000_000_000, 0, floor, floor, floor},
		{"window already over", 50_000_000_000_000, 3_000_000_000_000, floor, 2_120_000_000_000, 0},
		{"capped at the balance", 500_000_000_000, 0, floor, floor, 500_000_000_000},
		{"multiplies before dividing", 1_234_567_891_234, 0, 0, 49_382_715_649, 49_382_715_649},
	} {
		t.Run(tc.name, func(t *testing.T) {
			max, allowed := maxTreasuryExpenditureDCP0013(tc.balance, tc.spent, tc.floor)
			if max != tc.max || allowed != tc.allowed {
				t.Fatalf("got max %d allowed %d, want %d and %d", max, allowed, tc.max, tc.allowed)
			}
		})
	}
}

func TestTreasurySpendLimitAtSumsTheWindowAndTheMaturingBlock(t *testing.T) {
	p := chaincfg.MainNetParams()
	const n = 1_100_000
	read := map[int64]int{}
	updatesAt := func(_ context.Context, h int64) ([]int64, error) {
		read[h]++
		switch h {
		case n - 6911: // oldest block in the window
			return []int64{50_000_000, -1_000_000_000_000, -10_000}, nil
		case n - 6912: // just outside it
			return []int64{50_000_000, -7_777}, nil
		case n - 255: // matures in the block after n
			return []int64{50_000_000, 2_000_000_000}, nil
		}
		return []int64{50_000_000}, nil
	}
	l, err := treasurySpendLimitAt(context.Background(), p, n, 88_000_000_000_000, updatesAt)
	if err != nil {
		t.Fatal(err)
	}
	if l.SpentInWindowAtoms != 1_000_000_010_000 || l.BalanceAtoms != 88_002_050_000_000 ||
		l.MaxSpendableAtoms != 3_560_082_000_400 || l.AllowedAtoms != 2_560_081_990_400 {
		t.Fatalf("limit %+v", l)
	}
	if l.NextTVI != 1_100_160 || l.AtTVI || l.Height != n || !l.Active || l.PolicyWindowBlocks != 6912 {
		t.Fatalf("limit %+v", l)
	}
	if read[n-6912] != 0 || read[n-6911] != 1 || read[n] != 1 {
		t.Fatalf("window read %d/%d/%d times", read[n-6912], read[n-6911], read[n])
	}

	at, err := treasurySpendLimitAt(context.Background(), p, 1_100_159, 0, updatesAt)
	if err != nil {
		t.Fatal(err)
	}
	if !at.AtTVI || at.NextTVI != 1_100_160 {
		t.Fatalf("pre-TVI block %+v", at)
	}
}

func TestTreasurySpendLimitAtStopsWhereTheTreasuryBegins(t *testing.T) {
	p := chaincfg.MainNetParams()
	n := int64(TreasuryActivationHeight + 100)
	updatesAt := func(_ context.Context, h int64) ([]int64, error) {
		if h < TreasuryActivationHeight {
			t.Fatalf("read block %d before the treasury", h)
		}
		return []int64{-1}, nil
	}
	l, err := treasurySpendLimitAt(context.Background(), p, n, 5_000_000_000, updatesAt)
	if err != nil {
		t.Fatal(err)
	}
	// 101 blocks with one atom spent each; the maturing block is before the
	// treasury, so dcrd counts the balance as zero.
	if l.SpentInWindowAtoms != 101 || l.BalanceAtoms != 0 || l.AllowedAtoms != 0 {
		t.Fatalf("limit %+v", l)
	}
}
