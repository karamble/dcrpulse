// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"strings"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/hdkeychain/v3"
)

// accountKey derives m/44'/42'/0' from a fixed seed for params.
func accountKey(t *testing.T, params *chaincfg.Params) *hdkeychain.ExtendedKey {
	t.Helper()
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	key, err := hdkeychain.NewMaster(seed, params)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []uint32{44, 42, 0} {
		if key, err = key.ChildBIP32Std(hdkeychain.HardenedKeyStart + step); err != nil {
			t.Fatal(err)
		}
	}
	return key
}

// An account xpub used to be accepted on its dpub/tpub prefix alone.
func TestParseAccountXpub(t *testing.T) {
	main := chaincfg.MainNetParams()
	priv := accountKey(t, main)
	dpub := priv.Neuter().String()
	tpub := accountKey(t, chaincfg.TestNet3Params()).Neuter().String()

	if _, err := parseAccountXpub(dpub, main); err != nil {
		t.Fatalf("mainnet dpub on mainnet: %v", err)
	}
	last := dpub[len(dpub)-1]
	flipped := dpub[:len(dpub)-1] + map[bool]string{true: "a", false: "b"}[last != 'a']
	for name, key := range map[string]string{
		"a testnet key on mainnet": tpub,
		"a private key":            priv.String(),
		"a bad checksum":           flipped,
		"only the prefix":          "dpubXYZ",
	} {
		if _, err := parseAccountXpub(key, main); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := parseAccountXpub(priv.String(), main); err == nil || !strings.Contains(err.Error(), "PRIVATE") {
		t.Errorf("private key error = %v, want it named", err)
	}
}
