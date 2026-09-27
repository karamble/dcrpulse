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
// drop counters never fired. Game frames do not ride this stream; the bridge
// reads them from the GCMStream replay log, which loses nothing.
type notifGapState struct {
	lastSeq   uint64
	lastEpoch string
}

// observe folds one event into the state and reports how many events were lost
// before it, with a phrase naming why.
//
// Zero means no loss, and is the answer for every ambiguous case: a false gap
// is an alert about nothing, so anything short of a demonstrated hole is
// treated as continuity. A zero count with a reason still set is the third
// answer - nothing was lost, but something happened that an operator should
// see - and the caller logs it without raising an alert.
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
		// and treating it as a gap would raise an alert on every restart.
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
		// than declare a gap: a producer bug must not flood the alert list.
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

// noteNotifGap records a gap in the log and in the alert list, so an operator
// knows the dashboard may have missed chat events.
func noteNotifGap(n uint64, why, epoch string) {
	brelLog.Warnf("lost %s brclientd event(s): %s", describeGap(n), why)

	// Bucketed by the minute: Emit dedupes against every retained entry rather
	// than a window, so a fixed key would report the first gap and silence
	// every one after it.
	key := fmt.Sprintf("%s/%d", epoch, time.Now().Unix()/60)
	alerts.Emit("br_notif_gap",
		fmt.Sprintf("Lost %s event(s) from Bison Relay: %s.", describeGap(n), why), key)
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
