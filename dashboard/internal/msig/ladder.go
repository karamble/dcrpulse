// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"errors"
	"fmt"
	"sort"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/hdkeychain/v3"
)

// An HD shared wallet derives its addresses from the roster's account
// xpubs instead of a fixed key set: address (b, i) is the m-of-n multisig
// over the lexicographically sorted child public keys of every roster
// xpub at branch b, index i. Branch 0 receives, branch 1 takes change.
//
// Derivation uses hdkeychain's Child, the same variant dcrwallet's
// address manager applies to account xpubs, so the keys derived here are
// exactly the keys the wallet can sign with once the account's branch
// index is synced. An index is skipped if and only if Child returns
// ErrInvalidChild for any roster xpub; every participant computes over
// the identical xpub set, so skipping is deterministic, and it mirrors
// dcrwallet's own skip-on-invalid behavior. The roster's canonical xpub
// order is independent of the per-index key order: xpubs sort as strings
// for identity and deduplication, while each index re-sorts its derived
// keys with SortPubKeys.
const (
	// GapExt and GapInt are protocol constants, not tunables: every
	// member imports at least this far beyond the highest used index,
	// and no member hands out an address beyond its local bound, so an
	// index a peer can legally disclose is always inside every member's
	// imported window.
	GapExt uint32 = 20
	GapInt uint32 = 10

	// BranchExternal receives payments; BranchInternal takes change.
	BranchExternal uint32 = 0
	BranchInternal uint32 = 1

	// maxXpubLen bounds a serialized extended public key on the wire.
	maxXpubLen = 120
)

// DerivationScheme names the exact procedure implemented here, and is
// recorded in every backup card: a card is only restorable by a build
// that implements its scheme, because deriving even one address any
// other way rebuilds the wrong wallet. Changing anything about the
// derivation (path, child variant, sorting, skip rule) is a NEW scheme,
// never a refactor; the golden fixture test seals it.
const DerivationScheme = "dcrpulse-msig-hd-1"

// ErrSkipIndex marks a child index the whole roster deterministically
// skips because at least one xpub cannot derive it.
var ErrSkipIndex = errors.New("child index skipped")

// childSeam derives one non-hardened child. Tests inject failures here
// to exercise the skip rule, which real keys hit with ~2^-127 odds.
var childSeam = func(key *hdkeychain.ExtendedKey, i uint32) (*hdkeychain.ExtendedKey, error) {
	return key.Child(i)
}

// ParseXpub decodes an extended public key and requires it to be public
// and to belong to the given network.
func ParseXpub(s string, params *chaincfg.Params) (*hdkeychain.ExtendedKey, error) {
	if s == "" || len(s) > maxXpubLen {
		return nil, fmt.Errorf("malformed extended public key")
	}
	key, err := hdkeychain.NewKeyFromString(s, params)
	if err != nil {
		return nil, fmt.Errorf("malformed extended public key: %v", err)
	}
	if key.IsPrivate() {
		return nil, fmt.Errorf("extended key is private; only public keys may be shared")
	}
	return key, nil
}

// ParseXpubAnyNet reports whether s parses as a public extended key on
// any supported network. Wire validation uses it because accept frames
// carry no network field; handlers re-parse strictly against the
// record's network.
func ParseXpubAnyNet(s string) error {
	var lastErr error
	for _, network := range []string{"mainnet", "testnet3", "simnet"} {
		params, err := paramsForNetwork(network)
		if err != nil {
			continue
		}
		if _, err := ParseXpub(s, params); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("malformed extended public key")
	}
	return lastErr
}

// ValidateXpubRoster enforces the canonical roster form: exactly n
// strictly ascending, well-formed public extended keys. Ascending string
// order doubles as duplicate rejection.
func ValidateXpubRoster(xpubs []string, n int) error {
	if len(xpubs) != n {
		return fmt.Errorf("roster has %d keys, expected %d", len(xpubs), n)
	}
	for i, x := range xpubs {
		if err := ParseXpubAnyNet(x); err != nil {
			return err
		}
		if i > 0 && xpubs[i-1] >= x {
			return fmt.Errorf("roster keys not in canonical sorted order")
		}
	}
	return nil
}

