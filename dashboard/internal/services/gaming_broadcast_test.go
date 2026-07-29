// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"testing"

	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
)

// The scripts are built here rather than imported from the game, because the
// point of the rule is that this file does not know what game it is relaying
// for. If these ever need the poker package to be written, the rule has stopped
// being about shape.

func sig(t *testing.T, fill byte) []byte {
	t.Helper()
	return bytes.Repeat([]byte{fill}, sigLen)
}

// coSignedSpend is what a settlement hands over: one signature per member, then
// the branch selector, then the script they satisfy.
func coSignedSpend(t *testing.T, members int) []byte {
	t.Helper()
	b := txscript.NewScriptBuilder()
	for i := range members {
		b.AddData(sig(t, byte(0xa0+i)))
	}
	b.AddOp(txscript.OP_1)
	b.AddData(bytes.Repeat([]byte{0x51}, 40))
	script, err := b.Script()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return script
}

// unilateralSpend is what a refund or a sweep hands over: one signature, the
// other branch, and the script.
func unilateralSpend(t *testing.T) []byte {
	t.Helper()
	b := txscript.NewScriptBuilder()
	b.AddData(sig(t, 0xb0))
	b.AddOp(txscript.OP_0)
	b.AddData(bytes.Repeat([]byte{0x51}, 40))
	script, err := b.Script()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return script
}

func txSpending(scripts ...[]byte) *wire.MsgTx {
	tx := wire.NewMsgTx()
	for _, s := range scripts {
		tx.AddTxIn(&wire.TxIn{SignatureScript: s})
	}
	tx.AddTxOut(&wire.TxOut{Value: 1})
	return tx
}

func TestCountingWhoHadToAgree(t *testing.T) {
	if got := signaturesIn(coSignedSpend(t, 2)); got != 2 {
		t.Errorf("a two member settlement counted %d signatures, want 2", got)
	}
	if got := signaturesIn(coSignedSpend(t, 6)); got != 6 {
		t.Errorf("a six member settlement counted %d signatures, want 6", got)
	}
	if got := signaturesIn(unilateralSpend(t)); got != 1 {
		t.Errorf("a refund counted %d signatures, want 1", got)
	}
	// Nothing to count is not something to let through.
	for _, junk := range [][]byte{nil, {}, {0x51}, bytes.Repeat([]byte{0xff}, 8)} {
		if got := signaturesIn(junk); got != 0 {
			t.Errorf("%x counted %d signatures, want 0", junk, got)
		}
	}
}

// Coin a game could move on its own has to come back to the person who funded
// it, because the game holds that key and nothing else stands in the way.
func TestAUnilateralSpendMustComeHome(t *testing.T) {
	if outputsMayPayAnyone(txSpending(unilateralSpend(t))) {
		t.Fatal("a refund was allowed to pay somebody else, and a game holding the " +
			"only key needed could then pay itself")
	}
}

// Coin that took every member of a table to move may go where they agreed,
// because they have already agreed and any of them can relay it anyway.
func TestACoSignedSpendMayPayTheWinner(t *testing.T) {
	if !outputsMayPayAnyone(txSpending(coSignedSpend(t, 2), coSignedSpend(t, 2))) {
		t.Fatal("a settlement every member signed was refused, so a table cannot pay out")
	}
}

// The one that matters: a co-signed input must not carry a unilateral one out
// of the wallet beside it.
func TestOneCoSignedInputDoesNotExcuseTheRest(t *testing.T) {
	mixed := txSpending(coSignedSpend(t, 2), unilateralSpend(t))
	if outputsMayPayAnyone(mixed) {
		t.Fatal("a transaction mixing a settlement with a unilateral spend was allowed " +
			"to pay anywhere, so the second one leaves the wallet on the back of the first")
	}
}

func TestSpendingNothingIsNotCoSigned(t *testing.T) {
	if outputsMayPayAnyone(wire.NewMsgTx()) {
		t.Fatal("a transaction with no inputs was treated as co-signed")
	}
}
