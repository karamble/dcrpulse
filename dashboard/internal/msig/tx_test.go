// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"decred.org/dcrwallet/v5/wallet/txrules"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/sign"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

// testWallet emulates the cosigners of one shared wallet. Signing goes
// through txscript/sign, the same signer dcrwallet uses internally, so the
// signature script shapes asserted here are exactly what a live wallet
// produces.
type testWallet struct {
	t        *testing.T
	params   *chaincfg.Params
	m        int
	redeem   []byte
	sorted   [][]byte
	addr     string
	pkVers   uint16
	pkScript []byte
	privs    map[string]*secp256k1.PrivateKey // pubkey address -> key
	addrOf   []string                         // aligned with sorted
}

func newTestWallet(t *testing.T, m, n int) *testWallet {
	t.Helper()
	params := chaincfg.MainNetParams()
	byHex := make(map[string]*secp256k1.PrivateKey, n)
	pubs := make([][]byte, 0, n)
	for range n {
		priv, err := secp256k1.GeneratePrivateKey()
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		pub := priv.PubKey().SerializeCompressed()
		pubs = append(pubs, pub)
		byHex[hex.EncodeToString(pub)] = priv
	}
	redeem, sorted, err := MultiSigScript(m, pubs)
	if err != nil {
		t.Fatalf("build script: %v", err)
	}
	a, err := stdaddr.NewAddressScriptHashV0(redeem, params)
	if err != nil {
		t.Fatalf("address: %v", err)
	}
	vers, pkScript := a.PaymentScript()

	w := &testWallet{
		t: t, params: params, m: m, redeem: redeem, sorted: sorted,
		addr: a.String(), pkVers: vers, pkScript: pkScript,
		privs:  make(map[string]*secp256k1.PrivateKey, n),
		addrOf: make([]string, n),
	}
	for i, pub := range sorted {
		parsed, err := secp256k1.ParsePubKey(pub)
		if err != nil {
			t.Fatalf("parse key: %v", err)
		}
		pa, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(parsed, params)
		if err != nil {
			t.Fatalf("pubkey address: %v", err)
		}
		w.privs[pa.String()] = byHex[hex.EncodeToString(pub)]
		w.addrOf[i] = pa.String()
	}
	return w
}

// kdbFor exposes only the chosen signers' keys, emulating cosigners that
// each hold a single key of the roster.
func (w *testWallet) kdbFor(signers ...int) sign.KeyDB {
	allowed := make(map[string]bool, len(signers))
	for _, i := range signers {
		allowed[w.addrOf[i]] = true
	}
	return sign.KeyClosure(func(a stdaddr.Address) ([]byte, dcrec.SignatureType, bool, error) {
		if !allowed[a.String()] {
			return nil, 0, false, errNoKey
		}
		priv, ok := w.privs[a.String()]
		if !ok {
			return nil, 0, false, errNoKey
		}
		return priv.Serialize(), dcrec.STEcdsaSecp256k1, true, nil
	})
}

var errNoKey = errors.New("key not held by this signer")

func (w *testWallet) sdb() sign.ScriptDB {
	return sign.ScriptClosure(func(stdaddr.Address) ([]byte, error) {
		return w.redeem, nil
	})
}

func (w *testWallet) signInput(tx *wire.MsgTx, idx, signer int) {
	w.t.Helper()
	prev := tx.TxIn[idx].SignatureScript
	script, err := sign.SignTxOutput(w.params, tx, idx, w.pkScript,
		txscript.SigHashAll, w.kdbFor(signer), w.sdb(), prev, false)
	if err != nil {
		w.t.Fatalf("sign input %d with signer %d: %v", idx, signer, err)
	}
	tx.TxIn[idx].SignatureScript = script
}

func (w *testWallet) fundingTx(values ...int64) *wire.MsgTx {
	tx := wire.NewMsgTx()
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{}, 0, nil))
	for _, v := range values {
		tx.AddTxOut(&wire.TxOut{Value: v, Version: w.pkVers, PkScript: w.pkScript})
	}
	return tx
}

func (w *testWallet) spendTx(funding *wire.MsgTx, outValue int64, vouts ...uint32) *wire.MsgTx {
	tx := wire.NewMsgTx()
	h := funding.TxHash()
	for _, vout := range vouts {
		op := wire.OutPoint{Hash: h, Index: vout, Tree: 0}
		tx.AddTxIn(wire.NewTxIn(&op, funding.TxOut[vout].Value, nil))
	}
	tx.AddTxOut(&wire.TxOut{Value: outValue, Version: w.pkVers, PkScript: w.pkScript})
	return tx
}

