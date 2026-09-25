// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"dcrpulse/internal/types"
)

// monthChain holds one block time per height from the activation block on.
type monthChain struct {
	times []int64
	calls int
}

func (c *monthChain) timeAt(_ context.Context, h int64) (int64, error) {
	c.calls++
	i := h - TreasuryActivationHeight
	if i < 0 || i >= int64(len(c.times)) {
		return 0, fmt.Errorf("no block %d", h)
	}
	return c.times[i], nil
}

func (c *monthChain) sampleAt(ctx context.Context, h int64) (*types.BalanceSample, error) {
	t, err := c.timeAt(ctx, h)
	if err != nil {
		return nil, err
	}
	return &types.BalanceSample{Height: h, Time: t, Balance: float64(h)}, nil
}

func (c *monthChain) last() int64 { return TreasuryActivationHeight + int64(len(c.times)) - 1 }

func utc(y int, m time.Month, d, hh, mm int) int64 {
	return time.Date(y, m, d, hh, mm, 0, 0, time.UTC).Unix()
}

// newMonthChain mines a block every ten minutes from 2021-05-20, with no block
// for two days across 1 July, one block exactly on 1 August, and one block
// timestamped slightly earlier than the one before it.
func newMonthChain() *monthChain {
	var times []int64
	t := utc(2021, time.May, 20, 12, 3)
	end := utc(2021, time.October, 10, 0, 0)
	for t < end {
		times = append(times, t)
		t += 600
		if t > utc(2021, time.June, 30, 0, 0) && t < utc(2021, time.July, 2, 0, 0) {
			t = utc(2021, time.July, 2, 0, 7)
		}
		if t > utc(2021, time.July, 31, 23, 55) && t < utc(2021, time.August, 1, 0, 0) {
			t = utc(2021, time.August, 1, 0, 0)
		}
	}
	times[100] = times[99] - 30
	return &monthChain{times: times}
}

// wantMonthStarts is the first height at or after each month start, found by
// reading every block.
func wantMonthStarts(c *monthChain) []int64 {
	want := []int64{TreasuryActivationHeight}
	starts := []int64{
		utc(2021, time.June, 1, 0, 0), utc(2021, time.July, 1, 0, 0),
		utc(2021, time.August, 1, 0, 0), utc(2021, time.September, 1, 0, 0),
		utc(2021, time.October, 1, 0, 0),
	}
	for _, s := range starts {
		for i, t := range c.times {
			if t >= s {
				want = append(want, TreasuryActivationHeight+int64(i))
				break
			}
		}
	}
	return want
}

func heightsOf(s []types.BalanceSample) []int64 {
	out := make([]int64, len(s))
	for i, x := range s {
		out[i] = x.Height
	}
	return out
}

func TestExtendMonthStartsSamplesTheFirstBlockOfEveryMonth(t *testing.T) {
	c := newMonthChain()
	got, err := extendMonthStarts(context.Background(), nil, c.last(), c.sampleAt, c.timeAt)
	if err != nil {
		t.Fatal(err)
	}
	want := wantMonthStarts(c)
	if fmt.Sprint(heightsOf(got)) != fmt.Sprint(want) {
		t.Fatalf("month starts = %v, want %v", heightsOf(got), want)
	}
	aug := got[3]
	if aug.Time != utc(2021, time.August, 1, 0, 0) {
		t.Fatalf("August sample at %s, want the block mined exactly at the month start",
			time.Unix(aug.Time, 0).UTC())
	}
	jul := got[2]
	if jul.Time != utc(2021, time.July, 2, 0, 7) {
		t.Fatalf("July sample at %s, want the first block after the gap", time.Unix(jul.Time, 0).UTC())
	}
}

func TestExtendMonthStartsOnlyFetchesNewMonths(t *testing.T) {
	c := newMonthChain()
	full, err := extendMonthStarts(context.Background(), nil, c.last(), c.sampleAt, c.timeAt)
	if err != nil {
		t.Fatal(err)
	}
	c.calls = 0
	again, err := extendMonthStarts(context.Background(), full, c.last(), c.sampleAt, c.timeAt)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(heightsOf(again)) != fmt.Sprint(heightsOf(full)) {
		t.Fatalf("refresh changed the months: %v, want %v", heightsOf(again), heightsOf(full))
	}
	if c.calls != 1 {
		t.Fatalf("refresh with every month known made %d calls, want 1 (the tip time)", c.calls)
	}

	c.calls = 0
	grown, err := extendMonthStarts(context.Background(), full[:len(full)-1], c.last(), c.sampleAt, c.timeAt)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(heightsOf(grown)) != fmt.Sprint(heightsOf(full)) {
		t.Fatalf("one missing month gave %v, want %v", heightsOf(grown), heightsOf(full))
	}
	if c.calls > 25 {
		t.Fatalf("finding one month took %d calls", c.calls)
	}
}

func TestExtendMonthStartsStopsBeforeAMonthTheTipHasNotReached(t *testing.T) {
	c := newMonthChain()
	cut := int64(0)
	for i, x := range c.times {
		if x >= utc(2021, time.August, 1, 0, 0) {
			cut = int64(i) - 1
			break
		}
	}
	c.times = c.times[:cut+1]
	got, err := extendMonthStarts(context.Background(), nil, c.last(), c.sampleAt, c.timeAt)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got); n != 3 {
		t.Fatalf("got %d samples (%v), want activation, June and July", n, heightsOf(got))
	}
}
