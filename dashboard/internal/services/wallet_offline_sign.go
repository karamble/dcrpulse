// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// A hardware signer (Foundation Passport) cannot sign a raw Decred transaction:
// it needs, per input, the prevout script and the BIP32 derivation path so it can
// re-derive the key, recompute the p2pkh script, and refuse to sign if it does not
// match. Decred has no PSBT, so the device defines a minimal CBOR "SignRequest"
// that carries this metadata. As a watch-only (dpub) wallet, dcrpulse already knows
// each input's script, amount, and path, so it just assembles and encodes them.

type srInput struct {
	prevHash   []byte
	prevIndex  uint32
	tree       uint8
	sequence   uint32
	valueIn    int64
	branch     uint32
	index      uint32
	prevScript []byte
}

type srOutput struct {
	value    int64
	version  uint16
	pkScript []byte
	isChange bool
}

type signRequest struct {
	formatVersion uint8
	txVersion     uint16
	account       uint32
	lockTime      uint32
	expiry        uint32
	inputs        []srInput
	outputs       []srOutput
}

// cborWriter encodes the small, fixed subset of CBOR the SignRequest needs:
// definite-length arrays, unsigned/signed integers, and booleans. This matches
// what the device's minicbor derive emits and accepts: structs use minicbor's
// default array encoding (positional, no keys), and plain Vec<u8>/[u8;32] fields
// (no minicbor::bytes attribute) are arrays of integers, not byte strings. No
// general-purpose CBOR dependency is pulled in.
type cborWriter struct {
	buf []byte
}

