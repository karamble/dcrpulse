// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "time"

// The supervised reconnect loops (the transaction watcher, the DEX watcher and
// cmd/dcrpulse's RpcSync supervisor) share one backoff policy, mirroring the
// brclientd WS client's nextBackoff: an attempt that ran healthily for a while
// resets the sequence instead of inheriting the delay earned by whatever went
// wrong at startup. Without the reset, a rough boot leaves a permanent 60s
// reconnect gap - and streams without replay lose whatever happened in it.
const (
	supervisorMinBackoff = 5 * time.Second
	supervisorMaxBackoff = 60 * time.Second
	supervisorHealthyFor = 30 * time.Second
)

// NextSupervisorBackoff returns the delay to wait after an attempt that ran
// for ranFor, given the delay currently in force. A healthy run resets the
// sequence; otherwise it doubles, clamped after doubling so the ceiling is
// not overshot.
func NextSupervisorBackoff(current, ranFor time.Duration) time.Duration {
	if ranFor >= supervisorHealthyFor {
		return supervisorMinBackoff
	}
	next := current * 2
	if next > supervisorMaxBackoff {
		next = supervisorMaxBackoff
	}
	return next
}
