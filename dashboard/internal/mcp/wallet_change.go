// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import "dcrpulse/internal/services"

// Registering here rather than from the caller that starts the server: the
// linker guarantees it, and the hook has to be live even when the surface is
// off, since the operator can turn it on again without changing wallet.
func init() { services.OnActiveWalletChange(onActiveWalletChange) }

// onActiveWalletChange drops everything this package holds that was minted
// against the wallet the daemons are leaving. A spend grant names account
// numbers, caps and a passphrase but no wallet, so a grant issued against one
// wallet would otherwise keep its authority over the same-numbered accounts of
// the next, and the held passphrase would be tried against them.
//
// Agent identities, tokens, domains and blocks are untouched: the operator
// changed wallet, not their trust in the agent. The audit trail is kept too, so
// it still shows what an agent did up to the change.
func onActiveWalletChange(ch services.ActiveWalletChange) {
	n := grants.revokeAll()
	invalidateAllServers()
	InvalidateStakingProfile()
	resetEventRings()
	brmcpFeed.reset()
	if n > 0 {
		mcpLog.Warnf("Active wallet changed (%q to %q): revoked %d spend grant(s); "+
			"re-grant spend access under the new wallet", ch.Old, ch.New, n)
	}
}
