// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"decred.org/dcrwallet/v5/wallet/txrules"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

// MaxInputs bounds a proposal's input count so the framed transaction
// stays far below the Bison Relay message floor.
const MaxInputs = 30

// UTXO identifies one unspent output of the shared P2SH address.
type UTXO struct {
	TxID  string
	Vout  uint32
	Tree  int8
	Atoms int64
}

// Recipient is one payment output of a proposal.
type Recipient struct {
	Address string
	Atoms   int64
}

// BuildSpendParams describes a shared-wallet spend to assemble.
type BuildSpendParams struct {
	UTXOs         []UTXO
	Recipients    []Recipient
	ChangeAddress string
	RedeemScript  []byte
	RelayFeePerKb dcrutil.Amount
	ChainParams   *chaincfg.Params
}

func redeemScriptSize(n int) int {
	// OP_m, n direct 33-byte pushes, OP_n, OP_CHECKMULTISIG.
	return 3 + 34*n
}

func dataPushOverhead(n int) int {
	switch {
	case n <= 75:
		return 1
	case n <= 255:
		return 2
	default:
		return 3
	}
}

func estSigScriptLen(m, redeemLen int) int {
	// m worst-case DER signatures with hash type byte, one push opcode
	// each, then the redeem script push.
	return m*74 + dataPushOverhead(redeemLen) + redeemLen
}

// EstimateSize returns the worst-case serialized size of tx once every
// input carries m signatures and the redeem script push.
func EstimateSize(tx *wire.MsgTx, m, redeemLen int) int {
	size := tx.SerializeSize()
	want := estSigScriptLen(m, redeemLen)
	for _, in := range tx.TxIn {
		cur := len(in.SignatureScript)
		size += wire.VarIntSerializeSize(uint64(want)) + want
		size -= wire.VarIntSerializeSize(uint64(cur)) + cur
	}
	return size
}

// EstimateFullSize returns the worst-case size of a spend of numIn shared
// inputs to numOut outputs before the transaction is built. Outputs are
// costed at P2PKH size, the larger of the standard output kinds.
func EstimateFullSize(numIn, numOut, m, n int) int {
	redeemLen := redeemScriptSize(n)
	sigLen := estSigScriptLen(m, redeemLen)
	size := 12 // version, lock time, expiry
	size += wire.VarIntSerializeSize(uint64(numIn)) * 2
	size += wire.VarIntSerializeSize(uint64(numOut))
	size += numIn * (41 + 16 + wire.VarIntSerializeSize(uint64(sigLen)) + sigLen)
	size += numOut * 36
	return size
}

func paymentScript(addr string, params *chaincfg.Params) (uint16, []byte, error) {
	a, err := stdaddr.DecodeAddress(addr, params)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid address %q: %v", addr, err)
	}
	vers, script := a.PaymentScript()
	return vers, script, nil
}

// SelectUTXOs picks inputs covering targetAtoms plus the estimated fee,
// largest first so the input count stays small.
func SelectUTXOs(utxos []UTXO, targetAtoms int64, numRecipients, m, n int, relayFeePerKb dcrutil.Amount) ([]UTXO, error) {
	if relayFeePerKb == 0 {
		relayFeePerKb = txrules.DefaultRelayFeePerKb
	}
	sorted := make([]UTXO, len(utxos))
	copy(sorted, utxos)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Atoms > sorted[j].Atoms })

	var sum int64
	for k, u := range sorted {
		if k == MaxInputs {
			return nil, fmt.Errorf("spend would require more than %d inputs", MaxInputs)
		}
		sum += u.Atoms
		fee := txrules.FeeForSerializeSize(relayFeePerKb, EstimateFullSize(k+1, numRecipients+1, m, n))
		if sum >= targetAtoms+int64(fee) {
			return sorted[:k+1], nil
		}
	}
	return nil, fmt.Errorf("insufficient funds: have %v, need %v plus fees",
		dcrutil.Amount(sum), dcrutil.Amount(targetAtoms))
}