func (w *testWallet) engineErr(tx *wire.MsgTx, idx int) error {
	flags := txscript.ScriptDiscourageUpgradableNops |
		txscript.ScriptVerifyCleanStack |
		txscript.ScriptVerifySigPushOnly
	vm, err := txscript.NewEngine(w.pkScript, tx, idx, flags, 0, nil)
	if err != nil {
		return err
	}
	return vm.Execute()
}

func pushShapes(t *testing.T, sigScript []byte) (nonEmpty, empty int) {
	t.Helper()
	tok := txscript.MakeScriptTokenizer(0, sigScript)
	for tok.Next() {
		if len(tok.Data()) > 0 {
			nonEmpty++
		} else {
			empty++
		}
	}
	if err := tok.Err(); err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	return
}

func TestPartialSignatureShapes(t *testing.T) {
	w := newTestWallet(t, 2, 3)
	funding := w.fundingTx(100_000_000)
	tx := w.spendTx(funding, 99_990_000, 0)
	txid := tx.TxHash().String()

	// First signature: [sig][redeem], no OP_0 padding on Decred.
	w.signInput(tx, 0, 1)
	if got := tx.TxHash().String(); got != txid {
		t.Fatalf("txid changed after signing: %s", got)
	}
	count, err := CountSigs(tx.TxIn[0].SignatureScript, w.redeem)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count after first signature: got %d, want 1", count)
	}
	nonEmpty, empty := pushShapes(t, tx.TxIn[0].SignatureScript)
	if nonEmpty != 2 || empty != 0 { // sig + redeem push
		t.Fatalf("first-signature shape: %d data pushes, %d empty", nonEmpty, empty)
	}
	signers, err := VerifyProposalUpdate(tx, txid, w.redeem, w.sorted)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(signers) != 1 || !signers[hex.EncodeToString(w.sorted[1])] {
		t.Fatalf("attribution after first signature: %v", signers)
	}

	// Second signature out of roster order completes 2-of-3 with no
	// placeholder and the script engine accepts the input.
	w.signInput(tx, 0, 0)
	signers, err = VerifyProposalUpdate(tx, txid, w.redeem, w.sorted)
	if err != nil {
		t.Fatalf("verify complete: %v", err)
	}
	if len(signers) != 2 {
		t.Fatalf("signer count after completion: %d", len(signers))
	}
	nonEmpty, empty = pushShapes(t, tx.TxIn[0].SignatureScript)
	if nonEmpty != 3 || empty != 0 {
		t.Fatalf("complete shape: %d data pushes, %d empty", nonEmpty, empty)
	}
	if err := w.engineErr(tx, 0); err != nil {
		t.Fatalf("script engine rejects completed input: %v", err)
	}
}

func TestMergePaddingThreeOfThree(t *testing.T) {
	w := newTestWallet(t, 3, 3)
	funding := w.fundingTx(100_000_000)
	tx := w.spendTx(funding, 99_990_000, 0)
	txid := tx.TxHash().String()

	w.signInput(tx, 0, 0)
	w.signInput(tx, 0, 1)
	count, err := CountSigs(tx.TxIn[0].SignatureScript, w.redeem)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("count at two of three: got %d", count)
	}
	// The merge pads the missing third signature with an OP_0
	// placeholder; the input must not validate yet.
	nonEmpty, empty := pushShapes(t, tx.TxIn[0].SignatureScript)
	if empty != 1 || nonEmpty != 3 {
		t.Fatalf("padded shape: %d data pushes, %d empty", nonEmpty, empty)
	}
	if err := w.engineErr(tx, 0); err == nil {
		t.Fatalf("script engine accepted an incomplete 3-of-3 input")
	}

	w.signInput(tx, 0, 2)
	signers, err := VerifyProposalUpdate(tx, txid, w.redeem, w.sorted)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(signers) != 3 {
		t.Fatalf("signer count: %d", len(signers))
	}
	nonEmpty, empty = pushShapes(t, tx.TxIn[0].SignatureScript)
	if empty != 0 || nonEmpty != 4 {
		t.Fatalf("final shape: %d data pushes, %d empty", nonEmpty, empty)
	}
	if err := w.engineErr(tx, 0); err != nil {
		t.Fatalf("script engine rejects completed input: %v", err)
	}
}

