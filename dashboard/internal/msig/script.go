// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package msig implements the script and transaction primitives for
// dcrpulse shared wallets: version 0 P2SH multisig wallets whose
// cosigners coordinate over Bison Relay.
package msig

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/txscript/v4/stdscript"
)

const (
	// MinRequired is the smallest usable signature threshold.
	MinRequired = 1

	// MaxPubKeys caps the roster size. The network itself allows more
	// (dcrd's relay policy rejects P2SH redemptions past 15 sigops), but
	// dcrpulse caps schemes at 8 participants: beyond that the serial
	// signing relay becomes impractical before the chain does.
	MaxPubKeys = 8

	compressedPubKeyLen = 33
)

// SortPubKeys returns a lexicographically sorted copy of the given
// compressed public keys. Sorted order is the dcrpulse convention for
// shared wallets: every participant derives the same redeem script from
// the same key set no matter the order keys were exchanged in.
func SortPubKeys(pubKeys [][]byte) [][]byte {
	sorted := make([][]byte, len(pubKeys))
	copy(sorted, pubKeys)
	sort.Slice(sorted, func(i, j int) bool {
		return bytes.Compare(sorted[i], sorted[j]) < 0
	})
	return sorted
}

// MultiSigScript validates the key set, sorts it, and builds the version 0
// m-of-n CHECKMULTISIG redeem script. The sorted key order is returned so
// callers can persist the canonical roster.
func MultiSigScript(m int, pubKeys [][]byte) ([]byte, [][]byte, error) {
	n := len(pubKeys)
	if m < MinRequired {
		return nil, nil, fmt.Errorf("required signatures must be at least %d", MinRequired)
	}
	if n < m {
		return nil, nil, fmt.Errorf("%d keys cannot satisfy a threshold of %d", n, m)
	}
	if n > MaxPubKeys {
		return nil, nil, fmt.Errorf("%d keys exceeds the relay policy limit of %d", n, MaxPubKeys)
	}
	seen := make(map[string]struct{}, n)
	for _, pk := range pubKeys {
		if len(pk) != compressedPubKeyLen {
			return nil, nil, fmt.Errorf("public key must be %d bytes, got %d", compressedPubKeyLen, len(pk))
		}
		if _, err := secp256k1.ParsePubKey(pk); err != nil {
			return nil, nil, fmt.Errorf("invalid public key: %v", err)
		}
		if _, ok := seen[string(pk)]; ok {
			return nil, nil, fmt.Errorf("duplicate public key in key set")
		}
		seen[string(pk)] = struct{}{}
	}
	sorted := SortPubKeys(pubKeys)
	script, err := stdscript.MultiSigScriptV0(m, sorted...)
	if err != nil {
		return nil, nil, err
	}
	return script, sorted, nil
}

// ScriptDetails parses a version 0 multisig redeem script into its
// threshold and public keys.
func ScriptDetails(script []byte) (int, [][]byte, error) {
	details := stdscript.ExtractMultiSigScriptDetailsV0(script, true)
	if !details.Valid {
		return 0, nil, fmt.Errorf("not a version 0 multisig script")
	}
	return int(details.RequiredSigs), details.PubKeys, nil
}

// P2SHAddress returns the pay-to-script-hash address of a redeem script.
func P2SHAddress(script []byte, params *chaincfg.Params) (string, error) {
	addr, err := stdaddr.NewAddressScriptHashV0(script, params)
	if err != nil {
		return "", err
	}
	return addr.String(), nil
}
