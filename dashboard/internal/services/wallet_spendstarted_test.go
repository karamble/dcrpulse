// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"google.golang.org/grpc"

	"dcrpulse/internal/rpc"
)

// stubWallet answers only the calls SignAndPublishTransaction makes. Embedding
// the interface leaves the rest unimplemented, which is fine: reaching one would
// be a bug in the code under test.
type stubWallet struct {
	pb.WalletServiceClient
	signErr error
	pubErr  error

	// publishCtxErr records what the publish call saw, so a test can tell
	// whether the caller's cancellation reached it.
	publishCtxErr error
}

func (s *stubWallet) Accounts(context.Context, *pb.AccountsRequest, ...grpc.CallOption) (*pb.AccountsResponse, error) {
	return &pb.AccountsResponse{}, nil
}

func (s *stubWallet) UnlockAccount(context.Context, *pb.UnlockAccountRequest, ...grpc.CallOption) (*pb.UnlockAccountResponse, error) {
	return &pb.UnlockAccountResponse{}, nil
}

func (s *stubWallet) LockAccount(context.Context, *pb.LockAccountRequest, ...grpc.CallOption) (*pb.LockAccountResponse, error) {
	return &pb.LockAccountResponse{}, nil
}

func (s *stubWallet) SignTransaction(context.Context, *pb.SignTransactionRequest, ...grpc.CallOption) (*pb.SignTransactionResponse, error) {
	if s.signErr != nil {
		return nil, s.signErr
	}
	return &pb.SignTransactionResponse{Transaction: []byte{0x01}}, nil
}

func (s *stubWallet) PublishTransaction(ctx context.Context, _ *pb.PublishTransactionRequest, _ ...grpc.CallOption) (*pb.PublishTransactionResponse, error) {
	s.publishCtxErr = ctx.Err()
	if s.pubErr != nil {
		return nil, s.pubErr
	}
	return &pb.PublishTransactionResponse{TransactionHash: make([]byte, 32)}, nil
}

func withWallet(t *testing.T, c pb.WalletServiceClient) {
	t.Helper()
	prev := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = c
	t.Cleanup(func() { rpc.WalletGrpcClient = prev })
}

// TestSignAndPublishMarksOnlyCommittedFailures pins which failures release a
// spend reservation. Signing is still pre-spend, so its failure is refundable;
// once the transaction is handed over for publishing it may reach the network
// whatever the call returns, so that failure is not.
func TestSignAndPublishMarksOnlyCommittedFailures(t *testing.T) {
	tests := []struct {
		name            string
		signErr, pubErr error
		wantStarted     bool
	}{
		{name: "signing failed, nothing was spent", signErr: errors.New("bad passphrase")},
		{name: "publishing failed, the spend may have gone out", pubErr: errors.New("dcrd rejected"), wantStarted: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withWallet(t, &stubWallet{signErr: tc.signErr, pubErr: tc.pubErr})
			_, err := SignAndPublishTransaction(context.Background(), 0, []byte{0x01}, []byte("pw"))
			if err == nil {
				t.Fatal("want an error")
			}
			if got := errors.Is(err, ErrSpendStarted); got != tc.wantStarted {
				t.Fatalf("errors.Is(err, ErrSpendStarted) = %v, want %v (err: %v)", got, tc.wantStarted, err)
			}
		})
	}
}

// TestPublishSurvivesCallerCancellation is the property the detach exists for:
// an agent that cancels its tool call must not stop a transaction that is
// already on its way to the network.
func TestPublishSurvivesCallerCancellation(t *testing.T) {
	s := &stubWallet{}
	withWallet(t, s)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the caller has gone away before the publish

	if _, err := SignAndPublishTransaction(ctx, 0, []byte{0x01}, []byte("pw")); err != nil {
		t.Fatalf("SignAndPublishTransaction: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("the caller context should be cancelled in this test")
	}
	if s.publishCtxErr != nil {
		t.Fatalf("the publish inherited the caller's cancellation: %v", s.publishCtxErr)
	}
}

// purchaseTimeout is a hang guard, not a work budget. The mixed background
// worker passes 0 and stays unbounded because CSPP pairing waits on epoch
// boundaries, so a deadline there would abort a purchase still in progress;
// only the synchronous path carries this bound, and tightening it into
// something a real purchase could exceed would reintroduce that failure.
func TestPurchaseTimeoutIsAHangGuard(t *testing.T) {
	if purchaseTimeout < time.Hour {
		t.Fatalf("purchaseTimeout = %v; a bound a real mixed purchase could hit "+
			"would abort work rather than catch a hang", purchaseTimeout)
	}
}
