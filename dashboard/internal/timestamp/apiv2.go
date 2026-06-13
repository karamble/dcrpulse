// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package timestamp

import (
	"encoding/json"
	"regexp"
)

// dcrtime public v2 API endpoints. Note that timestamp.decred.org is the
// human-facing "Timestamply" web UI; the JSON API that it (and the dcrtime CLI)
// post to lives at time.decred.org:49152 for mainnet and
// time-testnet.decred.org:59152 for testnet. These mirror DefaultMainnetTimeHost
// /Port and DefaultTestnetTimeHost/Port in github.com/decred/dcrtime/api/v2.
const (
	mainnetAPIURL = "https://time.decred.org:49152"
	testnetAPIURL = "https://time-testnet.decred.org:59152"

	routeStatus         = "/v2/status"
	routeTimestampBatch = "/v2/timestamp/batch"
	routeVerifyBatch    = "/v2/verify/batch"
	routeVersion        = "/version"
)

// zeroHash is the all-zero transaction string dcrtime returns for a digest that
// has been recorded but not yet committed to an anchor transaction.
const zeroHash = "0000000000000000000000000000000000000000000000000000000000000000"

// regexpSHA256 matches the hex text form of a sha256 digest.
var regexpSHA256 = regexp.MustCompile("^[A-Fa-f0-9]{64}$")

// ValidDigest reports whether s is a 64-character hex sha256 digest.
func ValidDigest(s string) bool { return regexpSHA256.MatchString(s) }

// ResultT is dcrtime's per-digest result code.
type ResultT int

const (
	ResultInvalid          ResultT = 0
	ResultOK               ResultT = 1
	ResultExistsError      ResultT = 2
	ResultDoesntExistError ResultT = 3
	ResultDisabled         ResultT = 4
)

var resultText = map[ResultT]string{
	ResultInvalid:          "Invalid",
	ResultOK:               "OK",
	ResultExistsError:      "Exists",
	ResultDoesntExistError: "Doesn't exist",
	ResultDisabled:         "Query disallowed",
}

func (r ResultT) String() string {
	if s, ok := resultText[r]; ok {
		return s
	}
	return "Unknown"
}

// ChainState is the derived anchoring stage of a digest, mirroring the
// distinctions dcrtimegui draws (src/helpers/dcrtime.js).
type ChainState string

const (
	// StateNotFound: dcrtime has no record of the digest.
	StateNotFound ChainState = "notfound"
	// StateAwaiting: accepted, but not yet placed in an anchor transaction
	// (waiting for the next hourly flush). Transaction is the zero hash.
	StateAwaiting ChainState = "awaiting"
	// StatePending: placed in an anchor transaction that is not yet mined to
	// a confirmed block. Transaction is set, chaintimestamp is still 0.
	StatePending ChainState = "pending"
	// StateAnchored: committed to the chain (chaintimestamp is non-zero).
	StateAnchored ChainState = "anchored"
)

// --- request / response types (subset of github.com/decred/dcrtime/api/v2) ---

type timestampBatch struct {
	ID      string   `json:"id"`
	Digests []string `json:"digests"`
}

type timestampBatchReply struct {
	ID              string    `json:"id"`
	ServerTimestamp int64     `json:"servertimestamp"`
	Digests         []string  `json:"digests"`
	Results         []ResultT `json:"results"`
}

type verifyBatch struct {
	ID         string   `json:"id"`
	Digests    []string `json:"digests"`
	Timestamps []int64  `json:"timestamps"`
}

type verifyBatchReply struct {
	ID      string         `json:"id"`
	Digests []verifyDigest `json:"digests"`
}

type verifyDigest struct {
	Digest           string           `json:"digest"`
	ServerTimestamp  int64            `json:"servertimestamp"`
	FlushTimestamp   int64            `json:"flushtimestamp"`
	Result           ResultT          `json:"result"`
	ChainInformation chainInformation `json:"chaininformation"`
}

// chainInformation carries the anchor proof. MerklePath is kept as raw JSON so
// the proof is stored byte-for-byte; its shape matches merkle.Branch and is
// parsed on demand for verification (see verify.go).
type chainInformation struct {
	ChainTimestamp   int64           `json:"chaintimestamp"`
	Confirmations    *int32          `json:"confirmations,omitempty"`
	MinConfirmations int32           `json:"minconfirmations,omitempty"`
	Transaction      string          `json:"transaction"`
	MerkleRoot       string          `json:"merkleroot"`
	MerklePath       json.RawMessage `json:"merklepath"`
}

// state derives the anchoring stage of a verified digest.
func (vd verifyDigest) state() ChainState {
	if vd.Result != ResultOK {
		return StateNotFound
	}
	ci := vd.ChainInformation
	if ci.ChainTimestamp != 0 {
		return StateAnchored
	}
	if ci.Transaction != "" && ci.Transaction != zeroHash {
		return StatePending
	}
	return StateAwaiting
}
