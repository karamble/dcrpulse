// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"dcrpulse/internal/rpc"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"google.golang.org/grpc"
)

// fakeDiscoverWallet drives services.DiscoverUsage without a wallet: unlock
// can be told to refuse, and every step is recorded in order.
type fakeDiscoverWallet struct {
	pb.WalletServiceClient

	unlockErr error

	mu     sync.Mutex
	events []string
}

func (f *fakeDiscoverWallet) record(ev string) {
	f.mu.Lock()
	f.events = append(f.events, ev)
	f.mu.Unlock()
}

func (f *fakeDiscoverWallet) UnlockWallet(ctx context.Context, in *pb.UnlockWalletRequest, _ ...grpc.CallOption) (*pb.UnlockWalletResponse, error) {
	if f.unlockErr != nil {
		return nil, f.unlockErr
	}
	f.record("unlock")
	return &pb.UnlockWalletResponse{}, nil
}

func (f *fakeDiscoverWallet) LockWallet(ctx context.Context, in *pb.LockWalletRequest, _ ...grpc.CallOption) (*pb.LockWalletResponse, error) {
	f.record("lock")
	return &pb.LockWalletResponse{}, nil
}

func (f *fakeDiscoverWallet) DiscoverUsage(ctx context.Context, in *pb.DiscoverUsageRequest, _ ...grpc.CallOption) (*pb.DiscoverUsageResponse, error) {
	f.record("discover")
	return &pb.DiscoverUsageResponse{}, nil
}

// discoverHarness wires the fake wallet plus the persist/rescan seams and
// returns recorders for both.
func discoverHarness(t *testing.T, f *fakeDiscoverWallet) (persisted *[]int, rescans *[]int32) {
	t.Helper()
	prevClient := rpc.WalletGrpcClient
	rpc.WalletGrpcClient = f
	prevPersist, prevRescan, prevDelay := persistDiscoveryGap, discoverRescan, discoverRescanDelay
	var mu sync.Mutex
	var gaps []int
	var heights []int32
	persistDiscoveryGap = func(_ context.Context, gap int) {
		mu.Lock()
		gaps = append(gaps, gap)
		mu.Unlock()
		f.record("persist")
	}
	discoverRescan = func(h int32) {
		mu.Lock()
		heights = append(heights, h)
		mu.Unlock()
		f.record("rescan")
	}
	discoverRescanDelay = 0
	t.Cleanup(func() {
		rpc.WalletGrpcClient = prevClient
		persistDiscoveryGap, discoverRescan, discoverRescanDelay = prevPersist, prevRescan, prevDelay
	})
	return &gaps, &heights
}

func postDiscover(body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/wallet/settings/discover-addresses", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	DiscoverAddressesHandler(rec, req)
	return rec
}

// A refused passphrase must not change the stored preference and must not
// start a rescan - the failed attempt described a scan that never ran.
func TestDiscoverRefusalPersistsNothing(t *testing.T) {
	f := &fakeDiscoverWallet{unlockErr: contextErr("invalid passphrase")}
	persisted, rescans := discoverHarness(t, f)

	rec := postDiscover(`{"passphrase":"wrong","gapLimit":500}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong passphrase answered %d, want 401", rec.Code)
	}
	time.Sleep(50 * time.Millisecond) // give a buggy detached rescan time to appear
	if len(*persisted) != 0 {
		// Mutation catch: persisting before the scan runs records the refused gap.
		t.Fatalf("a refused attempt persisted gaps %v", *persisted)
	}
	if len(*rescans) != 0 {
		t.Fatalf("a refused attempt started a rescan: %v", *rescans)
	}
}

// A successful discovery persists the gap and hands off to a rescan from
// height 0, in that order - discovery marks addresses used, the rescan
// fetches their history.
func TestDiscoverSuccessPersistsThenRescans(t *testing.T) {
	f := &fakeDiscoverWallet{}
	persisted, rescans := discoverHarness(t, f)

	rec := postDiscover(`{"passphrase":"right","gapLimit":500}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("discover answered %d, want 204: %s", rec.Code, rec.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(*rescans)
		f.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(*persisted) != 1 || (*persisted)[0] != 500 {
		t.Fatalf("persisted gaps = %v, want [500]", *persisted)
	}
	if len(*rescans) != 1 || (*rescans)[0] != 0 {
		// Mutation catch: dropping the rescan hand-off leaves this empty.
		t.Fatalf("rescan heights = %v, want [0]", *rescans)
	}
	f.mu.Lock()
	events := append([]string(nil), f.events...)
	f.mu.Unlock()
	seen := map[string]int{}
	for i, ev := range events {
		if _, ok := seen[ev]; !ok {
			seen[ev] = i
		}
	}
	if !(seen["discover"] < seen["persist"] && seen["persist"] < seen["rescan"]) {
		t.Fatalf("order wrong: %v", events)
	}
}

func TestDiscoverRejectsOversizedGap(t *testing.T) {
	f := &fakeDiscoverWallet{}
	discoverHarness(t, f)
	rec := postDiscover(`{"passphrase":"x","gapLimit":20000}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized gap answered %d, want 400", rec.Code)
	}
}

// contextErr builds a plain error without importing errors twice in tests.
func contextErr(msg string) error { return &discoverErr{msg} }

type discoverErr struct{ s string }

func (e *discoverErr) Error() string { return e.s }
