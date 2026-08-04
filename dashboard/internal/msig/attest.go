// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

// Cosigner attestation. The roster a cosigner receives is assembled by the
// initiator, so on its own it proves only that the key set derives the address
// it claims. An attestation adds the missing half: every key in the roster is
// proven to be held by someone who signed off on this exact key set.
//
// What that does and does not buy, stated plainly because the UI must not
// overstate it. The guarantee is exactly: every key in the roster is held by
// someone who signed off on this exact key set. That closes a roster seeded
// with a key nobody holds (which would brick the funds at threshold time), a
// roster that lifts a third party's published xpub without their involvement,
// split views (a key can attest to at most one roster, so a divergent story
// cannot complete on both sides), and tampering by a courier or relay that
// holds no key in the roster.
//
// It does NOT prove the holders are distinct people, and it does NOT close a
// roster whose other slots are keys the initiator controls outright: it signs
// for those happily. Only verifying a cosigner's key with that person, over a
// channel the initiator does not carry, closes that one. Say so in the UI.
//
// The signing key is child (0,0) of the participant's own account xpub: the one
// key every other participant can derive from the roster alone. Signatures are
// dcrwallet signmessage compact signatures, so they verify with nothing but this
// package and are checkable out of band with dcrctl verifymessage.
const (
	attestRosterPrefix = "dcrpulse-msig-attest-v1"
	attestPoPPrefix    = "dcrpulse-msig-attest-pop-v1"

	// attestBranch and attestIndex pin the ladder rung that signs. They are
	// part of the wire contract: change them and old attestations stop
	// verifying.
	attestBranch = uint32(0)
	attestIndex  = uint32(0)

	// signedMessageMagic is dcrd's message-signing domain separator, which is
	// what keeps a signature over one of these strings from ever being a valid
	// signature over a transaction sighash.
	signedMessageMagic = "Decred Signed Message:\n"
)

// AttestRosterMessage is the string every participant signs to commit to a
// settled roster. It covers exactly what guards funds: the network, the round,
// the threshold, the derived address and the key set in canonical order.
//
// The roster's identity tuples are deliberately absent. They are asserted by
// the initiator, and signing them would launder a claim about who holds a key
// into something that looks verified.
func AttestRosterMessage(network, tempID string, m, n int, address string, xpubs []string) string {
	var b strings.Builder
	b.WriteString(attestRosterPrefix)
	b.WriteString("\n")
	b.WriteString(network)
	b.WriteString("\n")
	b.WriteString(tempID)
	b.WriteString("\n")
	b.WriteString(strconv.Itoa(m))
	b.WriteString("-of-")
	b.WriteString(strconv.Itoa(n))
	b.WriteString("\n")
	b.WriteString(address)
	for _, x := range SortXpubs(xpubs) {
		b.WriteString("\n")
		b.WriteString(x)
	}
	return b.String()
}

// AttestPoPMessage is the string a cosigner signs when it accepts an invite,
// proving it holds the key behind the xpub it is offering before that key ever
// reaches the roster.
func AttestPoPMessage(network, tempID, ownXpub string) string {
	return attestPoPPrefix + "\n" + network + "\n" + tempID + "\n" + ownXpub
}

// AttestAddress returns the address whose key signs this participant's
// attestations, so the signer can name it to the wallet.
func AttestAddress(xpub string, params *chaincfg.Params) (string, error) {
	keys, err := KeysAt([]string{xpub}, attestBranch, attestIndex, params)
	if err != nil {
		return "", err
	}
	addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(dcrutil.Hash160(keys[0]), params)
	if err != nil {
		return "", err
	}
	return addr.String(), nil
}

// signedMessageHash reproduces dcrd's message-signing digest.
func signedMessageHash(message string) ([]byte, error) {
	var buf bytes.Buffer
	if err := wire.WriteVarString(&buf, 0, signedMessageMagic); err != nil {
		return nil, err
	}
	if err := wire.WriteVarString(&buf, 0, message); err != nil {
		return nil, err
	}
	return chainhash.HashB(buf.Bytes()), nil
}

// VerifyAttest checks that sigB64 is a signature over message by the key behind
// xpub. It recovers the signing key rather than trusting any claim about who
// signed, so a signature only ever verifies against the one roster slot whose
// key produced it.
func VerifyAttest(xpub, message, sigB64 string, params *chaincfg.Params) error {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil {
		return fmt.Errorf("attestation is not valid base64: %w", err)
	}
	if len(sig) != 65 {
		return fmt.Errorf("attestation is %d bytes, want a 65-byte compact signature", len(sig))
	}
	hash, err := signedMessageHash(message)
	if err != nil {
		return err
	}
	pub, _, err := ecdsa.RecoverCompact(sig, hash)
	if err != nil {
		return fmt.Errorf("attestation does not recover a key: %w", err)
	}
	want, err := KeysAt([]string{xpub}, attestBranch, attestIndex, params)
	if err != nil {
		return err
	}
	if !bytes.Equal(pub.SerializeCompressed(), want[0]) {
		return fmt.Errorf("attestation was not signed by the key it claims")
	}
	return nil
}

// VerifyRosterAttests checks a full attestation set against the verifier's own
// locally derived roster: every xpub must carry exactly one valid signature over
// the verifier's own digest. Both halves matter. Requiring every xpub is what
// makes a phantom slot need a real signature; recomputing the message locally is
// what makes a split-view roster fail, because a signature collected over one
// key set cannot verify against another.
func VerifyRosterAttests(rec *WalletRecord, attests []RosterAttest, params *chaincfg.Params) error {
	if rec == nil {
		return fmt.Errorf("no wallet record")
	}
	message := AttestRosterMessage(rec.Network, rec.TempID, rec.M, rec.N, rec.Address, rec.Xpubs)
	seen := make(map[string]bool, len(attests))
	for _, a := range attests {
		if seen[a.Xpub] {
			return fmt.Errorf("attestation set repeats a key")
		}
		seen[a.Xpub] = true
	}
	for _, x := range rec.Xpubs {
		a, ok := attestFor(attests, x)
		if !ok {
			return fmt.Errorf("no cosigner attestation for one of the wallet's keys")
		}
		if err := VerifyAttest(x, message, a.Sig, params); err != nil {
			return err
		}
	}
	if len(attests) != len(rec.Xpubs) {
		return fmt.Errorf("attestation set covers %d keys, the wallet has %d", len(attests), len(rec.Xpubs))
	}
	return nil
}

func attestFor(attests []RosterAttest, xpub string) (RosterAttest, bool) {
	for _, a := range attests {
		if a.Xpub == xpub {
			return a, true
		}
	}
	return RosterAttest{}, false
}
