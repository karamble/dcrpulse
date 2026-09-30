// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"dcrpulse/internal/rpc"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"google.golang.org/grpc"
)

type shutdownLog struct {
	mu     sync.Mutex
	events []string
}

func (l *shutdownLog) add(ev string) {
	l.mu.Lock()
	l.events = append(l.events, ev)
	l.mu.Unlock()
}

func (l *shutdownLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

type shutdownWallet struct {
	pb.WalletServiceClient // panic on anything unexpected

	log *shutdownLog
}

func (w *shutdownWallet) Accounts(ctx context.Context, req *pb.AccountsRequest, _ ...grpc.CallOption) (*pb.AccountsResponse, error) {
	return &pb.AccountsResponse{Accounts: []*pb.AccountsResponse_Account{
		{AccountNumber: 3, AccountName: "mixed", AccountUnlocked: true, AccountEncrypted: true},
	}}, nil
}

func (w *shutdownWallet) GetTrackedVSPTickets(ctx context.Context, req *pb.GetTrackedVSPTicketsRequest, _ ...grpc.CallOption) (*pb.GetTrackedVSPTicketsResponse, error) {
	return &pb.GetTrackedVSPTicketsResponse{}, nil
}

func (w *shutdownWallet) LockAccount(ctx context.Context, req *pb.LockAccountRequest, _ ...grpc.CallOption) (*pb.LockAccountResponse, error) {
	w.log.add(fmt.Sprintf("lock %d", req.AccountNumber))
	return &pb.LockAccountResponse{}, nil
}

func useShutdownWallet(t *testing.T, log *shutdownLog) {
	t.Helper()
	prev := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = &shutdownWallet{log: log}
	t.Cleanup(func() {
		rpc.WalletGrpcClient = prev
		shuttingDown.Store(false)
	})
}

// finishLater stands in for a worker whose stop path takes a moment, as the
// mixer's and the autobuyer's relocks do.
func finishLater(log *shutdownLog, name string, after time.Duration, done func()) {
	go func() {
		time.Sleep(after)
		log.add(name)
		done()
	}()
}

// The final lock runs only once the mixer, the autobuyer, a vote trickle and a
// spend in flight have all finished, as Decrediton closes a wallet; and nothing
// may unlock an account after the shutdown has begun.
func TestStopWalletWorkLocksAfterTheWorkStops(t *testing.T) {
	log := &shutdownLog{}
	useShutdownWallet(t, log)

	mixerMu.Lock()
	mixerCancel = func() {
		finishLater(log, "mixer", 100*time.Millisecond, func() { mixerMu.Lock(); mixerCancel = nil; mixerMu.Unlock() })
	}
	mixerMu.Unlock()
	autobuyerMu.Lock()
	autobuyerCancel = func() {
		finishLater(log, "autobuyer", 150*time.Millisecond, func() { autobuyerMu.Lock(); autobuyerCancel = nil; autobuyerMu.Unlock() })
	}
	autobuyerMu.Unlock()
	st := &vtRunState{token: "trickle"}
	st.cancel = func() {
		finishLater(log, "trickle", 300*time.Millisecond, func() { vtMu.Lock(); st.done = true; vtMu.Unlock() })
	}
	vtMu.Lock()
	vtRuns[st.token] = st
	vtMu.Unlock()
	t.Cleanup(func() { vtMu.Lock(); delete(vtRuns, st.token); vtMu.Unlock() })
	beginUnlockedOp()
	finishLater(log, "spend", 400*time.Millisecond, endUnlockedOp)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	StopWalletWork(ctx)

	got := log.snapshot()
	if len(got) != 5 || got[4] != "lock 3" {
		t.Fatalf("events %v, want the mixer, autobuyer, trickle and spend to finish before \"lock 3\"", got)
	}
	if _, err := unlockAccountForSpend(ctx, 3, []byte("pass")); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("an unlock after the shutdown began returned %v, want ErrShuttingDown", err)
	}
}

// A ticket purchase can outlast the grace period. It is left to be cut off and
// the next boot says so; no account is locked under it.
func TestStopWalletWorkReportsACutOffPurchase(t *testing.T) {
	log := &shutdownLog{}
	useShutdownWallet(t, log)
	if !tryBeginTicketPurchase() {
		t.Fatal("a purchase is already marked active")
	}
	t.Cleanup(endTicketPurchase)
	beginUnlockedOp()
	t.Cleanup(endUnlockedOp)
	var codes []string
	prev := emitShutdownAlert
	emitShutdownAlert = func(code, detail, dedupeKey string) { codes = append(codes, code) }
	t.Cleanup(func() { emitShutdownAlert = prev })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	StopWalletWork(ctx)

	if len(codes) != 1 || codes[0] != "ticket_purchase_interrupted" {
		t.Fatalf("alerts %v, want one ticket_purchase_interrupted", codes)
	}
	if got := log.snapshot(); len(got) != 0 {
		t.Fatalf("events %v, want no lock while the purchase holds its account", got)
	}
}
