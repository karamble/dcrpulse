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

	// PerTableCapAtoms bounds a single buy-in.
	PerTableCapAtoms int64 `json:"perTableCapAtoms"`

	// PerDayCapAtoms bounds everything staked in a rolling day.
	PerDayCapAtoms int64 `json:"perDayCapAtoms"`

	// ApprovalTimeoutSecs is how long a stake waits for approval before it
	// is abandoned.
	ApprovalTimeoutSecs int `json:"approvalTimeoutSecs"`

	// InstalledGames holds the game ids the operator registered, lowercased
	// and sorted. An id is the routing key in the `--gaming[game=<id>]`
	// Bison Relay envelope, so an installation routes only the games listed
	// here and a frame for anything else is dropped as unroutable.
	//
	// Any id the wire can carry may be registered. There is no catalogue: a
	// game has to be able to appear without this build being taught its
	// name, which is what routing on a key is for.
	InstalledGames []string `json:"installedGames"`

	// GameNames maps a registered game id to the label the operator gave it,
	// for the interface to show. It is decoration: nothing routes, resolves
	// or authorises by it, and a game with no label is called by its id,
	// which is the only name the wire carries.
	GameNames map[string]string `json:"gameNames,omitempty"`

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
}

// GamingGame is one registered game, as the dashboard lists it.
type GamingGame struct {
	// ID is the routing key, and the only name the wire carries.
	ID string `json:"id"`

	// Name is the label the operator gave it, or the id when they gave none.
	Name string `json:"name"`

	// Ready reports whether the game is connected to the bridge right now.
	// Deliberately not the same as registered: a game is registered here and
	// run by the person elsewhere, so it can be added and not running.
	Ready bool `json:"ready"`
}
