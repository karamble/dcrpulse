// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

func TestMarkSignFailuresCountsUnsignedVotes(t *testing.T) {
	const token = "signfailtoken"
	st := &vtRunState{token: token, total: 5}
	st.markSignFailures([]string{"sign aa11: account locked", "sign bb22: account locked"})

	vtMu.Lock()
	snap := st.snapshotLocked()
	vtMu.Unlock()
	if snap.Failed != 2 || snap.Pending != 3 {
		t.Fatalf("failed/pending = %d/%d, want 2/3", snap.Failed, snap.Pending)
	}
	if snap.LastError != "sign bb22: account locked" {
		t.Fatalf("last error = %q", snap.LastError)
	}

	var got []string
	for _, ev := range LastVoteTrickleEvents(voteTrickleEventBufferSize) {
		if ev.Token == token && ev.Level == "error" && ev.Kind == "failed" {
			got = append(got, ev.Message)
		}
	}
	if len(got) != 2 || got[0] != "sign aa11: account locked" || got[1] != "sign bb22: account locked" {
		t.Fatalf("events = %q", got)
	}
}
