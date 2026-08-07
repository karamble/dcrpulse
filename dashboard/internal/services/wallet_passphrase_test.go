// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"dcrpulse/internal/rpc"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"google.golang.org/grpc"
)

// The wallet-wide passphrase change runs first and cannot be rolled back, so an
// account whose per-account update fails keeps the old passphrase and cannot be
// spent from. Cancelling the remaining updates on the first failure therefore
// strands more accounts than necessary; Decrediton's Promise.all lets every call
// finish, and these pin that behaviour.

// Embedding the interface supplies the ~100 methods this does not care about;
// calling any of them panics, which is what we want if the code under test
// starts using one.
type fakeWalletClient struct {
	pb.WalletServiceClient

	accounts []uint32
	failOn   map[uint32]error

	mu     sync.Mutex
	called []uint32
}

func (f *fakeWalletClient) ChangePassphrase(ctx context.Context, in *pb.ChangePassphraseRequest, _ ...grpc.CallOption) (*pb.ChangePassphraseResponse, error) {
	return &pb.ChangePassphraseResponse{}, nil
}

func (f *fakeWalletClient) Accounts(ctx context.Context, in *pb.AccountsRequest, _ ...grpc.CallOption) (*pb.AccountsResponse, error) {
	out := &pb.AccountsResponse{}
	for _, n := range f.accounts {
		out.Accounts = append(out.Accounts, &pb.AccountsResponse_Account{AccountNumber: n})
	}
	return out, nil
}

func (f *fakeWalletClient) SetAccountPassphrase(ctx context.Context, in *pb.SetAccountPassphraseRequest, _ ...grpc.CallOption) (*pb.SetAccountPassphraseResponse, error) {
	if err, ok := f.failOn[in.GetAccountNumber()]; ok {
		return nil, err
	}
	// Honour cancellation, so a fan-out that aborts its siblings is detectable:
	// without this the test would pass against the errgroup version too.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(30 * time.Millisecond):
	}
	f.mu.Lock()
	f.called = append(f.called, in.GetAccountNumber())
	f.mu.Unlock()
	return &pb.SetAccountPassphraseResponse{}, nil
}

func withFakeWallet(t *testing.T, f *fakeWalletClient) {
	t.Helper()
	prev := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = f
	t.Cleanup(func() { rpc.WalletGrpcClient = prev })
}

func TestChangePassphraseAttemptsEveryAccount(t *testing.T) {
	f := &fakeWalletClient{
		accounts: []uint32{0, 1, 2, 3},
		failOn:   map[uint32]error{1: errors.New("wallet must be unlocked to set a unique account passphrase")},
	}
	withFakeWallet(t, f)

	err := ChangePrivatePassphrase(context.Background(), []byte("old"), []byte("new"))
	if err == nil {
		t.Fatal("expected an error when one account fails")
	}

	// The three that could succeed must all have been attempted; the failing one
	// must not abort them.
	if len(f.called) != 3 {
		t.Errorf("%d accounts updated, want 3 (accounts: %v)", len(f.called), f.called)
	}

	var partial *PartialPassphraseChangeError
	if !errors.As(err, &partial) {
		t.Fatalf("error is %T, want *PartialPassphraseChangeError", err)
	}
	if len(partial.Accounts) != 1 || partial.Accounts[0] != 1 {
		t.Errorf("reported accounts %v, want [1]", partial.Accounts)
	}
}

// Every failure has to be named, not just the first, or the user cannot tell
// which accounts need attention.
func TestChangePassphraseReportsEveryFailure(t *testing.T) {
	f := &fakeWalletClient{
		accounts: []uint32{0, 1, 2, 3},
		failOn: map[uint32]error{
			1: errors.New("boom"),
			3: errors.New("boom"),
		},
	}
	withFakeWallet(t, f)

	err := ChangePrivatePassphrase(context.Background(), []byte("old"), []byte("new"))
	var partial *PartialPassphraseChangeError
	if !errors.As(err, &partial) {
		t.Fatalf("error is %T, want *PartialPassphraseChangeError", err)
	}
	if len(partial.Accounts) != 2 || partial.Accounts[0] != 1 || partial.Accounts[1] != 3 {
		t.Errorf("reported accounts %v, want [1 3]", partial.Accounts)
	}
}

// Imported (2^31-1) and xpub-imported (>= 2^31) accounts are not per-account
// encrypted and must be skipped.
func TestChangePassphraseSkipsImportedAccounts(t *testing.T) {
	f := &fakeWalletClient{accounts: []uint32{0, 1, 2147483647, 2147483648}}
	withFakeWallet(t, f)

	if err := ChangePrivatePassphrase(context.Background(), []byte("old"), []byte("new")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.called) != 2 {
		t.Errorf("updated %v, want only accounts 0 and 1", f.called)
	}
}
