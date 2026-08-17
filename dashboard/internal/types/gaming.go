// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// GamePolicy is one registered game's confinement: what it is called, the
// account it may spend from, and how much of it.
//
// Per game rather than per installation, because the credential a game presents
// is an identity rather than a password. A cap on a principal nobody can tell
// apart is not a cap: with one shared budget the busy game spends the careful
// one's allowance, and a spend one game asked for could be paid out of an
// account it was never allowed to touch.
//
// Amounts are atoms; the frontend speaks DCR and converts at the handler.
type GamePolicy struct {
	// Name is the label the operator gave this game, for the interface to
	// show. Decoration: nothing routes, resolves or authorises by it, and a
	// game with no label is called by its id, which is the only name the
	// wire carries.
	Name string `json:"name,omitempty"`

	// Account is the wallet account this game may spend from and be paid
	// into. Empty binds nothing and this game can stake nothing, which is
	// what a game registered but not yet funded looks like.
	//
	// Account scope is enforced here rather than in the wallet on purpose:
	// dcrwallet accounts share one seed and one unlock passphrase, so an
	// account is a policy boundary above the wallet, never a cryptographic
	// one below it. What it does buy is a bankroll - what is not in this
	// account is not at stake for this game, and what one game loses is not
	// drawn from another's.
	Account string `json:"account"`

	// PerTableCapAtoms bounds a single buy-in.
	PerTableCapAtoms int64 `json:"perTableCapAtoms"`

	// PerDayCapAtoms bounds everything this game stakes in a rolling day,
	// counted against this game alone.
	PerDayCapAtoms int64 `json:"perDayCapAtoms"`

	// ApprovalTimeoutSecs is how long one of this game's stakes waits for a
	// person before it is abandoned. Per game because the deadline belongs to
	// the game's protocol rather than to the person answering: a seat closes
	// at a block height, and an approval that lands after the table has
	// formed buys nothing.
	ApprovalTimeoutSecs int `json:"approvalTimeoutSecs"`
}

// GamingSettings is the bridge's own state: whether it runs, which games are
// registered, and what each of them is trusted with.
//
// Everything that costs money is per game, in GamePolicy. What is left here is
// the bridge itself, which is a tunnel: carrying frames needs no account and no
// caps, so nothing global decides what anything may spend.
//
// There is no automatic-payment setting. Every buy-in is approved by a person
// with the wallet passphrase, which this process never holds - so a setting
// saying otherwise could only ever be refused, and a refusal dressed as a
// choice is worse than no choice at all.
type GamingSettings struct {
	// Enabled is whether the bridge carries anything at all. It is refused
	// unless the App Password is actively protecting the dashboard: the caps
	// and the approvals are worth exactly as much as the certainty that the
	// person answering is the operator.
	Enabled bool `json:"enabled"`

	// RegisteredGames holds the game ids the operator registered, lowercased
	// and sorted. An id is the routing key in the `--gaming[game=<id>]`
	// Bison Relay envelope, so this list is the routing table and a frame for
	// anything else is dropped as unroutable.
	//
	// Any id the wire can carry may be registered. There is no catalogue: a
	// game has to be able to appear without this build being taught its name,
	// which is what routing on a key is for.
	RegisteredGames []string `json:"registeredGames"`

	// Policies holds one policy per registered game, keyed by id. An entry
	// exists for exactly the ids in RegisteredGames: registering mints one
	// from the defaults, removing deletes it, and an unrelated write leaves
	// every other game's untouched.
	Policies map[string]GamePolicy `json:"policies,omitempty"`

	// GameCredentials maps a registered game id to the credential it
	// authenticates with. A game with no entry has not been issued one yet
	// and cannot connect.
	//
	// The credential is the game's identity, not merely a password. Everything
	// the bridge enforces - which game a frame may be sent as, which account
	// may be spent from and under what caps - is enforced against the identity
	// a connection resolves to. That is why there is one per game rather than
	// one for the section: a shared secret would make every game the same
	// principal, and a limit on a principal nobody can tell apart is not a
	// limit.
	//
	// A game never states who it is. It presents a certificate and the bridge
	// decides, so a game cannot claim to be another one.
	GameCredentials map[string]GameCredential `json:"gameCredentials,omitempty"`
}

// GameCredential is what the bridge remembers about a game's certificate.
//
// The private key is not here and is never written down: it is shown to the
// operator once, at generation, and carried to the game by hand. That is what
// makes it impossible for anything on either machine to fetch a credential it
// was not given.
type GameCredential struct {
	// Fingerprint is the SHA-256 of the certificate, which is what a
	// connection resolves by.
	Fingerprint string `json:"fingerprint"`

	// CertPEM is the certificate itself.
	//
	// Kept because the certificate is its own root: a handshake verifies a
	// game's certificate by finding these exact bytes among the roots it
	// trusts, so a fingerprint alone could not survive a restart. A
	// certificate is public by construction - the secret is the key, and the
	// key is not here.
	CertPEM string `json:"certPem"`

	// IssuedAt is when the operator generated it, so the console can say how
	// old a credential is.
	IssuedAt int64 `json:"issuedAt"`
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

	// MinRefundBlocks and BondLockBlocks are the timelocks the game advertised
	// on Hello, for the console to disclose before a person pays. Zero when the
	// game advertised none, or is not connected.
	MinRefundBlocks uint32 `json:"minRefundBlocks"`
	BondLockBlocks  uint32 `json:"bondLockBlocks"`
}
