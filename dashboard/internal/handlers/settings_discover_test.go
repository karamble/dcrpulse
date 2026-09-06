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
// returns a snapshot reader for each. Readers rather than the slices themselves:
// the handler records from a detached goroutine, so every read has to take the
// same lock the write did.
func discoverHarness(t *testing.T, f *fakeDiscoverWallet) (persisted func() []int, rescans func() []int32) {
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
	return func() []int {
			mu.Lock()
			defer mu.Unlock()
			return append([]int(nil), gaps...)
		}, func() []int32 {
			mu.Lock()
			defer mu.Unlock()
			return append([]int32(nil), heights...)
		}
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
	if got := persisted(); len(got) != 0 {
		// Mutation catch: persisting before the scan runs records the refused gap.
		t.Fatalf("a refused attempt persisted gaps %v", got)
	}
	if got := rescans(); len(got) != 0 {
		t.Fatalf("a refused attempt started a rescan: %v", got)
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
	for time.Now().Before(deadline) && len(rescans()) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if got := persisted(); len(got) != 1 || got[0] != 500 {
		t.Fatalf("persisted gaps = %v, want [500]", got)
	}
	if got := rescans(); len(got) != 1 || got[0] != 0 {
		// Mutation catch: dropping the rescan hand-off leaves this empty.
		t.Fatalf("rescan heights = %v, want [0]", got)
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
