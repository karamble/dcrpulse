// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// maxSignedTxBytes caps the size of a parsed transaction. A standard Decred
// transaction is far smaller; this is a defense-in-depth sanity bound.
const maxSignedTxBytes = 100_000

// sectionHeaderRe matches a hardware-wallet export section line such as
// "=== signed tx hex ===".
var sectionHeaderRe = regexp.MustCompile(`(?i)^\s*===.*===\s*$`)

// ParseSignedTransaction extracts and validates a signed Decred transaction from
// hardware-wallet input. The Foundation Passport ".dcrtx" file is exactly the raw
// serialized transaction bytes, so that is tried first; failing that, the input is
// interpreted as text: a bare hex string, or an export with "=== ... ===" sections
// (a hex body under the "signed tx hex" section). It returns the serialized bytes
// and the deserialized transaction so callers can reject malformed input before
// contacting a daemon.
func ParseSignedTransaction(data []byte) ([]byte, *wire.MsgTx, error) {
	// Binary: the input is the raw serialized transaction.
	if txBytes, tx, ok := deserializeTx(data); ok {
		return txBytes, tx, nil
	}

	// Text: a "=== ... ===" line starts a new section. Bodies under a header
	// containing "signed tx hex" are tried first; any other block (including the
	// whole input when there are no headers, i.e. a bare hex paste) is tried after.
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	var preferred, candidates []string
	var cur strings.Builder
	headerSigned := false
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		if headerSigned {
			preferred = append(preferred, cur.String())
		} else {
			candidates = append(candidates, cur.String())
		}
		cur.Reset()
	}
	for _, line := range strings.Split(normalized, "\n") {
		if sectionHeaderRe.MatchString(line) {
			flush()
			headerSigned = strings.Contains(strings.ToLower(line), "signed tx hex")
			continue
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	flush()

	for _, block := range append(preferred, candidates...) {
		if txBytes, tx, ok := decodeHexTxCandidate(block); ok {
			return txBytes, tx, nil
		}
	}
	return nil, nil, fmt.Errorf("no valid Decred transaction found in input")
}

// deserializeTx validates raw serialized transaction bytes: within the size cap,
// deserializing cleanly with no trailing data, and having at least one input and
// one output.
func deserializeTx(b []byte) ([]byte, *wire.MsgTx, bool) {
	if len(b) == 0 || len(b) > maxSignedTxBytes {
		return nil, nil, false
	}
	rdr := bytes.NewReader(b)
	var tx wire.MsgTx
	if err := tx.Deserialize(rdr); err != nil || rdr.Len() != 0 {
		return nil, nil, false
	}
	if len(tx.TxIn) == 0 || len(tx.TxOut) == 0 {
		return nil, nil, false
	}
	return b, &tx, true
}

// decodeHexTxCandidate reads one text block as hex-encoded transaction bytes,
// stripping whitespace and an optional 0x prefix before decoding.
func decodeHexTxCandidate(block string) ([]byte, *wire.MsgTx, bool) {
	s := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, block)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	if len(s) == 0 || len(s)%2 != 0 {
		return nil, nil, false
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, nil, false
	}
	return deserializeTx(b)
}

// PreviewSignedTransaction parses and decodes a signed transaction into a summary
// the user can verify before broadcasting. Fee is derived from the committed input
// amounts; if any input lacks one, FeeKnown is false and the fee is omitted rather
// than reported as a bogus value.
func PreviewSignedTransaction(ctx context.Context, data []byte) (*types.SignedTxPreview, error) {
	txBytes, tx, err := ParseSignedTransaction(data)
	if err != nil {
		return nil, err
	}
	decoded, err := DecodeRawTransaction(ctx, txBytes)
	if err != nil {
		return nil, err
	}

	var inputsTotal, outputsTotal int64
	feeKnown := true
	for _, in := range decoded.Inputs {
		if in.AmountIn <= 0 {
			feeKnown = false
		}
		inputsTotal += in.AmountIn
	}

	outputs := make([]types.SignedTxPreviewOutput, 0, len(decoded.Outputs))
	for _, out := range decoded.Outputs {
		outputsTotal += out.Value
		addr := ""
		if len(out.Addresses) > 0 {
			addr = out.Addresses[0]
		}
		mine := false
		if addr != "" {
			if v, verr := ValidateAddress(ctx, addr); verr == nil {
				mine = v.IsMine
			}
		}
		outputs = append(outputs, types.SignedTxPreviewOutput{
			Index:       out.Index,
			Address:     addr,
			AmountAtoms: out.Value,
			ScriptClass: out.ScriptClass.String(),
			IsMine:      mine,
		})
	}

	var fee int64
	if feeKnown {
		fee = inputsTotal - outputsTotal
		if fee < 0 {
			feeKnown = false
			fee = 0
		}
	}

	return &types.SignedTxPreview{
		Txid:              tx.TxHash().String(),
		SizeBytes:         len(txBytes),
		InputsTotalAtoms:  inputsTotal,
		OutputsTotalAtoms: outputsTotal,
		FeeAtoms:          fee,
		FeeKnown:          feeKnown,
		Outputs:           outputs,
		TxHex:             hex.EncodeToString(txBytes),
	}, nil
}

// BroadcastSignedTransaction publishes an already-signed transaction through
// dcrwallet. PublishTransaction records the tx in the wallet's unmined set (so a
// watch-only wallet immediately tracks the spend and its change) and relays it via
// the wallet's network backend. It uses no private keys.
func BroadcastSignedTransaction(ctx context.Context, signedTxBytes []byte) (string, error) {
	if rpc.WalletGrpcClient == nil {
		return "", fmt.Errorf("wallet gRPC client not initialized")
	}
	resp, err := rpc.WalletGrpcClient.PublishTransaction(ctx, &pb.PublishTransactionRequest{
		SignedTransaction: signedTxBytes,
	})
	if err != nil {
		return "", err
	}
	if h, herr := chainhash.NewHash(resp.TransactionHash); herr == nil {
		return h.String(), nil
	}
	return hex.EncodeToString(resp.TransactionHash), nil
}
