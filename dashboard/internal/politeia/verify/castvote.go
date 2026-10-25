// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package verify

import (
	"encoding/hex"
	"errors"
	"fmt"

	"decred.org/dcrwallet/v5/wallet"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
)

// CastVoteVerify checks a ballot the way politeiawww's client does: the
// Decred signed message of Token+Ticket+VoteBit recovers to the ballot's
// address, and the server's receipt covers that signature. Whether the
// address is the ticket's commitment address takes the chain to answer.
func CastVoteVerify(v CastVote, serverKey string, params stdaddr.AddressParams) error {
	addr, err := stdaddr.DecodeAddress(v.Address, params)
	if err != nil {
		return fmt.Errorf("ballot address: %w", err)
	}
	sig, err := hex.DecodeString(v.Signature)
	if err != nil {
		return errors.New("ballot signature is not hex")
	}
	ok, err := wallet.VerifyMessage(v.Token+v.Ticket+v.VoteBit, addr, sig, params)
	if err != nil || !ok {
		return errors.New("ballot signature does not match its address")
	}
	if err := VerifySignature(v.Receipt, serverKey, v.Signature); err != nil {
		return fmt.Errorf("ballot receipt: %w", err)
	}
	return nil
}
