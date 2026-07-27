// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// GamingSettings is the policy that confines the Bison Relay gaming section.
// Games are untrusted: they run beside a wallet, dcrlnd and a BR identity, so
// they never get wallet credentials. Everything they can spend passes through
// this policy, and the account below is the only one they may touch.
//
// Account scope is enforced here rather than in the wallet on purpose:
// dcrwallet accounts share one seed and one unlock passphrase, so an account is
// a policy boundary above the wallet, never a cryptographic one below it.
//
// Amounts are atoms; the frontend speaks DCR and converts at the handler.
type GamingSettings struct {
	Enabled bool `json:"enabled"`

	// Account is the wallet account the gaming section may spend from and
	// receive payouts into. Empty means no account is bound and nothing can
	// be staked.
	Account string `json:"account"`

	// Mode is "approval" (ask before every stake) or "autopay" (stake under
	// the caps without asking).
	Mode string `json:"mode"`

	// PerTableCapAtoms bounds a single buy-in.
	PerTableCapAtoms int64 `json:"perTableCapAtoms"`

	// PerDayCapAtoms bounds everything staked in a rolling day.
	PerDayCapAtoms int64 `json:"perDayCapAtoms"`

	// MaxOpenTables bounds how many escrows may be funded at once. Separate
	// from the per-table cap because escrows overlap: the exposure that
	// matters is the total outstanding, not the largest single buy-in.
	MaxOpenTables int `json:"maxOpenTables"`

	// ApprovalTimeoutSecs is how long a stake waits for approval before it
	// is abandoned.
	ApprovalTimeoutSecs int `json:"approvalTimeoutSecs"`

	// InstalledGames holds the game ids the user added. An id is the routing
	// key in the `--gaming[game=<id>]` Bison Relay envelope, so an
	// installation routes only the games listed here.
	InstalledGames []string `json:"installedGames"`

	// GameTokens maps an installed game id to its bearer token.
	//
	// The token is the game's identity, not merely a password. A game
	// authenticates with it, and everything the host enforces - which game
	// a frame may be sent as, and later which account may be spent from and
	// under what caps - is enforced against the identity it resolves to.
	// That is why there is one per game rather than one for the section: a
	// shared secret would make every game the same principal, and a limit
	// on a principal nobody can tell apart is not a limit.
	//
	// A game never states who it is. It presents a token and the host
	// decides, so a game cannot claim to be another one.
	GameTokens map[string]string `json:"gameTokens,omitempty"`

	// Rev bumps on every write so a supervisor can notice a policy change.
	Rev int `json:"rev"`
}

// GamingGame is one entry in the gaming catalogue.
type GamingGame struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	// ProtocolVersion is the `gv=` key in the wire envelope. A client that
	// does not implement a game's protocol version must ignore its traffic
	// rather than surface it.
	ProtocolVersion int `json:"protocolVersion"`

	// Installed reports whether the user added this game.
	Installed bool `json:"installed"`

	// Ready reports whether the game's backend is actually reachable. A game
	// can be installed and not ready while its service is starting or absent.
	Ready bool `json:"ready"`
}
