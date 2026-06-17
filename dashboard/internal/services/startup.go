// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"strings"
)

// DaemonStartupState explains, in user-facing terms, why a daemon RPC call
// failed while the daemon is not yet serving. It is derived by tailing the
// daemon's own log file, because during a startup database upgrade the daemon
// has either not opened its RPC interface (dcrd) or not finished loading the
// wallet behind it (dcrwallet), so the upgrade is only observable in the log,
// not over RPC.
type DaemonStartupState struct {
	State   string `json:"state"`   // "upgrading" or "starting"
	Detail  string `json:"detail"`  // the matched log line, when upgrading
	Message string `json:"message"` // user-facing explanation
}

// connectivityMarkers are lowercased substrings that identify a transport-level
// failure (daemon down, still starting, or mid-upgrade) as opposed to a genuine
// RPC error from a daemon that is up and answering.
var connectivityMarkers = []string{
	"connection refused",
	"no such host",
	"i/o timeout",
	"deadline exceeded",
	"connection reset",
	"broken pipe",
	"no route to host",
	"dial tcp",
	"error while dialing",
	"code = unavailable",
	"server misbehaving",
	"actively refused",
}

// IsDaemonUnreachable reports whether err looks like a daemon that is not
// reachable yet (down, starting, or running a database upgrade) rather than a
// genuine RPC-level error from a responsive daemon.
func IsDaemonUnreachable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, m := range connectivityMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// lndStartupPhrases are substrings dcrlnd emits while it is down or still
// booting (the post-unlock "starting up, not yet ready" window) rather than a
// genuine RPC error such as a locked wallet.
var lndStartupPhrases = []string{
	"starting up",
	"not yet ready",
	"waiting to start",
	"rpc services not available",
}

// LndStartupOrUnreachable reports whether err indicates dcrlnd is unreachable or
// still starting up (vs a real RPC error like a locked wallet). Lets the UI show
// a "starting" state instead of the unlock wizard or a raw 500.
func LndStartupOrUnreachable(err error) bool {
	if err == nil {
		return false
	}
	if IsDaemonUnreachable(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, m := range lndStartupPhrases {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// Upgrade and "ready" log markers, lowercased. These are intentionally generic
// (no hardcoded version numbers) so they keep matching across daemon releases.
var (
	dcrdUpgradeStarts = []string{
		"upgrading database to version",
		"upgrading utxo database",
		"upgrading utxo set",
		"upgrading spend journal",
		"migrating database",
		"migrating utxo database",
		"migrating versioning scheme",
		"reindexing",
		"creating and storing gcs filters",
	}
	dcrdReadyMarkers = []string{
		"done upgrading",
		"done migrating",
		"new valid peer",
		"rpc server listening",
	}
	dcrwalletUpgradeStarts = []string{
		"upgrading database from version",
	}
	dcrwalletReadyMarkers = []string{
		"opened wallet",
	}
	dcrlndUpgradeStarts = []string{
		"performing database schema migration",
		"migrating",
	}
	dcrlndReadyMarkers = []string{
		"rpc server listening",
		"ready for",
	}
)

func startupMarkers(c LogComponent) (starts, ready []string) {
	switch c {
	case LogComponentDcrd:
		return dcrdUpgradeStarts, dcrdReadyMarkers
	case LogComponentDcrwallet:
		return dcrwalletUpgradeStarts, dcrwalletReadyMarkers
	case LogComponentDcrlnd:
		return dcrlndUpgradeStarts, dcrlndReadyMarkers
	default:
		return nil, nil
	}
}

// DaemonStartupHint inspects the tail of a daemon's log to decide whether an
// unreachable daemon is mid database upgrade or simply still starting, and
// returns a user-facing message. It is meant to be called only on the
// daemon-unreachable path (see IsDaemonUnreachable). A startup marker counts as
// "in progress" only when no completion/ready marker appears after it, so a
// previously-finished upgrade still in the log tail does not false-positive.
func DaemonStartupHint(ctx context.Context, component LogComponent) DaemonStartupState {
	noun := "node"
	switch component {
	case LogComponentDcrwallet:
		noun = "wallet"
	case LogComponentDcrlnd:
		noun = "Lightning node"
	}
	starting := DaemonStartupState{
		State:   "starting",
		Message: "The " + noun + " is starting up and is not reachable yet. This page will recover automatically once it is ready.",
	}

	starts, ready := startupMarkers(component)
	if len(starts) == 0 {
		return starting
	}

	lines, err := TailLog(ctx, component, 150)
	if err != nil || len(lines) == 0 {
		return starting
	}

	lastStart, lastReady, startLine := -1, -1, ""
	for i, ln := range lines {
		l := strings.ToLower(ln)
		for _, m := range starts {
			if strings.Contains(l, m) {
				lastStart, startLine = i, ln
				break
			}
		}
		for _, m := range ready {
			if strings.Contains(l, m) {
				lastReady = i
				break
			}
		}
	}

	if lastStart > lastReady {
		return DaemonStartupState{
			State:   "upgrading",
			Detail:  strings.TrimSpace(startLine),
			Message: "Database upgrade in progress. After a version update the " + noun + " upgrades its database before it can serve requests, which can take several minutes. This page will recover automatically when it finishes.",
		}
	}
	return starting
}
