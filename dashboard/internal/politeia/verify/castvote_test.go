// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package verify_test

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"

	"dcrpulse/internal/politeia/pitest"
	"dcrpulse/internal/politeia/verify"
)

func TestCastVoteVerify(t *testing.T) {
	server := pitest.NewKey()
	voter := pitest.NewVoter()
	params := chaincfg.MainNetParams()
	const token, ticket = "0123456789abcdef", "aa11223344556677889900aabbccddeeff00112233445566778899aabbccddee"
	good := voter.Ballot(server, token, ticket, "2")
	if err := verify.CastVoteVerify(good, server.Pub, params); err != nil {
		t.Fatalf("good ballot: %v", err)
	}
	for name, mutate := range map[string]func(*verify.CastVote){
		"other vote bit":    func(b *verify.CastVote) { b.VoteBit = "1" },
		"other ticket":      func(b *verify.CastVote) { b.Ticket = "bb" + b.Ticket[2:] },
		"other token":       func(b *verify.CastVote) { b.Token = "fedcba9876543210" },
		"other address":     func(b *verify.CastVote) { b.Address = pitest.NewVoter().Address },
		"forged receipt":    func(b *verify.CastVote) { b.Receipt = pitest.NewKey().Sign(b.Signature) },
		"signature not hex": func(b *verify.CastVote) { b.Signature = "zz" },
		"testnet address":   func(b *verify.CastVote) { b.Address = "Tsf5Qvq2m7X5KzTZDdSGfa6WrMtikYVRkaL" },
	} {
		b := good
		mutate(&b)
		if err := verify.CastVoteVerify(b, server.Pub, params); err == nil {
			t.Errorf("%s: verified", name)
		}
	}
}
