// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/hdkeychain/v3"
)

// testXpub derives a deterministic account-level xpub the way a wallet
// does: hardened 44'/42'/account' from a fixed per-name seed.
func testXpub(t *testing.T, name string, account uint32, params *chaincfg.Params) string {
	t.Helper()
	seed := make([]byte, 32)
	copy(seed, name)
	for i := len(name); i < len(seed); i++ {
		seed[i] = byte(i)
	}
	key, err := hdkeychain.NewMaster(seed, params)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []uint32{44, 42, account} {
		key, err = key.ChildBIP32Std(hdkeychain.HardenedKeyStart + step)
		if err != nil {
			t.Fatal(err)
		}
	}
	return key.Neuter().String()
}

// The golden vector freezes the derivation variant: account xpub ->
// Child(branch) -> Child(index), the same calls dcrwallet's address
// manager makes. Any change to the derivation path or child variant
// breaks these strings and must be treated as a consensus break between
// participants, not a refactor.
func TestLadderDerivationFixture(t *testing.T) {
	params := chaincfg.MainNetParams()
	xpubs := SortXpubs([]string{
		testXpub(t, "alice", 1, params),
		testXpub(t, "bob", 1, params),
	})
	if err := ValidateXpubRoster(xpubs, 2); err != nil {
		t.Fatalf("roster: %v", err)
	}

	golden := map[string]string{
		"ext0": "DceexfjpagKPLtaJK29JR6ss5zbnyxhU1Jc",
		"ext1": "DcjfgBhXfSuV6sSJytToB387PWFwgm2zoE8",
		"int0": "DciAe2F2iSxiNqG6b6AjLDSZUEd5QMhitRL",
	}
	got := map[string]string{}
	var err error
	if got["ext0"], err = AddressAt(2, xpubs, BranchExternal, 0, params); err != nil {
		t.Fatal(err)
	}
	if got["ext1"], err = AddressAt(2, xpubs, BranchExternal, 1, params); err != nil {
		t.Fatal(err)
	}
	if got["int0"], err = AddressAt(2, xpubs, BranchInternal, 0, params); err != nil {
		t.Fatal(err)
	}
	for k, want := range golden {
		if got[k] != want {
			t.Errorf("%s: %s != golden %s", k, got[k], want)
		}
	}

	id, err := WalletIDForRoster(2, xpubs, params)
	if err != nil {
		t.Fatal(err)
	}
	if id != got["ext0"] {
		t.Fatalf("wallet id %s != external index 0 %s", id, got["ext0"])
	}
	if got["ext0"] == got["ext1"] || got["ext0"] == got["int0"] || got["ext1"] == got["int0"] {
		t.Fatalf("derived addresses collide: %v", got)
	}
}

// The roster's canonical xpub order and each index's key order are
// independent: scripts must not depend on the order xpubs are passed in,
// and the per-index sorted key order may differ between indices.
func TestLadderSortIndependence(t *testing.T) {
	params := chaincfg.MainNetParams()
	a := testXpub(t, "alice", 1, params)
	b := testXpub(t, "bob", 1, params)
	c := testXpub(t, "carol", 1, params)

	s1, _, err := ScriptAt(2, []string{a, b, c}, BranchExternal, 0, params)
	if err != nil {
		t.Fatal(err)
	}
	s2, _, err := ScriptAt(2, []string{c, a, b}, BranchExternal, 0, params)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(s1, s2) {
		t.Fatalf("script depends on xpub input order")
	}

	// Find an index whose sorted key order differs from index 0's owner
	// order. With three keys re-sorted per index this happens quickly;
	// scan a window to keep the test deterministic without goldens.
	base, err := KeysAt([]string{a, b, c}, BranchExternal, 0, params)
	if err != nil {
		t.Fatal(err)
	}
	_ = base
	varies := false
	prevOwners := ""
	for i := uint32(0); i < 12; i++ {
		keys, err := KeysAt([]string{a, b, c}, BranchExternal, i, params)
		if err != nil {
			t.Fatal(err)
		}
		_, sorted, err := MultiSigScript(2, keys)
		if err != nil {
			t.Fatal(err)
		}
		owners := ""
		for _, sk := range sorted {
			for oi, ok := range keys {
				if bytes.Equal(sk, ok) {
					owners += fmt.Sprintf("%d", oi)
				}
			}
		}
		if prevOwners != "" && owners != prevOwners {
			varies = true
		}
		prevOwners = owners
	}
	if !varies {
		t.Fatalf("per-index key order never varied across the window; sorting is not per-index")
	}
}