// SortXpubs returns the canonical (ascending) copy of an xpub set.
func SortXpubs(xpubs []string) []string {
	sorted := append([]string(nil), xpubs...)
	sort.Strings(sorted)
	return sorted
}

// KeysAt derives every roster child public key at (branch, index),
// serialized compressed. Returns ErrSkipIndex when any roster xpub
// cannot derive the child.
func KeysAt(xpubs []string, branch, index uint32, params *chaincfg.Params) ([][]byte, error) {
	keys := make([][]byte, 0, len(xpubs))
	for _, x := range xpubs {
		acct, err := ParseXpub(x, params)
		if err != nil {
			return nil, err
		}
		branchKey, err := childSeam(acct, branch)
		if err != nil {
			if errors.Is(err, hdkeychain.ErrInvalidChild) {
				return nil, fmt.Errorf("%w: branch %d", ErrSkipIndex, branch)
			}
			return nil, err
		}
		child, err := childSeam(branchKey, index)
		if err != nil {
			if errors.Is(err, hdkeychain.ErrInvalidChild) {
				return nil, fmt.Errorf("%w: %d/%d", ErrSkipIndex, branch, index)
			}
			return nil, err
		}
		keys = append(keys, child.SerializedPubKey())
	}
	return keys, nil
}

// ScriptAt builds the redeem script at (branch, index): the m-of-n
// multisig over the sorted child keys. The returned key order is the
// script's, not the roster's.
func ScriptAt(m int, xpubs []string, branch, index uint32, params *chaincfg.Params) ([]byte, [][]byte, error) {
	keys, err := KeysAt(xpubs, branch, index, params)
	if err != nil {
		return nil, nil, err
	}
	return MultiSigScript(m, keys)
}

// AddressAt derives the P2SH address at (branch, index).
func AddressAt(m int, xpubs []string, branch, index uint32, params *chaincfg.Params) (string, error) {
	script, _, err := ScriptAt(m, xpubs, branch, index, params)
	if err != nil {
		return "", err
	}
	return P2SHAddress(script, params)
}

// LadderEntry is one derived rung of a shared wallet.
type LadderEntry struct {
	Branch  uint32
	Index   uint32
	Address string
	Script  []byte
}

// DeriveWindow derives entries for raw indices [from, to) on one branch,
// stepping over skipped indices. Cursors count raw indices, holes
// included, so every participant's window covers the same rungs.
func DeriveWindow(m int, xpubs []string, branch, from, to uint32, params *chaincfg.Params) ([]LadderEntry, error) {
	entries := make([]LadderEntry, 0, int(to-from))
	for i := from; i < to; i++ {
		script, _, err := ScriptAt(m, xpubs, branch, i, params)
		if errors.Is(err, ErrSkipIndex) {
			continue
		}
		if err != nil {
			return nil, err
		}
		addr, err := P2SHAddress(script, params)
		if err != nil {
			return nil, err
		}
		entries = append(entries, LadderEntry{Branch: branch, Index: i, Address: addr, Script: script})
	}
	return entries, nil
}

// WalletIDForRoster derives the wallet's identity: the address at the
// smallest non-skipped external index, index 0 in practice.
func WalletIDForRoster(m int, xpubs []string, params *chaincfg.Params) (string, error) {
	for i := uint32(0); i < GapExt; i++ {
		addr, err := AddressAt(m, xpubs, BranchExternal, i, params)
		if errors.Is(err, ErrSkipIndex) {
			continue
		}
		if err != nil {
			return "", err
		}
		return addr, nil
	}
	return "", fmt.Errorf("no derivable external index within the gap window")
}