// BuildSpend assembles the unsigned shared-wallet spend. Change returns to
// ChangeAddress (the shared address by convention); sub-dust change is
// folded into the fee. Input value fields are filled from the UTXO set so
// the transaction is self-describing for review.
func BuildSpend(p BuildSpendParams) (*wire.MsgTx, int64, int64, error) {
	m, _, err := ScriptDetails(p.RedeemScript)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(p.UTXOs) == 0 {
		return nil, 0, 0, fmt.Errorf("no inputs selected")
	}
	if len(p.UTXOs) > MaxInputs {
		return nil, 0, 0, fmt.Errorf("more than %d inputs", MaxInputs)
	}
	if len(p.Recipients) == 0 {
		return nil, 0, 0, fmt.Errorf("no recipients")
	}
	relayFee := p.RelayFeePerKb
	if relayFee == 0 {
		relayFee = txrules.DefaultRelayFeePerKb
	}

	tx := wire.NewMsgTx()
	var totalIn int64
	for _, u := range p.UTXOs {
		if u.Atoms <= 0 {
			return nil, 0, 0, fmt.Errorf("input %s:%d has no value", u.TxID, u.Vout)
		}
		h, err := chainhash.NewHashFromStr(u.TxID)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("invalid input txid %q: %v", u.TxID, err)
		}
		op := wire.OutPoint{Hash: *h, Index: u.Vout, Tree: u.Tree}
		tx.AddTxIn(wire.NewTxIn(&op, u.Atoms, nil))
		totalIn += u.Atoms
	}

	var totalOut int64
	for _, r := range p.Recipients {
		if r.Atoms <= 0 {
			return nil, 0, 0, fmt.Errorf("recipient amount must be positive")
		}
		vers, script, err := paymentScript(r.Address, p.ChainParams)
		if err != nil {
			return nil, 0, 0, err
		}
		tx.AddTxOut(&wire.TxOut{Value: r.Atoms, Version: vers, PkScript: script})
		totalOut += r.Atoms
	}

	changeVers, changeScript, err := paymentScript(p.ChangeAddress, p.ChainParams)
	if err != nil {
		return nil, 0, 0, err
	}

	sizeNoChange := EstimateSize(tx, m, len(p.RedeemScript))
	changeOutSize := 8 + 2 + wire.VarIntSerializeSize(uint64(len(changeScript))) + len(changeScript)
	feeWithChange := int64(txrules.FeeForSerializeSize(relayFee, sizeNoChange+changeOutSize))

	change := totalIn - totalOut - feeWithChange
	if change > 0 {
		out := &wire.TxOut{Value: change, Version: changeVers, PkScript: changeScript}
		if !txrules.IsDustOutput(out, relayFee) {
			tx.AddTxOut(out)
			return tx, feeWithChange, change, nil
		}
	}

	feeNoChange := int64(txrules.FeeForSerializeSize(relayFee, sizeNoChange))
	if totalIn-totalOut-feeNoChange < 0 {
		return nil, 0, 0, fmt.Errorf("insufficient funds: inputs %v, outputs %v plus fee %v",
			dcrutil.Amount(totalIn), dcrutil.Amount(totalOut), dcrutil.Amount(feeNoChange))
	}
	// The leftover is below the dust threshold or below the with-change
	// fee floor; it rides as extra fee.
	return tx, totalIn - totalOut, 0, nil
}

// DecodeTxHex strictly deserializes a fully serialized transaction.
func DecodeTxHex(txHex string) (*wire.MsgTx, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(txHex))
	if err != nil {
		return nil, fmt.Errorf("invalid transaction hex: %v", err)
	}
	r := bytes.NewReader(raw)
	var tx wire.MsgTx
	if err := tx.Deserialize(r); err != nil {
		return nil, fmt.Errorf("invalid transaction: %v", err)
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("trailing bytes after transaction")
	}
	return &tx, nil
}

