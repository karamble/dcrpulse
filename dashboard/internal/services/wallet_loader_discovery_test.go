// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"

	"dcrpulse/internal/rpc"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

// A restore's discovery goroutine runs for minutes or hours on a detached
// context, and the passphrase step it ends with is the one cleanup path in this
// package that retries, so it is the only one that can still reach the shared
// gRPC clients after a wallet switch has repointed them. These pin both exits.

type fakeSyncStream struct {
	grpc.ServerStreamingClient[pb.RpcSyncResponse]
	resp   *pb.RpcSyncResponse
	err    error
	sent   bool
	onRecv func() // fires mid-stream, where a real switch would land
}

func (f *fakeSyncStream) Recv() (*pb.RpcSyncResponse, error) {
	if f.sent {
		return nil, fmt.Errorf("stream closed")
	}
	f.sent = true
	if f.onRecv != nil {
		f.onRecv()
	}
	return f.resp, f.err
}

type fakeLoader struct {
	pb.WalletLoaderServiceClient // panic on anything unexpected
	stream                       *fakeSyncStream
}

func (f *fakeLoader) RpcSync(ctx context.Context, req *pb.RpcSyncRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[pb.RpcSyncResponse], error) {
	return f.stream, nil
}

// countingWallet records whether the passphrase step was reached at all.
type countingWallet struct {
	pb.WalletServiceClient // panic on anything unexpected
	accountsCalls          int
}

func (c *countingWallet) Accounts(ctx context.Context, req *pb.AccountsRequest, _ ...grpc.CallOption) (*pb.AccountsResponse, error) {
	c.accountsCalls++
	return &pb.AccountsResponse{}, nil
}

func withDiscoveryFakes(t *testing.T, stream *fakeSyncStream) *countingWallet {
	t.Helper()
	w := &countingWallet{}
	prevLoader, prevWallet := rpc.WalletLoaderClient, rpc.WalletGrpcClient
	rpc.WalletLoaderClient = &fakeLoader{stream: stream}
	rpc.WalletGrpcClient = w
	t.Cleanup(func() {
		rpc.WalletLoaderClient, rpc.WalletGrpcClient = prevLoader, prevWallet
	})
	return w
}

// A stream that ends in error never reached SYNCED, so the accounts the
// passphrase step would seal may not all exist yet. Falling through also let it
// run against whatever wallet a switch had since loaded.
func TestDiscoveryStopsWhenTheStreamFails(t *testing.T) {
	withHooks(t, "alpha")
	w := withDiscoveryFakes(t, &fakeSyncStream{err: fmt.Errorf("daemon went away")})

	runDiscoveryRpcSync([]byte("passphrase"))

	if w.accountsCalls != 0 {
		t.Fatalf("the passphrase step ran %d times after a failed discovery stream, want 0", w.accountsCalls)
	}
}

// Even on a clean SYNCED, the step must not run against a wallet this discovery
// never belonged to.
func TestDiscoverySkipsPassphrasesAfterAWalletSwitch(t *testing.T) {
	withHooks(t, "alpha")
	// The switch lands while the scan is still streaming, which is the only
	// ordering that matters: before it starts there is nothing to protect.
	w := withDiscoveryFakes(t, &fakeSyncStream{
		resp:   &pb.RpcSyncResponse{Synced: true},
		onRecv: func() { setActiveWalletName("beta") },
	})

	runDiscoveryRpcSync([]byte("passphrase"))

	if w.accountsCalls != 0 {
		t.Fatalf("the passphrase step ran %d times against another wallet, want 0", w.accountsCalls)
	}
}

// The regression pin: a clean discovery on its own wallet must still finalize,
// or every restored wallet is left without per-account passphrases.
func TestDiscoveryStillSetsPassphrasesOnItsOwnWallet(t *testing.T) {
	withHooks(t, "alpha")
	w := withDiscoveryFakes(t, &fakeSyncStream{resp: &pb.RpcSyncResponse{Synced: true}})

	runDiscoveryRpcSync([]byte("passphrase"))

	if w.accountsCalls == 0 {
		t.Fatal("a clean discovery did not reach the passphrase step on its own wallet")
	}
}
