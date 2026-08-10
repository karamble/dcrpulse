// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"io"
	"testing"

	"dcrpulse/internal/rpc"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
	"google.golang.org/grpc"
)

// listTestTx serializes a transaction with numIn inputs (the first carrying
// valueIn) and the given output amounts, returning the raw bytes and a hash.
func listTestTx(t *testing.T, seed byte, numIn int, valueIn int64, outAtoms []int64) ([]byte, []byte) {
	t.Helper()
	mtx := wire.NewMsgTx()
	for i := 0; i < numIn; i++ {
		vi := int64(0)
		if i == 0 {
			vi = valueIn
		}
		mtx.AddTxIn(wire.NewTxIn(&wire.OutPoint{}, vi, nil))
	}
	for _, v := range outAtoms {
		mtx.AddTxOut(wire.NewTxOut(v, []byte{0x76}))
	}
	var buf bytes.Buffer
	if err := mtx.Serialize(&buf); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	hash := make([]byte, 32)
	hash[0] = seed
	return buf.Bytes(), hash
}

func txidOf(t *testing.T, hash []byte) string {
	t.Helper()
	h, err := chainhash.NewHash(hash)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return h.String()
}

func TestDeriveListTxFacts(t *testing.T) {
	mixRaw, mixHash := listTestTx(t, 1, 3, 0, []int64{5e8, 5e8, 5e8, 1234})
	txid, f, ok := deriveListTxFacts(&pb.TransactionDetails{
		Hash:        mixHash,
		Transaction: mixRaw,
		Credits:     []*pb.TransactionDetails_Output{{Amount: 5e8}},
		Debits:      []*pb.TransactionDetails_Input{{PreviousAmount: 6e8}},
	})
	if !ok || txid != txidOf(t, mixHash) {
		t.Fatalf("mix tx not derived")
	}
	if !f.mixed {
		t.Fatalf("three equal outputs with three inputs must read as CoinJoin")
	}
	if f.netDCR != -1.0 {
		t.Fatalf("net = %v, want -1.0 (credits minus debits)", f.netDCR)
	}
	if f.hasReward {
		t.Fatalf("regular tx must not carry a vote reward")
	}

	voteRaw, voteHash := listTestTx(t, 2, 2, 123456789, []int64{123400000})
	_, f, ok = deriveListTxFacts(&pb.TransactionDetails{
		Hash:            voteHash,
		Transaction:     voteRaw,
		TransactionType: pb.TransactionDetails_VOTE,
	})
	if !ok {
		t.Fatalf("vote tx not derived")
	}
	if !f.hasReward || f.voteReward != 1.23456789 {
		t.Fatalf("vote reward = %v/%v, want 1.23456789 from the stakebase input", f.voteReward, f.hasReward)
	}
	if f.mixed {
		t.Fatalf("two-input vote must not read as CoinJoin")
	}

	plainRaw, plainHash := listTestTx(t, 3, 1, 0, []int64{1e8, 2e8})
	_, f, ok = deriveListTxFacts(&pb.TransactionDetails{Hash: plainHash, Transaction: plainRaw})
	if !ok || f.mixed || f.hasReward {
		t.Fatalf("plain send misclassified: %+v ok=%v", f, ok)
	}

	if _, _, ok := deriveListTxFacts(&pb.TransactionDetails{Hash: plainHash, Transaction: []byte{0x01}}); ok {
		t.Fatalf("undecodable bytes must be skipped")
	}
}

type fakeListWallet struct {
	pb.WalletServiceClient // panic on anything unexpected

	reqs    []*pb.GetTransactionsRequest
	mined   []*pb.TransactionDetails
	unmined []*pb.TransactionDetails
}

type fakeListTxStream struct {
	grpc.ServerStreamingClient[pb.GetTransactionsResponse]
	resps []*pb.GetTransactionsResponse
	i     int
}

func (s *fakeListTxStream) Recv() (*pb.GetTransactionsResponse, error) {
	if s.i >= len(s.resps) {
		return nil, io.EOF
	}
	r := s.resps[s.i]
	s.i++
	return r, nil
}

func (f *fakeListWallet) GetTransactions(ctx context.Context, req *pb.GetTransactionsRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.GetTransactionsResponse], error) {
	f.reqs = append(f.reqs, req)
	return &fakeListTxStream{resps: []*pb.GetTransactionsResponse{
		{MinedTransactions: &pb.BlockDetails{Height: 100, Transactions: f.mined}},
		{UnminedTransactions: f.unmined},
	}}, nil
}

func TestListTxFactsForStreamsOnce(t *testing.T) {
	minedRaw, minedHash := listTestTx(t, 4, 3, 0, []int64{7e8, 7e8, 7e8})
	unminedRaw, unminedHash := listTestTx(t, 5, 1, 0, []int64{1e8})
	fake := &fakeListWallet{
		mined:   []*pb.TransactionDetails{{Hash: minedHash, Transaction: minedRaw}},
		unmined: []*pb.TransactionDetails{{Hash: unminedHash, Transaction: unminedRaw}},
	}
	prev := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = fake
	t.Cleanup(func() { rpc.WalletGrpcClient = prev })

	facts, err := listTxFactsFor(context.Background(), 42)
	if err != nil {
		t.Fatalf("listTxFactsFor: %v", err)
	}
	if len(fake.reqs) != 1 {
		t.Fatalf("GetTransactions called %d times, want 1", len(fake.reqs))
	}
	if fake.reqs[0].StartingBlockHeight != 42 || fake.reqs[0].EndingBlockHeight != -1 {
		t.Fatalf("range = [%d, %d], want [42, -1]",
			fake.reqs[0].StartingBlockHeight, fake.reqs[0].EndingBlockHeight)
	}
	if !facts[txidOf(t, minedHash)].mixed {
		t.Fatalf("mined CoinJoin-shaped tx missing from facts")
	}
	if _, ok := facts[txidOf(t, unminedHash)]; !ok {
		t.Fatalf("unmined tx missing from facts")
	}

	rpc.WalletGrpcClient = nil
	if _, err := listTxFactsFor(context.Background(), 0); err == nil {
		t.Fatalf("nil client must error so the listing degrades")
	}
}

func TestListTxMinHeight(t *testing.T) {
	e := func(confs int64) listTxEntry { return listTxEntry{Confirmations: confs} }
	tests := []struct {
		name    string
		entries []listTxEntry
		tip     int64
		want    int32
	}{
		{"no tip", []listTxEntry{e(5)}, 0, 0},
		{"single mined", []listTxEntry{e(1)}, 1000, 1000},
		{"oldest wins", []listTxEntry{e(10), e(3)}, 1000, 991},
		{"unmined ignored", []listTxEntry{e(0), e(2)}, 1000, 999},
		{"all unmined", []listTxEntry{e(0), e(0)}, 1000, 0},
		{"deeper than tip ignored", []listTxEntry{e(2000), e(4)}, 1000, 997},
	}
	for _, tc := range tests {
		if got := listTxMinHeight(tc.entries, tc.tip); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
