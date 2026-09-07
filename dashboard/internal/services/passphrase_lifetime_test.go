// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

// A Go string cannot be zeroed, so anything holding a spending secret past the
// call that produced it takes a []byte and wipes it. These pin the wipe on the
// paths that outlive their caller: a restore's discovery goroutine, and the
// worker entry points whose own comments say they consume the slice.

// secret returns a slice the test owns, so a passing assertion cannot be
// reading some other buffer that happened to be zero already.
func secret() []byte { return []byte{1, 2, 3, 4, 5, 6, 7, 8} }

func requireWiped(t *testing.T, b []byte, what string) {
	t.Helper()
	if len(b) == 0 {
		t.Fatalf("%s: nothing to check, the fixture is empty", what)
	}
	if !bytes.Equal(b, make([]byte, len(b))) {
		t.Fatalf("%s was left readable in memory: %v", what, b)
	}
}

// The discovery goroutine outlives its request by a full chain scan, which is
// the whole reason this secret travels as a slice.
func TestDiscoveryZeroesThePassphrase(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stream *fakeSyncStream
	}{
		{"reaches SYNCED", &fakeSyncStream{resp: &pb.RpcSyncResponse{Synced: true}}},
		{"stream fails", &fakeSyncStream{err: fmt.Errorf("daemon went away")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withHooks(t, "alpha")
			withDiscoveryFakes(t, tc.stream)

			pass := secret()
			runDiscoveryRpcSync(pass)
			requireWiped(t, pass, "the discovery passphrase")
		})
	}
}

// The seed is the wallet. dcrwallet has its own copy once CreateWallet returns,
// so ours must not survive the call.
func TestCreateNewWalletZeroesTheSeed(t *testing.T) {
	withHooks(t, "alpha")
	l := &seedCapturingLoader{}
	prevLoader, prevWallet := rpc.WalletLoaderClient, rpc.WalletGrpcClient
	rpc.WalletLoaderClient = l
	rpc.WalletGrpcClient = &countingWallet{}
	t.Cleanup(func() { rpc.WalletLoaderClient, rpc.WalletGrpcClient = prevLoader, prevWallet })

	pass := secret()
	if err := CreateNewWallet(context.Background(), "public", pass, "0011223344556677", false); err != nil {
		t.Fatalf("CreateNewWallet() = %v, want nil", err)
	}
	if l.seed == nil {
		t.Fatal("CreateWallet was never called, so nothing was proven")
	}
	// The request still points at the same backing array the service decoded.
	requireWiped(t, l.seed, "the seed")
}

type seedCapturingLoader struct {
	pb.WalletLoaderServiceClient // panic on anything unexpected
	seed                         []byte
}

func (s *seedCapturingLoader) CreateWallet(_ context.Context, in *pb.CreateWalletRequest, _ ...grpc.CallOption) (*pb.CreateWalletResponse, error) {
	s.seed = in.Seed
	return &pb.CreateWalletResponse{}, nil
}

// StartMixer refuses while the autobuyer runs, and even that early exit must
// not leave the passphrase behind.
func TestStartMixerZeroesThePassphrase(t *testing.T) {
	defer setAutobuyerRunning(true)()

	pass := secret()
	if err := StartMixer(pass, 1, 0, 2); err == nil {
		t.Fatal("the mixer started while the autobuyer was running")
	}
	requireWiped(t, pass, "the mixer passphrase")
}

// The copy the ticket purchase hands over has no other owner, so this is the
// one that used to leak on every mixed purchase.
func TestRestartMixerAfterPurchaseZeroesItsCopy(t *testing.T) {
	withHooks(t, "beta")

	pass := secret()
	if err := restartMixerAfterPurchase("alpha", pass, 1, 0, 2); err == nil {
		t.Fatal("the restart ran against a wallet it did not belong to")
	}
	requireWiped(t, pass, "the purchase's mixer passphrase")
}

func TestStartAutobuyerZeroesThePassphrase(t *testing.T) {
	if !tryBeginTicketPurchase() {
		t.Fatal("a purchase was already marked active")
	}
	t.Cleanup(endTicketPurchase)

	pass := secret()
	if err := StartAutobuyer(&types.AutobuyerSettings{VspHost: "vsp.example.org", VspPubkey: "pk"}, pass); err == nil {
		t.Fatal("the autobuyer started during a ticket purchase")
	}
	requireWiped(t, pass, "the autobuyer passphrase")
}
