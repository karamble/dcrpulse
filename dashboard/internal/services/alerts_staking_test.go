// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

// The watcher diffs at most once per new block. The height must advance only
// when a listing actually completed, or a failed one costs that block its diff
// and nothing retries until the next block arrives.
func TestTicketDiffHeightAdvancesOnlyOnSuccess(t *testing.T) {
	t.Cleanup(resetTicketAlertBaseline)
	resetTicketAlertBaseline()

	if !shouldDiffAt(100) {
		t.Fatalf("first snapshot at height 100 should diff")
	}

	// A failed diff records nothing, so the next snapshot at the same height
	// tries again rather than skipping.
	if !shouldDiffAt(100) {
		t.Fatalf("height 100 should still diff after a failed attempt")
	}

	markDiffedAt(100)
	if shouldDiffAt(100) {
		t.Fatalf("height 100 should be skipped once its diff completed")
	}
	if !shouldDiffAt(101) {
		t.Fatalf("a new block should diff")
	}
}

// Snapshots can repeat or arrive out of order; the height is a high-water mark.
func TestTicketDiffHeightIsAHighWaterMark(t *testing.T) {
	t.Cleanup(resetTicketAlertBaseline)
	resetTicketAlertBaseline()

	markDiffedAt(200)
	markDiffedAt(150)
	if shouldDiffAt(150) {
		t.Fatalf("an older height must not reopen the window")
	}
	if shouldDiffAt(200) {
		t.Fatalf("the recorded height must stay skipped")
	}
	if !shouldDiffAt(201) {
		t.Fatalf("the next block should diff")
	}
}

func TestResetTicketAlertBaselineClearsHeight(t *testing.T) {
	t.Cleanup(resetTicketAlertBaseline)

	markDiffedAt(500)
	resetTicketAlertBaseline()
	if !shouldDiffAt(1) {
		t.Fatalf("a wallet switch must reopen diffing at any height")
	}
}
