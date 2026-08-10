// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/wire"
)

// buildTestVote serializes a structurally valid SSGen voting on the given
// block with the given vote bits and vote version. tspendVotes, when non-nil,
// is appended as a DCP-0006 TV output of (32-byte hash + 1-byte vote) tuples.
func buildTestVote(t *testing.T, votedHash chainhash.Hash, votedHeight uint32, bits uint16, voteVer uint32, tspendVotes []byte) string {
	t.Helper()
	mtx := wire.NewMsgTx()
	mtx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0xffffffff, Tree: wire.TxTreeRegular},
		SignatureScript:  []byte{0x00, 0x00},
		ValueIn:          wire.NullValueIn,
		BlockHeight:      wire.NullBlockHeight,
		BlockIndex:       wire.NullBlockIndex,
	})
	mtx.AddTxIn(wire.NewTxIn(
		&wire.OutPoint{Hash: chainhash.Hash{0x01}, Index: 0, Tree: wire.TxTreeStake},
		0, []byte{0x51}))
	ref := append([]byte{0x6a, 0x24}, votedHash[:]...)
	ref = binary.LittleEndian.AppendUint32(ref, votedHeight)
	mtx.AddTxOut(wire.NewTxOut(0, ref))
	vb := []byte{0x6a, 0x06}
	vb = binary.LittleEndian.AppendUint16(vb, bits)
	vb = binary.LittleEndian.AppendUint32(vb, voteVer)
	mtx.AddTxOut(wire.NewTxOut(0, vb))
	pay := append([]byte{0xbb, 0x76, 0xa9, 0x14}, make([]byte, 20)...)
	pay = append(pay, 0x88, 0xac)
	mtx.AddTxOut(wire.NewTxOut(1e8, pay))
	if tspendVotes != nil {
		mtx.Version = wire.TxVersionTreasury
		tv := []byte{0x6a, byte(2 + len(tspendVotes)), 'T', 'V'}
		tv = append(tv, tspendVotes...)
		mtx.AddTxOut(wire.NewTxOut(0, tv))
	}
	var buf bytes.Buffer
	if err := mtx.Serialize(&buf); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return hex.EncodeToString(buf.Bytes())
}

func TestSSGenVoteInfo(t *testing.T) {
	params := chaincfg.MainNetParams()
	votedHash := chainhash.Hash{0xab, 0xcd}

	// Block valid + yes on both mainnet v10 agendas
	// (blake3pow yes = 0x0004, changesubsidysplitr2 yes = 0x0040).
	info := ssgenVoteInfo(buildTestVote(t, votedHash, 123456, 0x0045, 10, nil), params)
	if info == nil {
		t.Fatal("valid vote decoded as nil")
	}
	if info.VotedOnHash != votedHash.String() || info.VotedOnHeight != 123456 {
		t.Fatalf("voted block = %s/%d", info.VotedOnHash, info.VotedOnHeight)
	}
	if !info.BlockValid || info.VoteVersion != 10 || info.VoteBits != 0x0045 {
		t.Fatalf("valid/version/bits = %v/%d/%#x", info.BlockValid, info.VoteVersion, info.VoteBits)
	}
	choices := map[string]string{}
	for _, a := range info.Agendas {
		choices[a.AgendaID] = a.Choice
	}
	if choices["blake3pow"] != "yes" || choices["changesubsidysplitr2"] != "yes" {
		t.Fatalf("agenda choices = %v, want both yes", choices)
	}

	// Block invalid + blake3pow no + subsidy split abstain.
	info = ssgenVoteInfo(buildTestVote(t, votedHash, 1, 0x0002, 10, nil), params)
	if info == nil || info.BlockValid {
		t.Fatalf("bit 0 clear must decode as disapproval")
	}
	choices = map[string]string{}
	for _, a := range info.Agendas {
		choices[a.AgendaID] = a.Choice
	}
	if choices["blake3pow"] != "no" || choices["changesubsidysplitr2"] != "abstain" {
		t.Fatalf("agenda choices = %v, want no/abstain", choices)
	}

	// Bits matching no defined choice fall back to abstain (both mask bits set).
	info = ssgenVoteInfo(buildTestVote(t, votedHash, 1, 0x0007, 10, nil), params)
	if info == nil {
		t.Fatal("unmatched-bits vote decoded as nil")
	}
	choices = map[string]string{}
	for _, a := range info.Agendas {
		choices[a.AgendaID] = a.Choice
	}
	if got, ok := choices["blake3pow"]; !ok || got != "abstain" {
		t.Fatalf("unmatched bits decoded as %q (present=%v), want abstain row present", got, ok)
	}

	// Treasury votes: one yes, one no.
	tv := append(bytes.Repeat([]byte{0x22}, 32), 0x01)
	tv = append(tv, append(bytes.Repeat([]byte{0x33}, 32), 0x02)...)
	info = ssgenVoteInfo(buildTestVote(t, votedHash, 9, 0x0001, 10, tv), params)
	if info == nil {
		t.Fatal("treasury vote decoded as nil")
	}
	if len(info.TreasuryVotes) != 2 {
		t.Fatalf("treasury votes = %d, want 2", len(info.TreasuryVotes))
	}
	yesHash, _ := chainhash.NewHash(bytes.Repeat([]byte{0x22}, 32))
	if info.TreasuryVotes[0].TSpend != yesHash.String() || info.TreasuryVotes[0].Choice != "yes" {
		t.Fatalf("first treasury vote = %+v, want yes on %s", info.TreasuryVotes[0], yesHash)
	}
	if info.TreasuryVotes[1].Choice != "no" {
		t.Fatalf("second treasury vote = %+v, want no", info.TreasuryVotes[1])
	}

	// Nil params: agendas absent, the rest still decodes.
	info = ssgenVoteInfo(buildTestVote(t, votedHash, 123456, 0x0045, 10, nil), nil)
	if info == nil || len(info.Agendas) != 0 || info.VoteVersion != 10 {
		t.Fatalf("nil-params decode = %+v", info)
	}

	// Garbage stays nil.
	if ssgenVoteInfo("zz", params) != nil || ssgenVoteInfo("6a", params) != nil {
		t.Fatal("non-vote input must return nil")
	}
}
