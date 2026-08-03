// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func testPubKeyHex(t *testing.T) string {
	t.Helper()
	priv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return hex.EncodeToString(priv.PubKey().SerializeCompressed())
}

func sortedPubKeyHexes(t *testing.T, n int) []string {
	t.Helper()
	keys := make([][]byte, n)
	for i := range keys {
		pk, err := hex.DecodeString(testPubKeyHex(t))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		keys[i] = pk
	}
	sorted := SortPubKeys(keys)
	out := make([]string, n)
	for i, k := range sorted {
		out[i] = hex.EncodeToString(k)
	}
	return out
}

func TestMessageRoundtrips(t *testing.T) {
	pks := sortedPubKeyHexes(t, 3)
	txid := strings.Repeat("ab", 32)
	valid := []*Message{
		{Type: TypeInvite, TempID: "00aabbcc", Label: "team", M: 2, N: 3, Network: "mainnet", PubKey: pks[0]},
		{Type: TypeAccept, TempID: "00aabbcc", PubKey: pks[1]},
		{Type: TypeDecline, TempID: "00aabbcc", Reason: "not now"},
		{Type: TypeRoster, TempID: "00aabbcc", Label: "team", M: 2, N: 3, Network: "mainnet",
			PubKeys: pks, Script: "5221aa52ae", Address: "DcSharedAddr"},
		{Type: TypeReady, TempID: "00aabbcc", WalletID: "DcSharedAddr"},
		{Type: TypeInviteCancel, TempID: "00aabbcc"},
		{Type: TypeSignReq, WalletID: "DcSharedAddr", TxID: txid, RawTx: "0100", SigsHave: 1, Note: "rent"},
		{Type: TypeSig, WalletID: "DcSharedAddr", TxID: txid, RawTx: "0100"},
		{Type: TypeSigDecline, WalletID: "DcSharedAddr", TxID: txid, Reason: "inputs in use"},
		{Type: TypeBroadcast, WalletID: "DcSharedAddr", TxID: txid},
	}
	for _, m := range valid {
		payload, err := EncodeMessage(m)
		if err != nil {
			t.Fatalf("%s: encode: %v", m.Type, err)
		}
		back, err := DecodeMessage(payload)
		if err != nil {
			t.Fatalf("%s: decode: %v", m.Type, err)
		}
		if back.Type != m.Type {
			t.Fatalf("%s: type roundtrip", m.Type)
		}
	}
}

func TestMessageValidation(t *testing.T) {
	pks := sortedPubKeyHexes(t, 3)
	txid := strings.Repeat("ab", 32)
	reversed := []string{pks[2], pks[1], pks[0]}
	dup := []string{pks[0], pks[0], pks[1]}

	invalid := []*Message{
		{Type: ""},
		{Type: TypeInvite, TempID: "XYZ", Label: "l", M: 2, N: 2, Network: "mainnet", PubKey: pks[0]},
		{Type: TypeInvite, TempID: "00aa", Label: "", M: 2, N: 2, Network: "mainnet", PubKey: pks[0]},
		{Type: TypeInvite, TempID: "00aa", Label: "l", M: 3, N: 2, Network: "mainnet", PubKey: pks[0]},
		{Type: TypeInvite, TempID: "00aa", Label: "l", M: 2, N: 2, Network: "moonnet", PubKey: pks[0]},
		{Type: TypeInvite, TempID: "00aa", Label: "l", M: 2, N: 2, Network: "mainnet", PubKey: "aa"},
		{Type: TypeInvite, TempID: "00aa", Label: strings.Repeat("x", MaxLabelLen+1), M: 2, N: 2, Network: "mainnet", PubKey: pks[0]},
		{Type: TypeRoster, TempID: "00aa", Label: "l", M: 2, N: 3, Network: "mainnet", PubKeys: reversed, Script: "52ae", Address: "Dc"},
		{Type: TypeRoster, TempID: "00aa", Label: "l", M: 2, N: 3, Network: "mainnet", PubKeys: dup, Script: "52ae", Address: "Dc"},
		{Type: TypeRoster, TempID: "00aa", Label: "l", M: 2, N: 3, Network: "mainnet", PubKeys: pks[:2], Script: "52ae", Address: "Dc"},
		{Type: TypeRoster, TempID: "00aa", Label: "l", M: 2, N: 3, Network: "mainnet", PubKeys: pks, Script: "", Address: "Dc"},
		{Type: TypeSignReq, WalletID: "Dc", TxID: "abcd", RawTx: "0100"},
		{Type: TypeSignReq, WalletID: "", TxID: txid, RawTx: "0100"},
		{Type: TypeSignReq, WalletID: "Dc", TxID: txid, RawTx: "GG"},
		{Type: TypeSignReq, WalletID: "Dc", TxID: txid, RawTx: strings.Repeat("ab", MaxRawTxHex)},
		{Type: TypeSignReq, WalletID: "Dc", TxID: txid, RawTx: "0100", SigsHave: 16},
		{Type: TypeSig, WalletID: "Dc", TxID: txid, RawTx: ""},
		{Type: TypeBroadcast, WalletID: "Dc", TxID: "zz"},
		{Type: TypeReady, TempID: "00aa", WalletID: ""},
	}
	for i, m := range invalid {
		if err := ValidateMessage(m); err == nil {
			t.Errorf("case %d (%s): expected validation error", i, m.Type)
		}
	}
}

