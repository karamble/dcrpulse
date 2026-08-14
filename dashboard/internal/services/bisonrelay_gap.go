// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"fmt"
	"time"

	"dcrpulse/internal/alerts"
)

// notifGapState follows brclientd's event numbering across reconnects.
//
// Across reconnects is the point. brclientd numbers a publish whether or not
// anybody is listening, so the same epoch with the sequence advanced over a
// reconnect is the one loss neither side can otherwise see: brclientd's own
// drop counters never fired, and the bridge's counters only ever counted
// frames that arrived. That is exactly the case where a game would be told
// "clean resume, no gap" about frames nobody knows are missing.
type notifGapState struct {
	lastSeq   uint64
	lastEpoch string
}

// observe folds one event into the state and reports how many events were lost
// before it, with a phrase naming why.
//
// Zero means no loss, and is the answer for every ambiguous case: the cost of
// a false gap is every game resyncing every table, so anything short of a
// demonstrated hole is treated as continuity. A zero count with a reason still
// set is the third answer - nothing was lost, but something happened that an
// operator should see - and the caller logs it without resyncing anybody.
func (s *notifGapState) observe(seq uint64, epoch string, missed uint64) (uint64, string) {
	// No sequence at all: the per-connection keepalive, or a brclientd from
	// before numbering existed. Leaving the baseline untouched is what makes
	// the two versions mix, and what stops a heartbeat from erasing the
	// baseline every thirty seconds.
	if seq == 0 {
		return 0, ""
	}

	switch {
	case s.lastEpoch == "":
		// First event since this process started. Nothing to compare against,
		// and treating it as a gap would resync every game on every restart.
		s.lastEpoch, s.lastSeq = epoch, seq
		if missed > 0 {
			return missed, "brclientd reported dropping them before our first event"
		}
		return 0, ""

	case epoch != s.lastEpoch:
		// brclientd restarted. Its numbering starts over, so the old sequence
		// says nothing about the new one, and whatever it published while it
		// was away is gone.
		s.lastEpoch, s.lastSeq = epoch, seq
		return unknownGap, "brclientd restarted"

	case seq > s.lastSeq+1:
		n := seq - s.lastSeq - 1
		s.lastSeq = seq
		return n, "events did not reach the dashboard"

	case seq <= s.lastSeq:
		// Not possible from a correct producer. Resynchronise quietly rather
		// than declare a gap: a producer bug must not put every game into a
		// resync loop.
		s.lastSeq = seq
		return 0, "sequence went backwards"

	default:
		s.lastSeq = seq
		if missed > 0 {
			return missed, "brclientd dropped them for a full buffer"
		}
		return 0, ""
	}
}

// noteNotifGap records a gap: in the log, in the alert list, and to every
// connected game.
//
// The games are the reason this matters. A game is told on subscribe whether
// it missed anything, and until now that answer could only be built from
// frames the bridge actually saw, so events lost upstream of it were reported
// as continuity. Marking the gap and then closing the stream is what turns
// that lie into a resync; the order is not negotiable, because closing first
// produces a fresh stream that truthfully reports no gap it knows about.
func noteNotifGap(n uint64, why, epoch string) {
	brelLog.Warnf("lost %s brclientd event(s): %s", describeGap(n), why)

	// Bucketed by the minute: Emit dedupes against every retained entry rather
	// than a window, so a fixed key would report the first gap and silence
	// every one after it.
	key := fmt.Sprintf("%s/%d", epoch, time.Now().Unix()/60)
	alerts.Emit("br_notif_gap",
		fmt.Sprintf("Lost %s event(s) from Bison Relay: %s. Games at a table have been asked to resynchronise.",
			describeGap(n), why), key)

	Gaming().resyncAllGames(why)
}

// unknownGap stands for "some events were lost and there is no way to count
// them", which is what a producer restart means. It is deliberately not zero,
// so it reads as a gap everywhere the count is only tested for being positive.
const unknownGap = ^uint64(0)

// describeGap renders a count for a log line, since the restart case has no
// number worth printing.
func describeGap(n uint64) string {
	if n == unknownGap {
		return "an unknown number of"
	}
	return fmt.Sprintf("%d", n)
}
