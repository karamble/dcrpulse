// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package timestamp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"dcrpulse/internal/services"
)

// Validation is the staged result of checking a timestamp proof end to end
// against the Decred chain, entirely through dcrpulse's own dcrd connection
// (no external block explorer). Each boolean is one independently verifiable
// step the UI can show as it passes.
type Validation struct {
	Digest          string `json:"digest"`
	HasProof        bool   `json:"hasProof"`        // proof has merkle path + root + tx
	MerklePathValid bool   `json:"merklePathValid"` // the path resolves to a root
	DigestInTree    bool   `json:"digestInTree"`    // the file digest is a leaf in the path
	RootMatches     bool   `json:"rootMatches"`     // computed root == claimed root
	AnchoredOnChain bool   `json:"anchoredOnChain"` // root committed in the anchor tx
	TxID            string `json:"txId,omitempty"`
	BlockHeight     int64  `json:"blockHeight,omitempty"`
	BlockTime       int64  `json:"blockTime,omitempty"` // unix seconds, 0 until mined
	Confirmations   int64  `json:"confirmations,omitempty"`
	MerkleRoot      string `json:"merkleRoot,omitempty"`
	Note            string `json:"note,omitempty"` // human note when a step blocks/fails
}

// ValidateProof verifies a dcrtime proof for digest without trusting dcrtime:
// the merkle path must resolve to the claimed root, the digest must be a leaf in
// that path, and the root must be committed in the anchor transaction's OP_RETURN
// on the chain dcrd is following. It is safe to call with a partial proof and
// returns early with HasProof=false.
func ValidateProof(ctx context.Context, digest, merkleRoot string, merklePath json.RawMessage, txID string) *Validation {
	v := &Validation{
		Digest:     strings.ToLower(strings.TrimSpace(digest)),
		MerkleRoot: strings.ToLower(strings.TrimSpace(merkleRoot)),
		TxID:       txID,
	}
	if len(merklePath) == 0 || v.MerkleRoot == "" || txID == "" {
		v.Note = "Proof is incomplete - the digest is not anchored yet."
		return v
	}
	v.HasProof = true

	// 1. The merkle path resolves to a root.
	var br Branch
	if err := json.Unmarshal(merklePath, &br); err != nil {
		v.Note = "Invalid merkle path: " + err.Error()
		return v
	}
	root, err := VerifyAuthPath(&br)
	if err != nil {
		v.Note = "Merkle path verification failed: " + err.Error()
		return v
	}
	v.MerklePathValid = true

	// 2. The file digest is one of the matched leaf hashes in the path.
	if db, err := hex.DecodeString(v.Digest); err == nil && len(db) == sha256.Size {
		var d [sha256.Size]byte
		copy(d[:], db)
		for _, h := range br.Hashes {
			if h == d {
				v.DigestInTree = true
				break
			}
		}
	}

	// 3. The computed root equals the proof's claimed root.
	claimed, err := hex.DecodeString(v.MerkleRoot)
	if err != nil {
		v.Note = "Invalid merkle root: " + err.Error()
		return v
	}
	v.RootMatches = bytes.Equal(root[:], claimed)
	if !v.RootMatches {
		v.Note = "Computed merkle root does not match the proof."
		return v
	}

	// 4. The root is committed in the anchor tx on this chain (dcrd, no dcrdata).
	tx, err := services.FetchTransaction(ctx, txID)
	if err != nil {
		v.Note = "Anchor transaction not found on this network: " + err.Error()
		return v
	}
	v.BlockHeight = tx.BlockHeight
	v.Confirmations = tx.Confirmations
	if tx.BlockHeight > 0 {
		v.BlockTime = tx.Timestamp.Unix()
	}
	for _, out := range tx.Outputs {
		if !strings.EqualFold(out.ScriptPubKey.Type, "nulldata") {
			continue
		}
		if extractNullDataMerkleRoot(out.ScriptPubKey.Hex) == v.MerkleRoot {
			v.AnchoredOnChain = true
			break
		}
	}
	if !v.AnchoredOnChain {
		v.Note = "Merkle root not found in the anchor transaction's OP_RETURN output."
	}
	return v
}

// extractNullDataMerkleRoot returns the lowercase hex of the 32-byte payload of a
// dcrtime anchor OP_RETURN output, or "" when the script is not exactly
// OP_RETURN OP_DATA_32 <32 bytes>. Mirrors extractNullDataMerkleRootV0 in
// github.com/decred/dcrtime/util/dcrdata.go.
func extractNullDataMerkleRoot(scriptHex string) string {
	h := strings.ToLower(strings.TrimSpace(scriptHex))
	// 6a = OP_RETURN, 20 = OP_DATA_32 (push 32 bytes); (2 + 32) bytes = 68 hex chars.
	if len(h) != 68 || !strings.HasPrefix(h, "6a20") {
		return ""
	}
	return h[4:]
}
