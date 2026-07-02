// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/hdkeychain/v3"
)

// testSignRequest is a fixed small request mirroring the shape the device's
// fp_compat fixtures exercise: one input, one output.
func testSignRequest() *signRequest {
	prevHash := make([]byte, 32)
	for i := range prevHash {
		prevHash[i] = byte(i)
	}
	return &signRequest{
		formatVersion: 1,
		txVersion:     1,
		account:       0,
		lockTime:      0,
		expiry:        0,
		inputs: []srInput{{
			prevHash:   prevHash,
			prevIndex:  0,
			tree:       0,
			sequence:   0xFFFFFFFF,
			valueIn:    100000,
			branch:     0,
			index:      3,
			prevScript: []byte{0x76, 0xA9, 0x14},
		}},
		outputs: []srOutput{{
			value:    94000,
			version:  0,
			pkScript: []byte{0x76, 0xA9},
			isChange: false,
		}},
	}
}

// The optional trailing account_fp follows the device's fp_compat fixture
// relationship exactly: without it the top-level array has 7 elements
// (header 0x87); with it the encoding is byte-identical except the header
// becomes 0x88 and a 4-integer CBOR array is appended.
func TestEncodeSignRequestAccountFp(t *testing.T) {
	without := encodeSignRequest(testSignRequest())
	if without[0] != 0x87 {
		t.Fatalf("header without fingerprint: %#x != 0x87", without[0])
	}

	withFp := testSignRequest()
	withFp.accountFp = []byte{1, 2, 3, 4}
	got := encodeSignRequest(withFp)
	if got[0] != 0x88 {
		t.Fatalf("header with fingerprint: %#x != 0x88", got[0])
	}
	if !bytes.Equal(got[1:len(without)], without[1:]) {
		t.Fatal("fingerprint variant altered the preceding elements")
	}
	tail := got[len(without):]
	want := []byte{0x84, 0x01, 0x02, 0x03, 0x04}
	if !bytes.Equal(tail, want) {
		t.Fatalf("fingerprint tail % x != % x", tail, want)
	}

	// Fingerprint bytes >= 24 take the 1+1 uint form; the tail is still a
	// 4-element array.
	withFp.accountFp = []byte{0x9C, 0x0D, 0x11, 0x39}
	got = encodeSignRequest(withFp)
	tail = got[len(without):]
	want = []byte{0x84, 0x18, 0x9C, 0x0D, 0x11, 0x18, 0x39}
	if !bytes.Equal(tail, want) {
		t.Fatalf("fingerprint tail % x != % x", tail, want)
	}
}

// The fingerprint is the account key's OWN hash160 snippet (Decred flavor
// via dcrutil.Hash160), not the parent fingerprint embedded in the dpub.
func TestAccountFingerprintDerivation(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(0x40 + i)
	}
	params := chaincfg.MainNetParams()
	master, err := hdkeychain.NewMaster(seed, params)
	if err != nil {
		t.Fatal(err)
	}
	acct, err := master.ChildBIP32Std(hdkeychain.HardenedKeyStart + 44)
	if err != nil {
		t.Fatal(err)
	}
	acct, err = acct.ChildBIP32Std(hdkeychain.HardenedKeyStart + 42)
	if err != nil {
		t.Fatal(err)
	}
	acct, err = acct.ChildBIP32Std(hdkeychain.HardenedKeyStart + 0)
	if err != nil {
		t.Fatal(err)
	}
	dpub := acct.Neuter().String()

	parsed, err := hdkeychain.NewKeyFromString(dpub, params)
	if err != nil {
		t.Fatal(err)
	}
	fp := dcrutil.Hash160(parsed.SerializedPubKey())[:4]
	wantFp := dcrutil.Hash160(acct.Neuter().SerializedPubKey())[:4]
	if !bytes.Equal(fp, wantFp) {
		t.Fatalf("fingerprint % x != % x", fp, wantFp)
	}
	if len(fp) != 4 {
		t.Fatalf("fingerprint length %d != 4", len(fp))
	}
	// The self-fingerprint must differ from the serialized parent
	// fingerprint (the wrong bytes to use).
	if pf := parsed.ParentFingerprint(); pf == uint32(fp[0])<<24|uint32(fp[1])<<16|uint32(fp[2])<<8|uint32(fp[3]) {
		t.Fatal("self fingerprint unexpectedly equals the parent fingerprint")
	}
}
