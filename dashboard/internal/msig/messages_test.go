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
	if TTLFor(TypeSignReq) >= TTLFor(TypeSig) {
		t.Fatalf("sign_req TTL must be the short one")
	}
	if TTLFor(TypeInvite) != 7*24*time.Hour {
		t.Fatalf("handshake TTL should match the BR delivery horizon")
	}
}
