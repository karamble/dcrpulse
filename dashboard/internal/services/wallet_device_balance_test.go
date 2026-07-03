// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"encoding/hex"
	"testing"
)

// Golden vectors computed with an independent CBOR + bytewords implementation
// (validated against foundation-ur's own test vector). They double as the
// fixture set for the device-side decoder: asof 1751537520, rate $16.5647,
// accounts fp 7f3a9c21 = 1200 DCR and fp b44e0d5a = 34.56 DCR.
const (
	goldenBalanceHex = "84011a686657701a00fcc1dc828284187f183a189c18211b0000001bf08eb000828418b4184e0d185a1acdfe6000"
	goldenBalanceUR  = "ur:dcr-balance/lradcyisiyhgjocyaeztseuolflflrcslbcsftcsnscsclcwaeaeaecwwtmnpfaelflrcsqzcsglbtcshtcysnzehnaeluwlregl"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex fixture: %v", err)
	}
	return b
}

func goldenAccounts(t *testing.T) []deviceBalanceAccount {
	return []deviceBalanceAccount{
		{fp: mustHex(t, "7f3a9c21"), atoms: 120000000000},
		{fp: mustHex(t, "b44e0d5a"), atoms: 3456000000},
	}
}

func TestEncodeBalanceUpdateGolden(t *testing.T) {
	got := encodeBalanceUpdate(1751537520, 16564700, goldenAccounts(t))
	if hex.EncodeToString(got) != goldenBalanceHex {
		t.Fatalf("golden mismatch:\n got %x\nwant %s", got, goldenBalanceHex)
	}
}

func TestEncodeBalanceUpdateURGolden(t *testing.T) {
	cbor := encodeBalanceUpdate(1751537520, 16564700, goldenAccounts(t))
	if got := EncodeUR("dcr-balance", cbor); got != goldenBalanceUR {
		t.Fatalf("UR mismatch:\n got %s\nwant %s", got, goldenBalanceUR)
	}
}

func TestEncodeBalanceUpdateEmptyAndRateZero(t *testing.T) {
	got := encodeBalanceUpdate(1751537520, 0, nil)
	if want := "84011a686657700080"; hex.EncodeToString(got) != want {
		t.Fatalf("empty mismatch:\n got %x\nwant %s", got, want)
	}
}

func TestEncodeBalanceUpdateMaxAtoms(t *testing.T) {
	// 21e14 atoms = the 21M DCR supply cap; exercises the 8-byte uint head.
	got := encodeBalanceUpdate(1751537520, 16564700, []deviceBalanceAccount{
		{fp: mustHex(t, "00ff10e0"), atoms: 2100000000000000},
	})
	if want := "84011a686657701a00fcc1dc8182840018ff1018e01b000775f05a074000"; hex.EncodeToString(got) != want {
		t.Fatalf("max mismatch:\n got %x\nwant %s", got, want)
	}
}
