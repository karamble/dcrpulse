// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

// The rescan slot admits one runner; concurrent requests coalesce into a
// single follow-up pass from the lowest begin height seen while busy.
func TestRescanSingleFlight(t *testing.T) {
	rescanRunMu.Lock()
	busy, from, queued, next := rescanBusy, rescanFrom, rescanQueued, rescanNext
	rescanBusy, rescanFrom, rescanQueued, rescanNext = false, 0, false, 0
	rescanRunMu.Unlock()
	t.Cleanup(func() {
		rescanRunMu.Lock()
		rescanBusy, rescanFrom, rescanQueued, rescanNext = busy, from, queued, next
		rescanRunMu.Unlock()
	})

	if !BeginRescan(1000) {
		t.Fatal("BeginRescan(1000) on an idle slot should run")
	}
	if BeginRescan(1000) {
		t.Fatal("duplicate BeginRescan(1000) should coalesce")
	}
	if BeginRescan(2000) {
		t.Fatal("BeginRescan(2000) is covered by the in-flight rescan")
	}
	if BeginRescan(500) {
		t.Fatal("BeginRescan(500) should queue, not run")
	}
	if BeginRescan(400) {
		t.Fatal("BeginRescan(400) should lower the queued height, not run")
	}
	if BeginRescan(600) {
		t.Fatal("BeginRescan(600) should not displace the queued 400")
	}
	if got, again := EndRescan(); !again || got != 400 {
		t.Fatalf("EndRescan = (%d, %v), want (400, true)", got, again)
	}
	if got, again := EndRescan(); again || got != 0 {
		t.Fatalf("EndRescan = (%d, %v), want (0, false)", got, again)
	}
	if !BeginRescan(1000) {
		t.Fatal("BeginRescan(1000) should run after the slot is released")
	}
}