func TestVerifyProposalUpdateTamper(t *testing.T) {
	w := newTestWallet(t, 2, 3)
	funding := w.fundingTx(100_000_000)
	tx := w.spendTx(funding, 99_990_000, 0)
	txid := tx.TxHash().String()
	w.signInput(tx, 0, 0)

	// Any prefix mutation changes the txid.
	tampered := tx.Copy()
	tampered.TxOut[0].Value++
	if _, err := VerifyProposalUpdate(tampered, txid, w.redeem, w.sorted); err == nil {
		t.Fatalf("prefix tamper not detected")
	}

	// A signature from a key outside the roster must be rejected.
	foreign, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	foreignTx := w.spendTx(funding, 99_990_000, 0)
	fsig, err := sign.RawTxInSignature(foreignTx, 0, w.redeem, txscript.SigHashAll,
		foreign.Serialize(), dcrec.STEcdsaSecp256k1)
	if err != nil {
		t.Fatalf("foreign signature: %v", err)
	}
	b := txscript.NewScriptBuilder().AddData(fsig).AddData(w.redeem)
	foreignScript, err := b.Script()
	if err != nil {
		t.Fatalf("build script: %v", err)
	}
	foreignTx.TxIn[0].SignatureScript = foreignScript
	if _, err := VerifyProposalUpdate(foreignTx, foreignTx.TxHash().String(), w.redeem, w.sorted); err == nil {
		t.Fatalf("non-member signature not detected")
	}

	// Junk that does not parse as DER must be rejected.
	junkTx := w.spendTx(funding, 99_990_000, 0)
	junk := bytes.Repeat([]byte{0x42}, 70)
	b = txscript.NewScriptBuilder().AddData(junk).AddData(w.redeem)
	junkScript, err := b.Script()
	if err != nil {
		t.Fatalf("build script: %v", err)
	}
	junkTx.TxIn[0].SignatureScript = junkScript
	if _, err := VerifyProposalUpdate(junkTx, junkTx.TxHash().String(), w.redeem, w.sorted); err == nil {
		t.Fatalf("junk signature not detected")
	}
}

func TestCountSigsWrongRedeem(t *testing.T) {
	w := newTestWallet(t, 2, 2)
	other := newTestWallet(t, 2, 2)
	funding := w.fundingTx(100_000_000)
	tx := w.spendTx(funding, 99_990_000, 0)
	w.signInput(tx, 0, 0)
	if _, err := CountSigs(tx.TxIn[0].SignatureScript, other.redeem); err == nil {
		t.Fatalf("wrong redeem script not detected")
	}
	if n, err := CountSigs(nil, w.redeem); err != nil || n != 0 {
		t.Fatalf("empty script: n=%d err=%v", n, err)
	}
}

func TestEstimateSizeCoversActual(t *testing.T) {
	w := newTestWallet(t, 2, 3)
	funding := w.fundingTx(60_000_000, 40_000_000)
	tx := w.spendTx(funding, 99_990_000, 0, 1)
	skeleton := tx.Copy()

	for idx := range tx.TxIn {
		w.signInput(tx, idx, 0)
		w.signInput(tx, idx, 2)
	}
	est := EstimateSize(skeleton, w.m, len(w.redeem))
	actual := tx.SerializeSize()
	if actual > est {
		t.Fatalf("estimate %d below actual %d", est, actual)
	}
	if est-actual > 20*len(tx.TxIn) {
		t.Fatalf("estimate %d too loose for actual %d", est, actual)
	}
}

// TestEstSigScriptLenExact pins the estimate to the exact length of a
// worst-case redeemed signature script. TestEstimateSizeCoversActual asserts
// only an inequality, which real signatures satisfy with room to spare, so it
// cannot see a one-byte error in the formula itself.
func TestEstSigScriptLenExact(t *testing.T) {
	tests := []struct{ m, n int }{
		{1, 1}, // redeem 37, one-byte push
		{2, 2}, // redeem 71, one-byte push
		{2, 7}, // redeem 241, two-byte push
		{2, 8}, // redeem 275, three-byte push, the MaxPubKeys cap
	}

	for _, test := range tests {
		w := newTestWallet(t, test.m, test.n)

		// The shape a signer emits: the CHECKMULTISIG dummy, m maximum
		// DER signatures each with a hash type byte, then the redeem push.
		b := txscript.NewScriptBuilder().AddOp(txscript.OP_0)
		for range test.m {
			b.AddData(make([]byte, 73))
		}
		b.AddData(w.redeem)
		script, err := b.Script()
		if err != nil {
			t.Fatalf("%d-of-%d: build script: %v", test.m, test.n, err)
		}

		if got := estSigScriptLen(test.m, len(w.redeem)); got != len(script) {
			t.Errorf("%d-of-%d: estSigScriptLen = %d, want %d (redeem %d)",
				test.m, test.n, got, len(script), len(w.redeem))
		}
	}
}

