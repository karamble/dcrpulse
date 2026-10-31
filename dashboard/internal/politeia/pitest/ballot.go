// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package pitest

import (
	"bytes"
	"encoding/hex"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"

	"dcrpulse/internal/politeia/verify"
)

// Voter holds a ticket's commitment key on mainnet.
type Voter struct {
	Address string
	priv    *secp256k1.PrivateKey
}

// NewVoter returns a fresh commitment key.
func NewVoter() Voter {
	priv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		panic(err)
	}
	pkh := stdaddr.Hash160(priv.PubKey().SerializeCompressed())
	addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(pkh, chaincfg.MainNetParams())
	if err != nil {
		panic(err)
	}
	return Voter{Address: addr.String(), priv: priv}
}

// Ballot returns the voter's ballot for ticket, signed as a Decred message
// over Token+Ticket+VoteBit, with the server's receipt.
func (v Voter) Ballot(server Key, token, ticket, voteBit string) verify.CastVote {
	var buf bytes.Buffer
	_ = wire.WriteVarString(&buf, 0, "Decred Signed Message:\n")
	_ = wire.WriteVarString(&buf, 0, token+ticket+voteBit)
	sig := hex.EncodeToString(ecdsa.SignCompact(v.priv, chainhash.HashB(buf.Bytes()), true))
	return verify.CastVote{
		Token: token, Ticket: ticket, VoteBit: voteBit, Address: v.Address,
		Signature: sig, Receipt: server.Sign(sig), Timestamp: time.Now().AddDate(0, -1, 0).Unix(),
	}
}
