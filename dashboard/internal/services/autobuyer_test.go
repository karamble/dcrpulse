// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"google.golang.org/grpc"
)

type fakeBuyerStream struct {
	grpc.ServerStreamingClient[pb.RunTicketBuyerResponse]
}

func (s *fakeBuyerStream) Recv() (*pb.RunTicketBuyerResponse, error) {
	return nil, io.EOF
}

type fakeTicketBuyer struct {
	pb.TicketBuyerServiceClient // panic on anything unexpected
}

func (f *fakeTicketBuyer) RunTicketBuyer(ctx context.Context, req *pb.RunTicketBuyerRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.RunTicketBuyerResponse], error) {
	return &fakeBuyerStream{}, nil
}

type fakeAutobuyerWallet struct {
	pb.WalletServiceClient // panic on anything unexpected

	locked []uint32
}

func (f *fakeAutobuyerWallet) GetTickets(ctx context.Context, req *pb.GetTicketsRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.GetTicketsResponse], error) {
	return nil, fmt.Errorf("no tickets in tests")
}

func (f *fakeAutobuyerWallet) LockAccount(ctx context.Context, req *pb.LockAccountRequest, _ ...grpc.CallOption) (*pb.LockAccountResponse, error) {
	f.locked = append(f.locked, req.AccountNumber)
	return &pb.LockAccountResponse{}, nil
}

// The ticket poller only returns when its context ends, so a stream that dies
// on its own (daemon restart, transport error) must still unwind the
// supervisor: relock the source account and clear the running flag.
func TestAutobuyerUnwindsOnStreamEOF(t *testing.T) {
	const sourceAccount = uint32(7)

	prevBuyer := rpc.TicketBuyerClient
	rpc.TicketBuyerClient = &fakeTicketBuyer{}
	t.Cleanup(func() { rpc.TicketBuyerClient = prevBuyer })

	wallet := &fakeAutobuyerWallet{}
	prevWallet := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = wallet
	t.Cleanup(func() { rpc.WalletGrpcClient = prevWallet })

	defer setAutobuyerRunning(true)()

	done := make(chan struct{})
	go func() {
		defer close(done)
		runAutobuyer(context.Background(), types.AutobuyerSettings{}, sourceAccount, TicketMixing{}, false, true, false)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runAutobuyer did not return after the stream ended")
	}
	if IsAutobuyerRunning() {
		t.Fatal("autobuyer still reports running after the stream ended")
	}
	relocked := false
	for _, a := range wallet.locked {
		if a == sourceAccount {
			relocked = true
		}
	}
	if !relocked {
		t.Fatalf("source account %d was not re-locked (locked: %v)", sourceAccount, wallet.locked)
	}
}

// Starting the autobuyer while the standalone mixer runs must be refused up
// front: both spend the mixed account, and the mixer's relock on stop would
// silently break the autobuyer's inline mixing.
func TestStartAutobuyerRefusesWhileMixerRunning(t *testing.T) {
	prevBuyer := rpc.TicketBuyerClient
	rpc.TicketBuyerClient = &fakeTicketBuyer{}
	t.Cleanup(func() { rpc.TicketBuyerClient = prevBuyer })

	prevWallet := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = &fakeAutobuyerWallet{}
	t.Cleanup(func() { rpc.WalletGrpcClient = prevWallet })

	defer setMixerRunning(true)()

	settings := &types.AutobuyerSettings{
		VspHost:   "vsp.example.org",
		VspPubkey: "pubkey",
	}
	err := StartAutobuyer(settings, []byte("passphrase"))
	if err == nil || !strings.Contains(err.Error(), "mixer") {
		t.Fatalf("StartAutobuyer = %v, want a mixer-running error", err)
	}
	if IsAutobuyerRunning() {
		t.Fatal("autobuyer must not report running after a refused start")
	}
}
