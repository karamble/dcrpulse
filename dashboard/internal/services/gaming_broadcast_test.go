// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"

	"bytes"
	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"encoding/hex"
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

// The bytes a real table actually hands over, and the reason this file exists.
//
// Every other case here builds its own signatures, which is how the rule came to
// be wrong in a way nothing caught: sigLen said 64, the fixtures were built from
// sigLen, and so the test and the code agreed with each other while disagreeing
// with the escrow. A schnorr signature is 64 bytes and what goes in a signature
// script is 65 - the signature and the sighash type byte after it.
//
// The consequence was not subtle. signaturesIn counted no signatures at all, so
// outputsMayPayAnyone was false for everything, so a table could not pay its
// winner and could not hand a bond back to anybody but the person running this
// wallet. Two live tables were stranded by it.
//
// These are captured from pkg/escrow itself. They must not be regenerated from
// anything in this package, because that is exactly the mistake being fixed.
const (
	realAliveSigScript    = "417af65a0dde09a82e4094c8172c505910e141264d4768fa1680a3cab24e77e66a0b48ff56605f50e83d61840d95567acc19b888934dd6cbbf6f04b098c8f039a80141041b08ce7fa14ed6e1de2414767ef0150d1b9f0352fd516768b577d6cadc70fbd8799e6aeb43012154c1f37427853564fbdf61712fdf4f79ea98bd9ca0e2b1f101514c9f632102a42612f5da95ce742e9b10bb51eb6616c7138013a4822bf802a7de5022839bff52bf2103e59b7836fdba1f1425e11a8874d7bb8c225d1c15c8ad172276a45258f272849952bf676353b2752102a42612f5da95ce742e9b10bb51eb6616c7138013a4822bf802a7de5022839bff52bf6702e007b2752103e59b7836fdba1f1425e11a8874d7bb8c225d1c15c8ad172276a45258f272849952bf686851"
	realBackstopSigScript = "417af65a0dde09a82e4094c8172c505910e141264d4768fa1680a3cab24e77e66a0b48ff56605f50e83d61840d95567acc19b888934dd6cbbf6f04b098c8f039a80100004c9f632102a42612f5da95ce742e9b10bb51eb6616c7138013a4822bf802a7de5022839bff52bf2103e59b7836fdba1f1425e11a8874d7bb8c225d1c15c8ad172276a45258f272849952bf676353b2752102a42612f5da95ce742e9b10bb51eb6616c7138013a4822bf802a7de5022839bff52bf6702e007b2752103e59b7836fdba1f1425e11a8874d7bb8c225d1c15c8ad172276a45258f272849952bf686851"
)

func fromHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return b
}

// A bond released the way a table that ends properly releases it: every member
// signs, and it may pay the member whose bond it is.
func TestARealCoSignedSpendIsSeenAsCoSigned(t *testing.T) {
	got := signaturesIn(fromHex(t, realAliveSigScript))
	if got != 2 {
		t.Fatalf("a real two-member release counts %d signatures, want 2", got)
	}
	if !outputsMayPayAnyone(txSpending(fromHex(t, realAliveSigScript))) {
		t.Fatal("a real co-signed spend was refused permission to pay anyone")
	}
}

// And the branch one key can take alone still must not.
func TestARealUnilateralSpendMustStillComeHome(t *testing.T) {
	if got := signaturesIn(fromHex(t, realBackstopSigScript)); got != 1 {
		t.Fatalf("a real backstop counts %d signatures, want 1", got)
	}
	if outputsMayPayAnyone(txSpending(fromHex(t, realBackstopSigScript))) {
		t.Fatal("a spend one key can make alone was allowed to pay anyone")
	}
}

// Coin that was never this wallet's may leave, so long as all of it leaves.
//
// This is the shape an accusation answer takes: one bond spent, alone, straight
// into a fresh one. The seat holding it has about fifteen minutes to send it
// before the others may take the bond, so a bridge that refused it would cost
// somebody real money while looking like a policy decision.
//
// Note what these do not exercise: walletOwns needs a wallet gRPC client and
// answers "not mine" without one, so the not-the-wallet's half is satisfied
// trivially here. What is under test is the arithmetic and the script class.
func TestCoinPassingThroughIsAllowedOut(t *testing.T) {
	tx := txSpending(unilateralSpend(t))
	tx.TxOut = []*wire.TxOut{{Value: 990_000}}
	decoded := scriptHashOutputs(1)

	if !passesThrough(context.Background(), tx, decoded, 1_000_000) {
		t.Fatal("a bond spent straight into another bond was refused; the seat that had to send " +
			"it loses the bond it was answering for")
	}
}

// Most of it leaving is not all of it leaving.
func TestCoinPassingThroughMayNotBeSkimmed(t *testing.T) {
	tx := txSpending(unilateralSpend(t))
	tx.TxOut = []*wire.TxOut{{Value: 400_000}}
	decoded := scriptHashOutputs(1)

	// 600_000 atoms unaccounted for. A fee this size is not a fee.
	if passesThrough(context.Background(), tx, decoded, 1_000_000) {
		t.Fatal("a transaction that left 600000 atoms behind was let out; a game could hand its " +
			"own bond to a miner a slice at a time")
	}
}

// Coin may pass through, but only into another script - never to an address a
// game picked, which is how it would pay itself.
func TestCoinPassingThroughMayNotGoToAPlainAddress(t *testing.T) {
	tx := txSpending(unilateralSpend(t))
	tx.TxOut = []*wire.TxOut{{Value: 990_000}}

	decoded := &pb.DecodedTransaction{Outputs: []*pb.DecodedTransaction_Output{{
		ScriptClass: pb.DecodedTransaction_Output_PUB_KEY_HASH,
		Addresses:   []string{"Dsomebodyelse"},
	}}}
	if passesThrough(context.Background(), tx, decoded, 1_000_000) {
		t.Fatal("a game moved coin to a plain address of its own choosing, which is the whole " +
			"thing this rule exists to refuse")
	}
}

// A transaction with nothing coming in cannot conserve anything.
//
// The second case is the one that needs saying: nothing in and nothing out
// balances perfectly, so the arithmetic alone would wave it through. Whatever
// that transaction is, it is not coin passing through, and the only reason it
// is refused is that this is checked separately.
func TestCoinPassingThroughNeedsKnownInputs(t *testing.T) {
	tx := txSpending(unilateralSpend(t))
	tx.TxOut = []*wire.TxOut{{Value: 990_000}}
	if passesThrough(context.Background(), tx, scriptHashOutputs(1), 0) {
		t.Fatal("a transaction whose inputs are worth nothing known was let out on the strength " +
			"of arithmetic nobody could do")
	}

	empty := txSpending(unilateralSpend(t))
	empty.TxOut = []*wire.TxOut{{Value: 0}}
	if passesThrough(context.Background(), empty, scriptHashOutputs(1), 0) {
		t.Fatal("nothing in and nothing out balanced, and was let out; the fee arithmetic " +
			"cannot be the only thing standing here")
	}
}

// scriptHashOutputs is n outputs paying script hashes nobody here holds.
func scriptHashOutputs(n int) *pb.DecodedTransaction {
	out := &pb.DecodedTransaction{}
	for i := 0; i < n; i++ {
		out.Outputs = append(out.Outputs, &pb.DecodedTransaction_Output{
			ScriptClass: pb.DecodedTransaction_Output_SCRIPT_HASH,
			Addresses:   []string{"Tsomescripthash"},
		})
	}
	return out
}