// sigPushes tokenizes a P2SH multisig signature script and returns the
// signature pushes preceding the redeem script push. The final push must
// byte-equal the expected redeem script. An empty script means no
// signatures yet and yields no pushes.
func sigPushes(sigScript, redeemScript []byte) ([][]byte, error) {
	if len(sigScript) == 0 {
		return nil, nil
	}
	tok := txscript.MakeScriptTokenizer(0, sigScript)
	var pushes [][]byte
	for tok.Next() {
		// Signature scripts may only push data; OP_0 pushes the empty
		// placeholder the signer leaves for a missing signature.
		if tok.Opcode() > txscript.OP_PUSHDATA4 {
			return nil, fmt.Errorf("non-push opcode in signature script")
		}
		pushes = append(pushes, tok.Data())
	}
	if err := tok.Err(); err != nil {
		return nil, err
	}
	if len(pushes) == 0 {
		return nil, fmt.Errorf("missing redeem script push")
	}
	if !bytes.Equal(pushes[len(pushes)-1], redeemScript) {
		return nil, fmt.Errorf("signature script does not end with the expected redeem script")
	}
	return pushes[:len(pushes)-1], nil
}

// CountSigs returns the number of signatures present on a P2SH multisig
// signature script, ignoring OP_0 placeholders. This counter is the only
// authority for co-signing progress: dcrwallet's signrawtransaction
// reports complete=true even for partially signed multisig inputs.
func CountSigs(sigScript, redeemScript []byte) (int, error) {
	pushes, err := sigPushes(sigScript, redeemScript)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, d := range pushes {
		if len(d) > 0 {
			count++
		}
	}
	return count, nil
}

// AttributeSigs verifies every signature on input idx against the member
// public keys and returns the members that signed, keyed by lowercase hex
// pubkey. Any signature that fails to verify is an error: a relayed
// transaction must never carry junk that would only surface as a script
// failure at broadcast time.
func AttributeSigs(tx *wire.MsgTx, idx int, redeemScript []byte, memberKeys [][]byte) (map[string]bool, error) {
	if idx < 0 || idx >= len(tx.TxIn) {
		return nil, fmt.Errorf("input index %d out of range", idx)
	}
	pushes, err := sigPushes(tx.TxIn[idx].SignatureScript, redeemScript)
	if err != nil {
		return nil, err
	}
	signed := make(map[string]bool)
	for _, push := range pushes {
		if len(push) == 0 {
			continue
		}
		if len(push) < 9 {
			return nil, fmt.Errorf("input %d carries a malformed signature", idx)
		}
		hashType := txscript.SigHashType(push[len(push)-1])
		sig, err := ecdsa.ParseDERSignature(push[:len(push)-1])
		if err != nil {
			return nil, fmt.Errorf("input %d carries an unparseable signature: %v", idx, err)
		}
		hash, err := txscript.CalcSignatureHash(redeemScript, hashType, tx, idx, nil)
		if err != nil {
			return nil, err
		}
		matched := false
		for _, pk := range memberKeys {
			pub, err := secp256k1.ParsePubKey(pk)
			if err != nil {
				continue
			}
			if sig.Verify(hash, pub) {
				signed[hex.EncodeToString(pk)] = true
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("input %d carries a signature from a non-member key", idx)
		}
	}
	return signed, nil
}

// VerifyProposalUpdate checks a transaction returned by a cosigner against
// the proposal's txid and returns the members whose signatures are present
// on every input. The txid covers the whole transaction prefix, so the
// single hash comparison proves inputs, outputs, lock time and expiry are
// untouched; only signature scripts can legally differ.
func VerifyProposalUpdate(updated *wire.MsgTx, expectedTxID string, redeemScript []byte, memberKeys [][]byte) (map[string]bool, error) {
	if updated.TxHash().String() != expectedTxID {
		return nil, fmt.Errorf("transaction prefix was altered: txid changed")
	}
	if len(updated.TxIn) == 0 {
		return nil, fmt.Errorf("transaction has no inputs")
	}
	var common map[string]bool
	for idx := range updated.TxIn {
		signed, err := AttributeSigs(updated, idx, redeemScript, memberKeys)
		if err != nil {
			return nil, err
		}
		if common == nil {
			common = signed
			continue
		}
		for k := range common {
			if !signed[k] {
				delete(common, k)
			}
		}
	}
	return common, nil
}
