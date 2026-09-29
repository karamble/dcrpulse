// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"runtime"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// The reviewer's payloads: a full transaction header followed by a count of
// millions that no bytes back. 3,000,000 outputs and 700,000 inputs as
// 0xfe-prefixed varints.
var (
	tenByteOutputs = []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xc0, 0xc6, 0x2d, 0x00}
	nineByteInputs = []byte{0x01, 0x00, 0x00, 0x00, 0xfe, 0x60, 0xae, 0x0a, 0x00}
)

func allocatedBy(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func TestDeserializeTxRefusesCountsItsBytesCannotBack(t *testing.T) {
	for name, raw := range map[string][]byte{"outputs": tenByteOutputs, "inputs": nineByteInputs} {
		var ok bool
		alloc := allocatedBy(func() { _, _, ok = deserializeTx(raw) })
		if ok {
			t.Errorf("%s: a %d-byte transaction decoded", name, len(raw))
		}
		if alloc > 1<<20 {
			t.Errorf("%s: decoding %d bytes allocated %d bytes", name, len(raw), alloc)
		}
	}
}

func testTx(t *testing.T) *wire.MsgTx {
	t.Helper()
	tx := wire.NewMsgTx()
	for i := byte(0); i < 2; i++ {
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&chainhash.Hash{i + 1}, uint32(i), 0), 1e8, []byte{0x51, i}))
		tx.AddTxOut(wire.NewTxOut(5e7, []byte{0x76, 0xa9, i}))
	}
	return tx
}

func TestTxCountsFitAcceptsARealTransaction(t *testing.T) {
	raw, err := testTx(t).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := TxCountsFit(raw); err != nil {
		t.Fatalf("a real transaction was refused: %v", err)
	}
	if _, tx, ok := deserializeTx(raw); !ok || len(tx.TxIn) != 2 || len(tx.TxOut) != 2 {
		t.Fatal("a real transaction no longer decodes")
	}
}

func TestTxCountsFitRefusesAPrefixOnlySerialization(t *testing.T) {
	tx := testTx(t)
	tx.SerType = wire.TxSerializeNoWitness
	raw, err := tx.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if TxCountsFit(raw) == nil {
		t.Fatal("a prefix-only transaction passed")
	}
}