func TestBuildSpendFeeAndChange(t *testing.T) {
	w := newTestWallet(t, 2, 3)
	funding := w.fundingTx(100_000_000)
	utxo := UTXO{TxID: funding.TxHash().String(), Vout: 0, Tree: 0, Atoms: 100_000_000}

	tx, fee, change, err := BuildSpend(BuildSpendParams{
		UTXOs:         []UTXO{utxo},
		Recipients:    []Recipient{{Address: w.addr, Atoms: 50_000_000}},
		ChangeAddress: w.addr,
		RedeemScript:  w.redeem,
		ChainParams:   w.params,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(tx.TxOut) != 2 {
		t.Fatalf("output count: %d", len(tx.TxOut))
	}
	if !bytes.Equal(tx.TxOut[1].PkScript, w.pkScript) {
		t.Fatalf("change does not pay the shared address")
	}
	if change != 100_000_000-50_000_000-fee {
		t.Fatalf("change arithmetic: fee=%d change=%d", fee, change)
	}
	wantFee := int64(txrules.FeeForSerializeSize(txrules.DefaultRelayFeePerKb,
		EstimateSize(tx, w.m, len(w.redeem))))
	if fee != wantFee {
		t.Fatalf("fee %d, want %d", fee, wantFee)
	}

	// A leftover below the dust threshold folds into the fee.
	tx, fee, change, err = BuildSpend(BuildSpendParams{
		UTXOs:         []UTXO{utxo},
		Recipients:    []Recipient{{Address: w.addr, Atoms: 100_000_000 - 5_000}},
		ChangeAddress: w.addr,
		RedeemScript:  w.redeem,
		ChainParams:   w.params,
	})
	if err != nil {
		t.Fatalf("build dust case: %v", err)
	}
	if len(tx.TxOut) != 1 || change != 0 || fee != 5_000 {
		t.Fatalf("dust fold: outs=%d fee=%d change=%d", len(tx.TxOut), fee, change)
	}

	// Recipients above the input value must fail.
	_, _, _, err = BuildSpend(BuildSpendParams{
		UTXOs:         []UTXO{utxo},
		Recipients:    []Recipient{{Address: w.addr, Atoms: 100_000_001}},
		ChangeAddress: w.addr,
		RedeemScript:  w.redeem,
		ChainParams:   w.params,
	})
	if err == nil {
		t.Fatalf("insufficient funds not detected")
	}
}

func TestSelectUTXOs(t *testing.T) {
	utxos := []UTXO{
		{TxID: "01", Vout: 0, Atoms: 100_000_000},
		{TxID: "02", Vout: 0, Atoms: 500_000_000},
		{TxID: "03", Vout: 0, Atoms: 300_000_000},
	}
	sel, err := SelectUTXOs(utxos, 600_000_000, 1, 2, 3, 0)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(sel) != 2 || sel[0].Atoms != 500_000_000 || sel[1].Atoms != 300_000_000 {
		t.Fatalf("selection: %+v", sel)
	}
	if _, err := SelectUTXOs(utxos, 900_000_000, 1, 2, 3, 0); err == nil {
		t.Fatalf("insufficient funds not detected")
	}

	many := make([]UTXO, MaxInputs+1)
	for i := range many {
		many[i] = UTXO{TxID: "aa", Vout: uint32(i), Atoms: 100_000_000}
	}
	if _, err := SelectUTXOs(many, int64(MaxInputs)*100_000_000+50_000_000, 1, 2, 3, 0); err == nil {
		t.Fatalf("input cap not enforced")
	}
}

func TestDecodeTxHexStrict(t *testing.T) {
	w := newTestWallet(t, 2, 2)
	tx := w.fundingTx(1_000_000)
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	valid := hex.EncodeToString(buf.Bytes())
	decoded, err := DecodeTxHex(valid)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.TxHash() != tx.TxHash() {
		t.Fatalf("roundtrip txid mismatch")
	}
	if _, err := DecodeTxHex(valid + "00"); err == nil {
		t.Fatalf("trailing bytes not detected")
	}
	if _, err := DecodeTxHex("zz"); err == nil {
		t.Fatalf("bad hex not detected")
	}
}