func TestMessageUnknownTypeAndFields(t *testing.T) {
	msg, err := DecodeMessage([]byte(`{"type":"future_thing","tempId":"00aa","newField":true}`))
	if !errors.Is(err, ErrUnknownType) {
		t.Fatalf("expected ErrUnknownType, got %v", err)
	}
	if msg == nil || msg.Type != "future_thing" {
		t.Fatalf("unknown-type message not preserved for journaling")
	}
	// Unknown fields on a known type are ignored.
	if _, err := DecodeMessage([]byte(`{"type":"invite_cancel","tempId":"00aa","extra":"ok"}`)); err != nil {
		t.Fatalf("unknown field rejected: %v", err)
	}
}

func TestMessageTTLs(t *testing.T) {
	if TTLFor(TypeSignReq, false) >= TTLFor(TypeSig, false) {
		t.Fatalf("sign_req TTL must be the short one")
	}
	if TTLFor(TypeInvite, false) != 7*24*time.Hour {
		t.Fatalf("handshake TTL should match the BR delivery horizon")
	}
}

func testRosterXpubs(t *testing.T, n int) []string {
	t.Helper()
	params := chaincfg.MainNetParams()
	names := []string{"alice", "bob", "carol", "dave"}
	xpubs := make([]string, 0, n)
	for i := 0; i < n; i++ {
		xpubs = append(xpubs, testXpub(t, names[i%len(names)], uint32(i), params))
	}
	return SortXpubs(xpubs)
}

func TestMessageHDForms(t *testing.T) {
	xp := testRosterXpubs(t, 3)
	valid := []*Message{
		{Type: TypeInvite, Ver: ProtoHD, TempID: "00aabbcc", Label: "team", M: 2, N: 3,
			Network: "mainnet", Xpub: xp[0]},
		{Type: TypeAccept, Ver: ProtoHD, TempID: "00aabbcc", Xpub: xp[1]},
		{Type: TypeRoster, Ver: ProtoHD, TempID: "00aabbcc", Label: "team", M: 2, N: 3,
			Network: "mainnet", Xpubs: xp, Address: "DcSharedAddr"},
	}
	for _, m := range valid {
		payload, err := EncodeMessage(m)
		if err != nil {
			t.Fatalf("%s v2 rejected: %v", m.Type, err)
		}
		back, err := DecodeMessage(payload)
		if err != nil {
			t.Fatalf("%s v2 decode: %v", m.Type, err)
		}
		if back.Ver != ProtoHD {
			t.Fatalf("%s lost its version", m.Type)
		}
	}
}

