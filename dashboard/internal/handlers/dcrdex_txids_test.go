// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"io"
	"testing"
	"time"

	"dcrpulse/internal/rpc"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"google.golang.org/grpc"
)

// fakeWalletService implements the three WalletService methods the dex txid
// scan uses and records every GetTransactions start height, so the tests can
// assert the scan is incremental rather than whole-history.
type fakeWalletService struct {
	pb.WalletServiceClient // panic on anything unexpected

	tip    uint32
	mined  map[int32][]*pb.TransactionDetails // by block height
	starts []int32
}

func (f *fakeWalletService) Accounts(ctx context.Context, _ *pb.AccountsRequest, _ ...grpc.CallOption) (*pb.AccountsResponse, error) {
	return &pb.AccountsResponse{Accounts: []*pb.AccountsResponse_Account{
		{AccountName: "default", AccountNumber: 0},
		{AccountName: dexAccountName, AccountNumber: 4},
	}}, nil
}

func (f *fakeWalletService) BestBlock(ctx context.Context, _ *pb.BestBlockRequest, _ ...grpc.CallOption) (*pb.BestBlockResponse, error) {
	return &pb.BestBlockResponse{Height: f.tip}, nil
}

type fakeTxStream struct {
	grpc.ServerStreamingClient[pb.GetTransactionsResponse]
	resps []*pb.GetTransactionsResponse
	i     int
}

func (s *fakeTxStream) Recv() (*pb.GetTransactionsResponse, error) {
	if s.i >= len(s.resps) {
		return nil, io.EOF
	}
	r := s.resps[s.i]
	s.i++
	return r, nil
}

func (f *fakeWalletService) GetTransactions(ctx context.Context, req *pb.GetTransactionsRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.GetTransactionsResponse], error) {
	f.starts = append(f.starts, req.StartingBlockHeight)
	var resps []*pb.GetTransactionsResponse
	for h, txs := range f.mined {
		if h >= req.StartingBlockHeight {
			resps = append(resps, &pb.GetTransactionsResponse{
				MinedTransactions: &pb.BlockDetails{Height: h, Transactions: txs},
			})
		}
	}
	return &fakeTxStream{resps: resps}, nil
}

// dexTx builds a mined transaction crediting the dex account (4).
func dexTx(seed byte) *pb.TransactionDetails {
	h := make([]byte, 32)
	h[0] = seed
	return &pb.TransactionDetails{
		Hash:    h,
		Credits: []*pb.TransactionDetails_Output{{Account: 4, Amount: 1000}},
	}
}

func resetDexTxIDCache(t *testing.T) {
	t.Helper()
	prevClient := rpc.WalletGrpcClient
	clear := func() {
		dexTxIDMu.Lock()
		dexTxIDSet, dexUnminedTxs, dexTxIDAt = nil, nil, time.Time{}
		dexTxIDHeight, dexTxIDClient = 0, nil
		dexTxIDMu.Unlock()
	}
	clear()
	t.Cleanup(func() {
		clear()
		rpc.WalletGrpcClient = prevClient
	})
}

// The whole point of the incremental cursor: a warm rescan must start near the
// last scanned tip, not at zero, and must keep what it already found.
func TestDexTxIDScanIsIncremental(t *testing.T) {
	resetDexTxIDCache(t)
	fake := &fakeWalletService{
		tip:   100,
		mined: map[int32][]*pb.TransactionDetails{10: {dexTx(1)}},
	}
	rpc.WalletGrpcClient = fake

	set, _, err := dexAccountTxIDs(context.Background(), true)
	if err != nil {
		t.Fatalf("cold scan: %v", err)
	}
	if len(set) != 1 {
		t.Fatalf("cold scan found %d txids, want 1", len(set))
	}
	if len(fake.starts) != 1 || fake.starts[0] != 0 {
		t.Fatalf("cold scan started at %v, want [0]", fake.starts)
	}

	// New block with a new dex tx; the wallet moved to 105.
	fake.tip = 105
	fake.mined[101] = []*pb.TransactionDetails{dexTx(2)}

	set, _, err = dexAccountTxIDs(context.Background(), true)
	if err != nil {
		t.Fatalf("warm scan: %v", err)
	}
	if len(fake.starts) != 2 {
		t.Fatalf("warm scan issued %d requests, want 2 total", len(fake.starts))
	}
	if fake.starts[1] == 0 {
		t.Fatal("warm scan started at 0: the whole wallet was rescanned")
	}
	if want := int32(100 - dexTxIDReorgDepth); fake.starts[1] != want {
		t.Fatalf("warm scan started at %d, want %d (tip minus the reorg overlap)", fake.starts[1], want)
	}
	if len(set) != 2 {
		t.Fatalf("warm scan holds %d txids, want the merged 2", len(set))
	}
}

// A swapped wallet client (wallet switch or reconnect) must drop the cache, or
// the old wallet's txids scope the new wallet's history.
func TestDexTxIDCacheResetsOnClientSwap(t *testing.T) {
	resetDexTxIDCache(t)
	first := &fakeWalletService{tip: 50, mined: map[int32][]*pb.TransactionDetails{5: {dexTx(1)}}}
	rpc.WalletGrpcClient = first
	if _, _, err := dexAccountTxIDs(context.Background(), true); err != nil {
		t.Fatalf("first wallet: %v", err)
	}

	second := &fakeWalletService{tip: 60, mined: map[int32][]*pb.TransactionDetails{6: {dexTx(9)}}}
	rpc.WalletGrpcClient = second
	set, _, err := dexAccountTxIDs(context.Background(), true)
	if err != nil {
		t.Fatalf("second wallet: %v", err)
	}
	if len(second.starts) != 1 || second.starts[0] != 0 {
		t.Fatalf("swapped client scanned from %v, want a full scan [0]", second.starts)
	}
	if len(set) != 1 {
		t.Fatalf("swapped client set holds %d txids, want only the new wallet's 1", len(set))
	}
}

// Pagination reuse: within the TTL a non-fresh call must answer from cache.
func TestDexTxIDCacheServesPagination(t *testing.T) {
	resetDexTxIDCache(t)
	fake := &fakeWalletService{tip: 50, mined: map[int32][]*pb.TransactionDetails{5: {dexTx(1)}}}
	rpc.WalletGrpcClient = fake
	if _, _, err := dexAccountTxIDs(context.Background(), true); err != nil {
		t.Fatalf("cold: %v", err)
	}
	if _, _, err := dexAccountTxIDs(context.Background(), false); err != nil {
		t.Fatalf("cached: %v", err)
	}
	if len(fake.starts) != 1 {
		t.Fatalf("a cached call hit the wallet: %v", fake.starts)
	}
}
