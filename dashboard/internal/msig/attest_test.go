// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
)

// TestAttestDigestGolden seals the exact bytes participants sign. The digest is
// wire contract: change it and every attestation already in flight stops
// verifying, so a deliberate edit has to come here first.
func TestAttestDigestGolden(t *testing.T) {
	xpubs := []string{"xpubB", "xpubA", "xpubC"}
	got := AttestRosterMessage("simnet", "00aabbcc", 2, 3, "DsAddr", xpubs)
	want := strings.Join([]string{
		"dcrpulse-msig-attest-v1",
		"simnet",
		"00aabbcc",
		"2-of-3",
		"DsAddr",
		"xpubA", "xpubB", "xpubC",
	}, "\n")
	if got != want {
		t.Fatalf("roster digest drifted:\n got %q\nwant %q", got, want)
	}
	// Order of the input must not change the digest: every participant
	// derives the same string from the same key set.
	if other := AttestRosterMessage("simnet", "00aabbcc", 2, 3, "DsAddr", []string{"xpubC", "xpubA", "xpubB"}); other != got {
		t.Fatalf("digest depends on input order")
	}
	// The identity tuples are deliberately outside the digest, so nothing a
	// nick or uid says can ever be laundered into something signed.
	if strings.Contains(got, "nick") || strings.Contains(got, "uid") {
		t.Fatalf("digest carries identity claims: %q", got)
	}

	pop := AttestPoPMessage("simnet", "00aabbcc", "xpubA")
	if pop != "dcrpulse-msig-attest-pop-v1\nsimnet\n00aabbcc\nxpubA" {
		t.Fatalf("proof-of-possession digest drifted: %q", pop)
	}
	// The two must never collide, or a proof of possession could be
	// replayed as a roster commitment.
	if strings.HasPrefix(pop, attestRosterPrefix+"\n") {
		t.Fatalf("the two attestation messages share a prefix")
	}
}

// TestVerifyAttestRejectsGarbage covers the shape checks before any curve work.
func TestVerifyAttestRejectsGarbage(t *testing.T) {
	params := chaincfg.SimNetParams()
	xpub := "spub..." // never reached: the signature is rejected first
	for _, tc := range []struct{ name, sig string }{
		{"empty", ""},
		{"not base64", "!!!!"},
		{"too short", "c2hvcnQ="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := VerifyAttest(xpub, "message", tc.sig, params); err == nil {
				t.Fatalf("accepted %q as a signature", tc.sig)
			}
		})
	}
}

// TestAttestedRosterNeedsEveryKeySigned is the regression test for the finding.
// A roster is only as good as the proof that its keys are held: an initiator
// that seats a key nobody signed for must not be able to complete the round.
func TestAttestedRosterNeedsEveryKeySigned(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob", "carol")
	tempID := hd.createHD(t, 2, "alice", "bob", "carol")
	hd.acceptAll(t, tempID, "bob", "carol")
	hd.settle(t, tempID, "alice", "bob", "carol")

	rec := hd.record("bob", tempID)
	if rec.Status != StatusActive || len(rec.Attests) != 3 {
		t.Fatalf("setup: bob %s with %d attestations", rec.Status, len(rec.Attests))
	}
	params := chaincfg.SimNetParams()

	// Drop one signature: the set no longer covers every key.
	short := rec.Attests[:len(rec.Attests)-1]
	if err := VerifyRosterAttests(rec, short, params); err == nil {
		t.Fatal("a set missing a key's signature was accepted")
	}

	// Duplicate one to pad the count back up. Covering a key twice must not
	// stand in for covering another key at all.
	padded := append(append([]RosterAttest(nil), short...), short[0])
	if err := VerifyRosterAttests(rec, padded, params); err == nil {
		t.Fatal("a set padded with a repeated key was accepted")
	}

	// Move a valid signature onto a different key: verification recovers the
	// signing key rather than trusting the label, so it fails.
	swapped := append([]RosterAttest(nil), rec.Attests...)
	swapped[0].Sig, swapped[1].Sig = swapped[1].Sig, swapped[0].Sig
	if err := VerifyRosterAttests(rec, swapped, params); err == nil {
		t.Fatal("signatures attributed to the wrong keys were accepted")
	}
}

// TestAttestedSplitRosterNeverCompletes pins the split-view guarantee: a
// signature is over one exact key set, so a set gathered for one roster can
// never satisfy a node holding a different one.
func TestAttestedSplitRosterNeverCompletes(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob", "carol")
	tempID := hd.createHD(t, 2, "alice", "bob", "carol")
	hd.acceptAll(t, tempID, "bob", "carol")
	hd.settle(t, tempID, "alice", "bob", "carol")

	rec := hd.record("bob", tempID)
	params := chaincfg.SimNetParams()
	if err := VerifyRosterAttests(rec, rec.Attests, params); err != nil {
		t.Fatalf("the round's own set does not verify: %v", err)
	}

	// The same signatures against a record that differs in any part of the
	// digest: a different threshold, address or key set is a different wallet.
	for _, tc := range []struct {
		name  string
		twist func(*WalletRecord)
	}{
		{"threshold", func(r *WalletRecord) { r.M = 3 }},
		{"address", func(r *WalletRecord) { r.Address = "DsSomewhereElse" }},
		{"key set", func(r *WalletRecord) { r.Xpubs = r.Xpubs[:len(r.Xpubs)-1] }},
		{"round", func(r *WalletRecord) { r.TempID = "00ffffff" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := cloneRecord(rec)
			tc.twist(other)
			if err := VerifyRosterAttests(other, rec.Attests, params); err == nil {
				t.Fatalf("a set from another roster satisfied a changed %s", tc.name)
			}
		})
	}
}

// TestAttestedAcceptWithoutProofFailsRound pins that an accept must prove the
// key it offers is held, and that the round fails loudly rather than settling
// on a key nobody has.
func TestAttestedAcceptWithoutProofFailsRound(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()

	// Bob answers with a well-formed accept carrying no proof, which is what
	// a build without cosigner attestation would send.
	bob := hd.record("bob", tempID)
	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	bob = hd.record("bob", tempID)
	hd.queue = nil
	store := hd.store("alice")
	rec := hd.record("alice", tempID)
	inboundAcceptHD(hd.ctx, store, rec, &Message{
		Type: TypeAccept, Ver: ProtoHD, TempID: tempID, Xpub: bob.OwnHD.Xpub,
	}, rec.Peers[0].UID, time.Now())

	got := hd.record("alice", tempID)
	if got.Status != StatusFailed {
		t.Fatalf("round survived an unproven key: %s", got.Status)
	}
	if !strings.Contains(got.FailReason, "upgrade") {
		t.Fatalf("failure reason does not point at the fix: %q", got.FailReason)
	}
}