func (w *cborWriter) head(major byte, n uint64) {
	switch {
	case n < 24:
		w.buf = append(w.buf, major<<5|byte(n))
	case n <= 0xFF:
		w.buf = append(w.buf, major<<5|24, byte(n))
	case n <= 0xFFFF:
		w.buf = append(w.buf, major<<5|25, byte(n>>8), byte(n))
	case n <= 0xFFFFFFFF:
		w.buf = append(w.buf, major<<5|26, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	default:
		w.buf = append(w.buf, major<<5|27,
			byte(n>>56), byte(n>>48), byte(n>>40), byte(n>>32),
			byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
}

func (w *cborWriter) uint(n uint64) { w.head(0, n) }

func (w *cborWriter) int(i int64) {
	if i >= 0 {
		w.head(0, uint64(i))
	} else {
		w.head(1, uint64(-1-i))
	}
}

func (w *cborWriter) arrayHead(n int) { w.head(4, uint64(n)) }

// byteArray encodes a byte slice as minicbor encodes a plain Vec<u8>/[u8;N]: a
// CBOR array of integers, one per byte (not a byte string).
func (w *cborWriter) byteArray(b []byte) {
	w.arrayHead(len(b))
	for _, x := range b {
		w.uint(uint64(x))
	}
}

func (w *cborWriter) boolean(b bool) {
	if b {
		w.buf = append(w.buf, 0xF5)
	} else {
		w.buf = append(w.buf, 0xF4)
	}
}

// encodeSignRequest serializes the SignRequest as CBOR exactly as the device's
// minicbor derive does: each struct is a definite-length array of its fields in
// declaration order (the minicbor default; the Rust structs carry no #[cbor(map)]),
// and byte fields are integer arrays.
func encodeSignRequest(sr *signRequest) []byte {
	w := &cborWriter{}
	w.arrayHead(7)
	w.uint(uint64(sr.formatVersion))
	w.uint(uint64(sr.txVersion))
	w.uint(uint64(sr.account))
	w.uint(uint64(sr.lockTime))
	w.uint(uint64(sr.expiry))
	w.arrayHead(len(sr.inputs))
	for _, in := range sr.inputs {
		w.arrayHead(8)
		w.byteArray(in.prevHash)
		w.uint(uint64(in.prevIndex))
		w.uint(uint64(in.tree))
		w.uint(uint64(in.sequence))
		w.int(in.valueIn)
		w.uint(uint64(in.branch))
		w.uint(uint64(in.index))
		w.byteArray(in.prevScript)
	}
	w.arrayHead(len(sr.outputs))
	for _, out := range sr.outputs {
		w.arrayHead(4)
		w.int(out.value)
		w.uint(uint64(out.version))
		w.byteArray(out.pkScript)
		w.boolean(out.isChange)
	}
	return w.buf
}

// prevoutInfo returns the prevout's pkScript, owning address, and amount for an
// input, read from the wallet's own record of the funding transaction.
func prevoutInfo(ctx context.Context, prevHash chainhash.Hash, prevIndex uint32) ([]byte, string, int64, error) {
	resp, err := rpc.WalletGrpcClient.GetTransaction(ctx, &pb.GetTransactionRequest{TransactionHash: prevHash[:]})
	if err != nil {
		return nil, "", 0, fmt.Errorf("get prevout tx %s: %w", prevHash, err)
	}
	if resp.Transaction == nil {
		return nil, "", 0, fmt.Errorf("prevout tx %s not found in wallet", prevHash)
	}
	for _, c := range resp.Transaction.Credits {
		if c.Index == prevIndex {
			return c.OutputScript, c.Address, c.Amount, nil
		}
	}
	return nil, "", 0, fmt.Errorf("prevout %s:%d is not owned by this wallet", prevHash, prevIndex)
}

// BuildSignRequest constructs an unsigned transaction and packages it as a CBOR
// SignRequest for an air-gapped hardware wallet to sign. It uses no private keys
// (works for watch-only wallets): dcrwallet selects coins and derives change, and
// each input is enriched with its prevout script and derivation path.
func BuildSignRequest(ctx context.Context, sourceAccount uint32, outputs []types.TxRecipient, sendAll bool) (*types.SignRequestExport, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet gRPC client not initialized")
	}
	cResp, err := ConstructTransaction(ctx, sourceAccount, outputs, sendAll)
	if err != nil {
		return nil, err
	}
	var tx wire.MsgTx
	if err := tx.Deserialize(bytes.NewReader(cResp.UnsignedTransaction)); err != nil {
		return nil, fmt.Errorf("decode unsigned tx: %w", err)
	}

	sr := &signRequest{
		formatVersion: 1,
		txVersion:     tx.Version,
		account:       sourceAccount,
		lockTime:      tx.LockTime,
		expiry:        tx.Expiry,
	}

	var inputsTotal int64
	for i := range tx.TxIn {
		in := tx.TxIn[i]
		prevHash := in.PreviousOutPoint.Hash
		prevIndex := in.PreviousOutPoint.Index

		prevScript, address, amount, perr := prevoutInfo(ctx, prevHash, prevIndex)
		if perr != nil {
			return nil, perr
		}
		va, verr := ValidateAddress(ctx, address)
		if verr != nil {
			return nil, fmt.Errorf("validate input address %s: %w", address, verr)
		}
		branch := uint32(0)
		if va.IsInternal {
			branch = 1
		}
		sr.inputs = append(sr.inputs, srInput{
			prevHash:   prevHash[:],
			prevIndex:  prevIndex,
			tree:       uint8(in.PreviousOutPoint.Tree),
			sequence:   in.Sequence,
			valueIn:    amount,
			branch:     branch,
			index:      va.Index,
			prevScript: prevScript,
		})
		inputsTotal += amount
	}

	decoded, err := DecodeRawTransaction(ctx, cResp.UnsignedTransaction)
	if err != nil {
		return nil, fmt.Errorf("decode outputs: %w", err)
	}

	var outputsTotal, change int64
	for i := range tx.TxOut {
		out := tx.TxOut[i]
		outputsTotal += out.Value
		// is_change must be true ONLY when the output pays one of our own derived
		// addresses: the device hides change from its review screen, so an external
		// recipient has to be is_change=false to be shown for verification. Do NOT
		// use ConstructTransaction's ChangeIndex - in send-all it points at the
		// external recipient. Fail safe: when ownership is uncertain (no address or a
		// lookup error) treat it as not-change, so the output is shown, never hidden.
		isMine := false
		if i < len(decoded.Outputs) && len(decoded.Outputs[i].Addresses) > 0 {
			if va, verr := ValidateAddress(ctx, decoded.Outputs[i].Addresses[0]); verr == nil {
				isMine = va.IsMine
			}
		}
		if isMine {
			change += out.Value
		}
		sr.outputs = append(sr.outputs, srOutput{
			value:    out.Value,
			version:  out.Version,
			pkScript: out.PkScript,
			isChange: isMine,
		})
	}

	cbor := encodeSignRequest(sr)
	return &types.SignRequestExport{
		SignRequestB64:      base64.StdEncoding.EncodeToString(cbor),
		SignRequestUR:       EncodeUR("dcr-sign-request", cbor),
		InputsTotalAtoms:    inputsTotal,
		OutputsTotalAtoms:   outputsTotal,
		ChangeAtoms:         change,
		FeeAtoms:            inputsTotal - outputsTotal,
		TotalDebitedAtoms:   inputsTotal - change,
		EstimatedSignedSize: cResp.EstimatedSignedSize,
	}, nil
}
