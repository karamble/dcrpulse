// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package timestamp

import "encoding/json"

// Proof is the self-contained, exportable proof for one anchored digest. A third
// party can verify it without this app (and, eventually, without the public
// dcrtime server) by hashing the file, walking the merkle path to the root, and
// confirming the root is committed in the referenced Decred transaction.
type Proof struct {
	Version  int         `json:"version"`
	Digest   string      `json:"digest"`
	Filename string      `json:"filename,omitempty"`
	Title    string      `json:"title,omitempty"`
	Anchor   ProofAnchor `json:"anchor"`
}

// ProofAnchor holds the on-chain anchor data. MerklePath is the verbatim dcrtime
// proof structure.
type ProofAnchor struct {
	MerkleRoot string          `json:"merkleRoot"`
	MerklePath json.RawMessage `json:"merklePath"`
	TxID       string          `json:"txId"`
	Timestamp  int64           `json:"timestamp"`
	Chain      string          `json:"chain"`
	Server     string          `json:"server"`
}

// NewProof builds an exportable proof from a record. chain and server describe
// the network the proof was anchored against (e.g. "decred-mainnet" and the
// dcrtime API host).
func NewProof(r *Record, chain, server string) *Proof {
	return &Proof{
		Version:  1,
		Digest:   r.Digest,
		Filename: r.Filename,
		Title:    r.Title,
		Anchor: ProofAnchor{
			MerkleRoot: r.MerkleRoot,
			MerklePath: r.MerklePath,
			TxID:       r.TxID,
			Timestamp:  r.AnchorTime,
			Chain:      chain,
			Server:     server,
		},
	}
}
