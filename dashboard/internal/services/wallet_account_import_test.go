// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"strings"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
)

// Golden fixture shared with the device (decred-core account_export.rs pins
// the same bytes): two accounts, index 0 "Main" and index 1 "acc2", using the
// hosted-sim wallet's real dpubs.
const goldenAccountExportHex = "8201828300786f647075625a47344a314745655353466334565671594447745047457562666b6d4c47626672707151437638484c59644e4e4d5238744a426d61657468626e336d794146354e684c4c70546d4b32524543504e5274715a546b6d6b78364e41327476507267553277474c79796e755358644d61696e8301786f647075625a47344a31474565535346633568526b555747565a77484a43467764695779363773504e584c394d55707432587673575048423178486679795278714b694e79375831544c4a48665073524e6f54354c39617646763339786155474b5a6d4b6f724c376a314c37456e5a716461636332"

const (
	testDpub0 = "dpubZG4J1GEeSSFc4VVqYDGtPGEubfkmLGbfrpqQCv8HLYdNNMR8tJBmaethbn3myAF5NhLLpTmK2RECPNRtqZTkmkx6NA2tvPrgU2wGLyynuSX"
	testDpub1 = "dpubZG4J1GEeSSFc5hRkUWGVZwHJCFwdiWy67sPNXL9MUpt2XvsWPHB1xHfyyRxqKiNy7X1TLJHfPsRNoT5L9avFv39xaUGKZmKorL7j1L7EnZq"
)

// Test-side builders mirroring the device encoder, on top of the cborWriter.
func buildEntry(account uint64, dpub, name string) []byte {
	w := &cborWriter{}
	w.arrayHead(3)
	w.uint(account)
	w.head(3, uint64(len(dpub)))
	w.buf = append(w.buf, dpub...)
	w.head(3, uint64(len(name)))
	w.buf = append(w.buf, name...)
	return w.buf
}

func buildAccountExport(entries ...[]byte) []byte {
	w := &cborWriter{}
	w.arrayHead(2)
	w.uint(1)
	w.arrayHead(len(entries))
	out := w.buf
	for _, e := range entries {
		out = append(out, e...)
	}
	return out
}

func TestParseAccountExportGolden(t *testing.T) {
	data := mustHex(t, goldenAccountExportHex)
	entries, err := parseAccountExport(data, chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	// The fingerprints must match what the device derives for these accounts
	// (verified against the KeyOS hosted sim wallet).
	want := []struct {
		account uint32
		dpub    string
		name    string
		fp      string
	}{
		{0, testDpub0, "Main", "e2feaa8b"},
		{1, testDpub1, "acc2", "e96a2ed6"},
	}
	for i, w := range want {
		e := entries[i]
		if e.Account != w.account || e.Dpub != w.dpub || e.Name != w.name || e.Fp != w.fp {
			t.Fatalf("entry %d = %+v, want %+v", i, e, w)
		}
	}
	// The builders must reproduce the golden bytes exactly, proving they are
	// usable as the encoder reference for the other cases.
	rebuilt := buildAccountExport(buildEntry(0, testDpub0, "Main"), buildEntry(1, testDpub1, "acc2"))
	if string(rebuilt) != string(data) {
		t.Fatal("test builders diverge from the golden bytes")
	}
}

func TestParseAccountExportRejects(t *testing.T) {
	golden := mustHex(t, goldenAccountExportHex)
	params := chaincfg.MainNetParams()

	// Wrong version.
	bad := append([]byte(nil), golden...)
	bad[1] = 0x02
	if _, err := parseAccountExport(bad, params); err == nil {
		t.Fatal("version 2 accepted")
	}

	// Wrong network: the mainnet dpubs must be rejected under testnet params.
	if _, err := parseAccountExport(golden, chaincfg.TestNet3Params()); err == nil {
		t.Fatal("mainnet dpub accepted under testnet params")
	}

	// Truncated file.
	if _, err := parseAccountExport(golden[:40], params); err == nil {
		t.Fatal("truncated file accepted")
	}

	// Not CBOR at all.
	if _, err := parseAccountExport([]byte("dcr=1.23\n"), params); err == nil {
		t.Fatal("text file accepted")
	}

	// Duplicate account index inside one file.
	doubled := buildAccountExport(buildEntry(0, testDpub0, "Main"), buildEntry(0, testDpub1, "x"))
	if _, err := parseAccountExport(doubled, params); err == nil ||
		!strings.Contains(err.Error(), "twice") {
		t.Fatalf("duplicate index not rejected: %v", err)
	}

	// A garbage dpub string with valid CBOR framing.
	garbage := buildAccountExport(buildEntry(0, "dpubNOTAKEY", "Main"))
	if _, err := parseAccountExport(garbage, params); err == nil {
		t.Fatal("garbage dpub accepted")
	}
}

func TestParseAccountExportToleratesAppendedFields(t *testing.T) {
	// Future versions append fields; today's parser must skip them.
	golden := mustHex(t, goldenAccountExportHex)
	extended := append([]byte(nil), golden...)
	extended[0] = 0x83 // top-level array 2 -> 3
	extended = append(extended, 0x00)
	entries, err := parseAccountExport(extended, chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("appended top-level field rejected: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
}
