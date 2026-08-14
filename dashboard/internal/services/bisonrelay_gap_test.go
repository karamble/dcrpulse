// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

// TestTheFirstEventIsNeverAGap guards the most damaging possible regression
// here: treating the first event of a connection as a hole would ask every
// game to resynchronise every table every time the dashboard starts.
func TestTheFirstEventIsNeverAGap(t *testing.T) {
	var s notifGapState
	// brclientd has been up a while, so its first delivered sequence is
	// arbitrary rather than 1.
	if n, why := s.observe(9174, "abc", 0); n != 0 {
		t.Fatalf("the first event reported a gap of %d (%s)", n, why)
	}
	if n, _ := s.observe(9175, "abc", 0); n != 0 {
		t.Fatalf("the second consecutive event reported a gap of %d", n)
	}
}

// TestAKeepaliveDoesNotTouchTheBaseline kills the bug where the unnumbered
// heartbeat resets the baseline, which would suppress every later gap.
func TestAKeepaliveDoesNotTouchTheBaseline(t *testing.T) {
	var s notifGapState
	s.observe(10, "abc", 0)
	if n, _ := s.observe(0, "", 0); n != 0 {
		t.Fatalf("a keepalive reported a gap of %d", n)
	}
	// The hole must still be visible across it.
	if n, _ := s.observe(14, "abc", 0); n != 3 {
		t.Fatalf("after a keepalive the gap is %d, want 3", n)
	}
}

func TestAHoleInTheSequenceIsCounted(t *testing.T) {
	var s notifGapState
	s.observe(1, "abc", 0)
	n, why := s.observe(5, "abc", 0)
	if n != 3 {
		t.Fatalf("gap is %d, want 3", n)
	}
	if why == "" {
		t.Fatal("a gap was reported with no reason")
	}
}

func TestAProducerRestartIsAGapOfUnknownSize(t *testing.T) {
	var s notifGapState
	s.observe(500, "abc", 0)
	n, why := s.observe(1, "def", 0)
	if n != unknownGap {
		t.Fatalf("a restart reported %d, want an unknown-sized gap", n)
	}
	if describeGap(n) != "an unknown number of" {
		t.Fatalf("describeGap renders the restart case as %q", describeGap(n))
	}
	if why == "" {
		t.Fatal("a restart was reported with no reason")
	}
	// And the new numbering is adopted, so the next event is not also a gap.
	if n, _ := s.observe(2, "def", 0); n != 0 {
		t.Fatalf("the event after a restart reported a gap of %d", n)
	}
}

// TestABackwardsSequenceIsReportedButCausesNoResync kills two changes: turning
// a producer bug into a resync storm across every game, and letting the
// anomaly pass unremarked so nobody ever chases it.
func TestABackwardsSequenceIsReportedButCausesNoResync(t *testing.T) {
	var s notifGapState
	s.observe(100, "abc", 0)

	n, why := s.observe(40, "abc", 0)
	if n != 0 {
		t.Fatalf("a backwards sequence reported a gap of %d, want 0", n)
	}
	if why == "" {
		t.Fatal("a backwards sequence passed silently; it is a producer bug and must be logged")
	}

	// It resynchronises rather than latching, so normal counting resumes, and
	// an ordinary event afterwards says nothing at all.
	n, why = s.observe(41, "abc", 0)
	if n != 0 || why != "" {
		t.Fatalf("after a backwards sequence an ordinary event reported %d (%q)", n, why)
	}
}

// TestMissedIsBelievedWithoutASequenceHole covers the case a sequence cannot
// see: events dropped before this subscriber's first delivered event, where
// there is no baseline to compare against.
func TestMissedIsBelievedWithoutASequenceHole(t *testing.T) {
	var s notifGapState
	n, why := s.observe(7, "abc", 4)
	if n != 4 {
		t.Fatalf("gap is %d, want the 4 brclientd reported", n)
	}
	if why == "" {
		t.Fatal("a reported drop came with no reason")
	}
}

// TestAnOldBrclientdIsInert is the mixed-version claim: a producer that sends
// no sequence must never look like loss.
func TestAnOldBrclientdIsInert(t *testing.T) {
	var s notifGapState
	for i := 0; i < 50; i++ {
		if n, _ := s.observe(0, "", 0); n != 0 {
			t.Fatalf("an unnumbered event reported a gap of %d", n)
		}
	}
	if s.lastEpoch != "" || s.lastSeq != 0 {
		t.Fatalf("unnumbered events moved the baseline to %d/%q", s.lastSeq, s.lastEpoch)
	}
}
