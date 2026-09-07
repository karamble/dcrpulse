// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"google.golang.org/grpc"

	"dcrpulse/internal/rpc"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

// dcrwallet's wallet-wide lock zeroes EVERY account's key, including the
// per-account unlocks the mixer and the autobuyer hold for their whole run,
// while its wallet-wide unlock cannot reopen them. So checking a passphrase
// must never reach for that pair: it would silently disarm a running worker
// that keeps reporting itself healthy.

type verifyWallet struct {
	pb.WalletServiceClient // panic on anything unexpected

	unlockAccountErr error
	wasUnlocked      bool

	mu     sync.Mutex
	events []string
	opened []uint32
}

func (v *verifyWallet) record(ev string) {
	v.mu.Lock()
	v.events = append(v.events, ev)
	v.mu.Unlock()
}

func (v *verifyWallet) seen(ev string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, e := range v.events {
		if e == ev {
			return true
		}
	}
	return false
}

func (v *verifyWallet) UnlockWallet(context.Context, *pb.UnlockWalletRequest, ...grpc.CallOption) (*pb.UnlockWalletResponse, error) {
	v.record("unlock-wallet")
	return &pb.UnlockWalletResponse{}, nil
}

func (v *verifyWallet) LockWallet(context.Context, *pb.LockWalletRequest, ...grpc.CallOption) (*pb.LockWalletResponse, error) {
	v.record("lock-wallet")
	return &pb.LockWalletResponse{}, nil
}

func (v *verifyWallet) Accounts(context.Context, *pb.AccountsRequest, ...grpc.CallOption) (*pb.AccountsResponse, error) {
	return &pb.AccountsResponse{Accounts: []*pb.AccountsResponse_Account{
		{AccountNumber: 0, AccountName: "default", AccountUnlocked: v.wasUnlocked, AccountEncrypted: true},
	}}, nil
}

func (v *verifyWallet) UnlockAccount(_ context.Context, in *pb.UnlockAccountRequest, _ ...grpc.CallOption) (*pb.UnlockAccountResponse, error) {
	if v.unlockAccountErr != nil {
		return nil, v.unlockAccountErr
	}
	v.record("unlock-account")
	v.mu.Lock()
	v.opened = append(v.opened, in.AccountNumber)
	v.mu.Unlock()
	return &pb.UnlockAccountResponse{}, nil
}

func (v *verifyWallet) LockAccount(_ context.Context, in *pb.LockAccountRequest, _ ...grpc.CallOption) (*pb.LockAccountResponse, error) {
	v.record("lock-account")
	return &pb.LockAccountResponse{}, nil
}

func withVerifyWallet(t *testing.T, v *verifyWallet) {
	t.Helper()
	prev := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = v
	t.Cleanup(func() { rpc.WalletGrpcClient = prev })
}

func TestVerifyWalletPassphraseNeverLocksWalletWide(t *testing.T) {
	v := &verifyWallet{}
	withVerifyWallet(t, v)

	if err := VerifyWalletPassphrase(context.Background(), []byte("right")); err != nil {
		t.Fatalf("VerifyWalletPassphrase() = %v, want nil", err)
	}
	if v.seen("lock-wallet") || v.seen("unlock-wallet") {
		v.mu.Lock()
		defer v.mu.Unlock()
		t.Fatalf("the check used the wallet-wide lock: %v", v.events)
	}
	if !v.seen("unlock-account") {
		t.Fatal("the passphrase was never actually checked against an account")
	}
	v.mu.Lock()
	opened := append([]uint32(nil), v.opened...)
	v.mu.Unlock()
	if len(opened) != 1 || opened[0] != 0 {
		t.Fatalf("checked accounts %v, want only account 0", opened)
	}
}

// Verifying must leave the account exactly as it found it, or a check would
// leave a spendable key open on a wallet nobody asked to unlock.
func TestVerifyWalletPassphraseRelocksWhatItOpened(t *testing.T) {
	v := &verifyWallet{wasUnlocked: false}
	withVerifyWallet(t, v)

	if err := VerifyWalletPassphrase(context.Background(), []byte("right")); err != nil {
		t.Fatalf("VerifyWalletPassphrase() = %v, want nil", err)
	}
	if !v.seen("lock-account") {
		v.mu.Lock()
		defer v.mu.Unlock()
		t.Fatalf("an account opened by the check was left unlocked: %v", v.events)
	}
}

// An account the mixer or autobuyer already holds open must be left open.
func TestVerifyWalletPassphraseLeavesAnAlreadyOpenAccount(t *testing.T) {
	v := &verifyWallet{wasUnlocked: true}
	withVerifyWallet(t, v)

	if err := VerifyWalletPassphrase(context.Background(), []byte("right")); err != nil {
		t.Fatalf("VerifyWalletPassphrase() = %v, want nil", err)
	}
	if v.seen("lock-account") {
		v.mu.Lock()
		defer v.mu.Unlock()
		t.Fatalf("the check locked an account it did not open: %v", v.events)
	}
}

// The whole point of the call: a wrong passphrase must still be rejected.
func TestVerifyWalletPassphraseRejectsAWrongOne(t *testing.T) {
	v := &verifyWallet{unlockAccountErr: fmt.Errorf("invalid passphrase")}
	withVerifyWallet(t, v)

	if err := VerifyWalletPassphrase(context.Background(), []byte("wrong")); err == nil {
		t.Fatal("a wrong passphrase was accepted")
	}
}

// Discovery scans public data, so it needs no key at all; upstream reaches for
// the cointype private key only when discovering accounts, which this does not.
func TestDiscoverUsageNeverLocksWalletWide(t *testing.T) {
	v := &verifyWallet{}
	withVerifyWallet(t, v)

	if err := DiscoverUsage(context.Background(), 500); err != nil {
		t.Fatalf("DiscoverUsage() = %v, want nil", err)
	}
	if v.seen("lock-wallet") || v.seen("unlock-wallet") {
		v.mu.Lock()
		defer v.mu.Unlock()
		t.Fatalf("discovery touched the wallet-wide lock: %v", v.events)
	}
}

func (v *verifyWallet) DiscoverUsage(_ context.Context, in *pb.DiscoverUsageRequest, _ ...grpc.CallOption) (*pb.DiscoverUsageResponse, error) {
	if in.DiscoverAccounts {
		return nil, fmt.Errorf("account discovery needs the cointype key and must not be requested here")
	}
	v.record("discover")
	return &pb.DiscoverUsageResponse{}, nil
}
