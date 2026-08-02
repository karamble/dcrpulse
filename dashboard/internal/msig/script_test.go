// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"bytes"
	"strings"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func genPubKeys(t *testing.T, n int) [][]byte {
	t.Helper()
	keys := make([][]byte, n)
	for i := range keys {
		priv, err := secp256k1.GeneratePrivateKey()
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		keys[i] = priv.PubKey().SerializeCompressed()
	}
	return keys
}

func TestMultiSigScriptCanonical(t *testing.T) {
	params := chaincfg.MainNetParams()
	pubs := genPubKeys(t, 3)

	script, sorted, err := MultiSigScript(2, pubs)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	addr, err := P2SHAddress(script, params)
	if err != nil {
		t.Fatalf("address: %v", err)
	}

	// Every exchange order of the same key set must produce the same
	// script and address.
	orders := [][][]byte{
		{pubs[2], pubs[0], pubs[1]},
		{pubs[1], pubs[2], pubs[0]},
		{pubs[2], pubs[1], pubs[0]},
	}
	for i, order := range orders {
		s2, _, err := MultiSigScript(2, order)
		if err != nil {
			t.Fatalf("order %d: %v", i, err)
		}
		if !bytes.Equal(script, s2) {
			t.Fatalf("order %d produced a different script", i)
		}
		a2, err := P2SHAddress(s2, params)
		if err != nil {
			t.Fatalf("order %d address: %v", i, err)
		}
		if a2 != addr {
			t.Fatalf("order %d produced a different address", i)
		}
	}

	m, extracted, err := ScriptDetails(script)
	if err != nil {
		t.Fatalf("details: %v", err)
	}
	if m != 2 || len(extracted) != 3 {
		t.Fatalf("details roundtrip: got %d-of-%d", m, len(extracted))
	}
	for i, pk := range extracted {
		if !bytes.Equal(pk, sorted[i]) {
			t.Fatalf("extracted key %d does not match sorted roster", i)
		}
		if !ContainsPubKey(pk, pubs) {
			t.Fatalf("extracted key %d not in original set", i)
		}
	}
}

func TestMultiSigScriptValidation(t *testing.T) {
	pubs := genPubKeys(t, 2)
	badLen := make([]byte, 32)
	badFormat := make([]byte, 33)
	badFormat[0] = 0x05

	cases := []struct {
		name string
		m    int
		keys [][]byte
	}{
		{"zero threshold", 0, pubs},
		{"threshold above n", 3, pubs},
		{"too many keys", 2, genPubKeys(t, 16)},
		{"duplicate key", 2, [][]byte{pubs[0], pubs[0]}},
		{"wrong key length", 2, [][]byte{pubs[0], badLen}},
		{"invalid key format", 2, [][]byte{pubs[0], badFormat}},
	}
	for _, tc := range cases {
		if _, _, err := MultiSigScript(tc.m, tc.keys); err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}
}

func TestP2SHAddressMainnetPrefix(t *testing.T) {
	script, _, err := MultiSigScript(2, genPubKeys(t, 2))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	addr, err := P2SHAddress(script, chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("address: %v", err)
	}
	if !strings.HasPrefix(addr, "Dc") {
		t.Fatalf("mainnet P2SH address %q lacks Dc prefix", addr)
	}
}
