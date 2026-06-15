// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import "errors"

// isOverLimit reports whether err is a spend-amount limit violation (over the
// per-transaction or daily cap). Wrong-account and non-allowlisted denials are
// excluded: those are likelier an honest agent mistake than a theft attempt.
func isOverLimit(err error) bool {
	return errors.Is(err, errPerTxExceeded) || errors.Is(err, errDailyExceeded)
}

// tripwire reacts to a failed spend authorization. If the agent attempted to
// exceed its spend limit, it is treated as compromised: the grant is revoked
// (passphrase zeroed) and the token is blocked (persisted), so the agent loses
// all spend access and cannot reconnect until the user unblocks it in the
// dashboard. Returns true if it tripped.
func tripwire(agentID string, err error) bool {
	if !isOverLimit(err) {
		return false
	}
	grants.revoke(agentID)
	reg.block(agentID)
	_ = saveAgents()
	return true
}