// Skipping is deterministic and window iteration steps over holes.
func TestLadderSkipDeterminism(t *testing.T) {
	params := chaincfg.MainNetParams()
	xpubs := SortXpubs([]string{
		testXpub(t, "alice", 1, params),
		testXpub(t, "bob", 1, params),
	})

	orig := childSeam
	t.Cleanup(func() { childSeam = orig })
	failures := 0
	childSeam = func(key *hdkeychain.ExtendedKey, i uint32) (*hdkeychain.ExtendedKey, error) {
		// Fail index 1 on the external branch for every key: the
		// branch step passes (depth parity distinguishes account ->
		// branch from branch -> index by the requested value).
		child, err := key.Child(i)
		if err != nil {
			return nil, err
		}
		if i == 1 && key.Depth() == 4 {
			failures++
			return nil, hdkeychain.ErrInvalidChild
		}
		return child, nil
	}

	if _, err := KeysAt(xpubs, BranchExternal, 1, params); !errors.Is(err, ErrSkipIndex) {
		t.Fatalf("expected ErrSkipIndex, got %v", err)
	}
	entries, err := DeriveWindow(2, xpubs, BranchExternal, 0, 4, params)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("window entries: %d != 3", len(entries))
	}
	for _, e := range entries {
		if e.Index == 1 {
			t.Fatalf("window did not skip index 1")
		}
	}
	if failures == 0 {
		t.Fatalf("seam never fired")
	}

	// A skipped index 0 moves the wallet id to index 1... simulate by
	// failing index 0 instead.
	childSeam = func(key *hdkeychain.ExtendedKey, i uint32) (*hdkeychain.ExtendedKey, error) {
		if i == 0 && key.Depth() == 4 {
			return nil, hdkeychain.ErrInvalidChild
		}
		return key.Child(i)
	}
	id, err := WalletIDForRoster(2, xpubs, params)
	if err != nil {
		t.Fatal(err)
	}
	childSeam = orig
	want, err := AddressAt(2, xpubs, BranchExternal, 1, params)
	if err != nil {
		t.Fatal(err)
	}
	if id != want {
		t.Fatalf("wallet id with skipped index 0: %s != index-1 address %s", id, want)
	}
}

func TestValidateXpubRoster(t *testing.T) {
	params := chaincfg.MainNetParams()
	a := testXpub(t, "alice", 1, params)
	b := testXpub(t, "bob", 1, params)
	sorted := SortXpubs([]string{a, b})

	if err := ValidateXpubRoster(sorted, 2); err != nil {
		t.Fatalf("valid roster rejected: %v", err)
	}
	if err := ValidateXpubRoster(sorted, 3); err == nil {
		t.Fatalf("count mismatch accepted")
	}
	if err := ValidateXpubRoster([]string{sorted[1], sorted[0]}, 2); err == nil {
		t.Fatalf("unsorted roster accepted")
	}
	if err := ValidateXpubRoster([]string{sorted[0], sorted[0]}, 2); err == nil {
		t.Fatalf("duplicate roster accepted")
	}
	if err := ValidateXpubRoster([]string{sorted[0], "dpubnonsense"}, 2); err == nil {
		t.Fatalf("garbage xpub accepted")
	}
}

func TestParseXpubRejections(t *testing.T) {
	mainnet := chaincfg.MainNetParams()
	simnet := chaincfg.SimNetParams()

	// Private extended keys must never pass.
	seed := make([]byte, 32)
	master, err := hdkeychain.NewMaster(seed, mainnet)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseXpub(master.String(), mainnet); err == nil {
		t.Fatalf("private extended key accepted")
	}

	// Wrong-network keys must fail strict parsing but pass any-net.
	sim := testXpub(t, "alice", 1, simnet)
	if _, err := ParseXpub(sim, mainnet); err == nil {
		t.Fatalf("simnet xpub accepted under mainnet params")
	}
	if err := ParseXpubAnyNet(sim); err != nil {
		t.Fatalf("any-net rejected a valid simnet xpub: %v", err)
	}
	if err := ParseXpubAnyNet("nonsense"); err == nil {
		t.Fatalf("any-net accepted garbage")
	}
}
