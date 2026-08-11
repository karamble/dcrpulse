// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// buildTestTSpend serializes a structurally valid TSpend that embeds the
// given Pi pubkey in its signature script and payload in its OP_RETURN.
func buildTestTSpend(t *testing.T, pubKey, payload []byte) string {
	t.Helper()
	mtx := wire.NewMsgTx()
	mtx.Version = wire.TxVersionTreasury
	sig := append([]byte{0x40}, make([]byte, 64)...)
	sig = append(sig, 0x21)
	sig = append(sig, pubKey...)
	sig = append(sig, 0xc2)
	mtx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0xffffffff, Tree: wire.TxTreeRegular},
		SignatureScript:  sig,
		ValueIn:          wire.NullValueIn,
		BlockHeight:      wire.NullBlockHeight,
		BlockIndex:       wire.NullBlockIndex,
	})
	mtx.AddTxOut(wire.NewTxOut(0, append([]byte{0x6a, 0x20}, payload...)))
	pay := append([]byte{0xc3, 0x76, 0xa9, 0x14}, make([]byte, 20)...)
	pay = append(pay, 0x88, 0xac)
	mtx.AddTxOut(wire.NewTxOut(1e8, pay))
	var buf bytes.Buffer
	if err := mtx.Serialize(&buf); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return hex.EncodeToString(buf.Bytes())
}

func TestTSpendPiKey(t *testing.T) {
	pubKey := append([]byte{0x02}, bytes.Repeat([]byte{0x7b}, 32)...)
	payload := bytes.Repeat([]byte{0x5a}, 32)

	valid := buildTestTSpend(t, pubKey, payload)
	got := tspendPiKey(valid)
	if want := hex.EncodeToString(pubKey); got != want {
		t.Fatalf("pi key = %q, want %q", got, want)
	}
	if len(got) != 66 {
		t.Fatalf("pi key length = %d, want 66", len(got))
	}
	if got == hex.EncodeToString(payload) {
		t.Fatal("pi key must not be the OP_RETURN payload")
	}

	// Garbage, truncated, and non-tspend inputs stay empty.
	if tspendPiKey("zz") != "" {
		t.Fatal("invalid hex must return empty")
	}
	if tspendPiKey(valid[:len(valid)-2]) != "" {
		t.Fatal("truncated tx must return empty")
	}
	vote := buildTestVote(t, chainhash.Hash{0x01}, 1, 0x0001, 10, nil)
	if tspendPiKey(vote) != "" {
		t.Fatal("non-tspend tx must return empty")
	}
}