func TestMessageHDRejections(t *testing.T) {
	xp := testRosterXpubs(t, 3)
	pks := sortedPubKeyHexes(t, 3)
	invalid := []*Message{
		// Mixed forms, both directions.
		{Type: TypeInvite, Ver: ProtoHD, TempID: "00aabbcc", Label: "t", M: 2, N: 3,
			Network: "mainnet", Xpub: xp[0], PubKey: pks[0]},
		{Type: TypeInvite, TempID: "00aabbcc", Label: "t", M: 2, N: 3,
			Network: "mainnet", Xpub: xp[0]},
		{Type: TypeAccept, Ver: ProtoHD, TempID: "00aabbcc", Xpub: xp[0], PubKey: pks[0]},
		{Type: TypeAccept, TempID: "00aabbcc", Xpub: xp[0]},
		{Type: TypeRoster, Ver: ProtoHD, TempID: "00aabbcc", Label: "t", M: 2, N: 3,
			Network: "mainnet", Xpubs: xp, PubKeys: pks, Address: "A"},
		{Type: TypeRoster, Ver: ProtoHD, TempID: "00aabbcc", Label: "t", M: 2, N: 3,
			Network: "mainnet", Xpubs: xp, Script: "5221aa52ae", Address: "A"},
		// Missing the HD payload.
		{Type: TypeInvite, Ver: ProtoHD, TempID: "00aabbcc", Label: "t", M: 2, N: 3,
			Network: "mainnet"},
		// Unsorted and short rosters.
		{Type: TypeRoster, Ver: ProtoHD, TempID: "00aabbcc", Label: "t", M: 2, N: 3,
			Network: "mainnet", Xpubs: []string{xp[2], xp[0], xp[1]}, Address: "A"},
		{Type: TypeRoster, Ver: ProtoHD, TempID: "00aabbcc", Label: "t", M: 2, N: 3,
			Network: "mainnet", Xpubs: xp[:2], Address: "A"},
		// Future version.
		{Type: TypeInvite, Ver: 3, TempID: "00aabbcc", Label: "t", M: 2, N: 3,
			Network: "mainnet", Xpub: xp[0]},
	}
	for i, m := range invalid {
		if err := ValidateMessage(m); err == nil {
			t.Fatalf("case %d (%s ver %d) unexpectedly valid", i, m.Type, m.Ver)
		}
	}
}

// validateV1Rules replicates the pre-HD validation of the three handshake
// types verbatim. It freezes the compatibility contract: every valid HD
// frame MUST fail these rules, because that is what makes old builds drop
// the frame without journaling its mid, letting the post-upgrade history
// replay process it.
func validateV1Rules(m *Message) error {
	switch m.Type {
	case TypeInvite:
		if !validTempID(m.TempID) || m.Label == "" || !validParams(m.M, m.N) ||
			!validNetwork(m.Network) || !validPubKey(m.PubKey) {
			return errors.New("invalid under v1 rules")
		}
	case TypeAccept:
		if !validTempID(m.TempID) || !validPubKey(m.PubKey) {
			return errors.New("invalid under v1 rules")
		}
	case TypeRoster:
		if !validTempID(m.TempID) || m.Label == "" || !validParams(m.M, m.N) ||
			!validNetwork(m.Network) || len(m.PubKeys) != m.N ||
			m.Script == "" || !validWalletID(m.Address) {
			return errors.New("invalid under v1 rules")
		}
	}
	return nil
}

func TestHDFramesFailV1Rules(t *testing.T) {
	xp := testRosterXpubs(t, 2)
	hdFrames := []*Message{
		{Type: TypeInvite, Ver: ProtoHD, TempID: "00aabbcc", Label: "t", M: 2, N: 2,
			Network: "mainnet", Xpub: xp[0]},
		{Type: TypeAccept, Ver: ProtoHD, TempID: "00aabbcc", Xpub: xp[1]},
		{Type: TypeRoster, Ver: ProtoHD, TempID: "00aabbcc", Label: "t", M: 2, N: 2,
			Network: "mainnet", Xpubs: xp, Address: "DcSharedAddr"},
	}
	for _, m := range hdFrames {
		if err := ValidateMessage(m); err != nil {
			t.Fatalf("%s not valid under HD rules: %v", m.Type, err)
		}
		if err := validateV1Rules(m); err == nil {
			t.Fatalf("%s frame passes v1 rules; old builds would journal it as processed", m.Type)
		}
	}
}
