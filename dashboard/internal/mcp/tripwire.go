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

// saveAgentsFn persists the agent roster. A variable so a test can drive the
// failure branch below, which otherwise needs an unwritable data volume.
var saveAgentsFn = saveAgents

// blockAndPersist revokes the agent's grant, blocks its token, and writes the
// roster so the block survives a restart.
//
// The write is best-effort because neither caller has anywhere to return an
// error to, but it is not silent. The block is in force in this process either
// way; if it never reaches the file, a restart brings the token back, and the
// only thing lost is the reason - the agent's authority to spend needs a fresh
// grant regardless, since grants do not survive a restart at all.
func blockAndPersist(agentID string) {
	grants.revoke(agentID)
	reg.block(agentID)
	if err := saveAgentsFn(); err != nil {
		mcpLog.Errorf("Agent %s is blocked, but saving the roster failed, so a "+
			"restart will accept its token again: %v", agentID, err)
	}
}

// tripwire reacts to a failed spend authorization. If the agent attempted to
// exceed its spend limit, it is treated as compromised: the grant is revoked
// (passphrase zeroed) and the token is blocked, so the agent loses all spend
// access and cannot reconnect until the user unblocks it in the dashboard.
// Returns true if it tripped.
//
// No audit row here: every call site records its own, naming the tool that
// tripped.
func tripwire(agentID string, err error) bool {
	if !isOverLimit(err) {
		return false
	}
	blockAndPersist(agentID)
	return true
}
