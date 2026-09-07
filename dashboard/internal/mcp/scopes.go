// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

// Write scopes are the grantable write/action capabilities, generalizing the
// older fixed allow* flags into one extensible set. A write tool needs two keys:
// its read-domain (so the tool is visible to the agent) AND its write-scope (so
// the action is authorized by the user's spend grant). Adding a new write
// capability is a one-line addition here plus the tool's scope check.
const (
	scopeGovernance = "governance"
	scopeLightning  = "lightning"
	scopeDex        = "dex"
	scopeDexSpend   = "dex.spend"
	scopeBR         = "bisonrelay"
	scopeBRAdmin    = "bisonrelay.admin"
	scopeTimestamp  = "timestamp"
	scopeTor        = "tor"
	scopeStaking    = "staking"
	scopePrivacy    = "privacy"

	// Broadcasting a hardware-signed transaction is a non-fund action: the human
	// authorized the spend by signing on the device, so the agent only relays bytes.
	scopeWalletBroadcast = "wallet.broadcast"
)

// WriteScope is one grantable write/action capability. The dashboard renders the
// catalog as a checklist; the grant handler validates requests against it; tools
// gate on Key via grants.authorizeAction / authorizeActionGated / authorizeActionPass
// / authorizeLightning / authorizeSpendScoped / authorizeVSPFees.
type WriteScope struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Domain    string `json:"domain"`    // the read-domain this scope belongs to
	NeedsPass bool   `json:"needsPass"` // signs with the held wallet passphrase
	Fund      bool   `json:"fund"`      // draws on the DCR spend budget (caps + tripwire)
	Risk      bool   `json:"risk"`      // high blast-radius; shown with a warning in the UI
}

// writeScopeCatalog is the single source of truth for grantable write scopes.
var writeScopeCatalog = []WriteScope{
	{scopeGovernance, "Governance voting", "governance", true, false, false},
	{scopeLightning, "Lightning actions", "lightning", false, true, false},
	{scopeDex, "DEX trading + wallet ops", "dex", false, true, true},
	{scopeDexSpend, "DEX send / post-bond", "dex", false, true, true},
	{scopeBR, "Bison Relay write", "bisonrelay", false, false, false},
	{scopeBRAdmin, "Bison Relay group admin", "bisonrelay", false, false, true},
	{scopeTimestamp, "Timestamp write", "timestamp", false, false, false},
	{scopeTor, "Tor control", "tor", false, false, false},
	{scopeStaking, "Staking actions", "staking", true, true, false},
	{scopePrivacy, "Mixer control", "privacy", true, false, false},
	{scopeWalletBroadcast, "Broadcast pre-signed transactions", "wallet", false, false, false},
}

var writeScopeByKey = func() map[string]WriteScope {
	m := make(map[string]WriteScope, len(writeScopeCatalog))
	for _, w := range writeScopeCatalog {
		m[w.Key] = w
	}
	return m
}()

// WriteScopes returns the grantable write-scope catalog for the dashboard UI.
func WriteScopes() []WriteScope { return writeScopeCatalog }

// FilterScopes drops unknown/duplicate scope keys and reports whether any
// retained scope signs with the wallet passphrase (needsPass) or draws on the
// DCR spend budget (needsFund). Used by the dashboard grant handler to validate
// a request and decide which inputs (passphrase, caps) are required.
func FilterScopes(in []string) (scopes []string, needsPass, needsFund bool) {
	seen := map[string]bool{}
	for _, k := range in {
		w, ok := writeScopeByKey[k]
		if !ok || seen[k] {
			continue
		}
		seen[k] = true
		scopes = append(scopes, k)
		needsPass = needsPass || w.NeedsPass
		needsFund = needsFund || w.Fund
	}
	return
}
