// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/decred/dcrlnd/lnrpc"
	"google.golang.org/grpc"
)

// lightningNodeAlias used to read the package-level client itself, inside the
// channel-enrichment loop and after several slow RPCs, so a wallet switch could
// nil it mid-request and panic the handler goroutine. Taking the client as a
// parameter means the caller captures once and the whole request runs against
// the client it started with.

type fakeLightningClient struct {
	lnrpc.LightningClient // embedded: anything unexpected panics

	alias string
	err   error
	calls int
}

func (f *fakeLightningClient) GetNodeInfo(ctx context.Context, in *lnrpc.NodeInfoRequest, _ ...grpc.CallOption) (*lnrpc.NodeInfo, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &lnrpc.NodeInfo{Node: &lnrpc.LightningNode{Alias: f.alias}}, nil
}

func resetAliasCache(t *testing.T) {
	t.Helper()
	nodeAliasMu.Lock()
	nodeAliasCache = map[string]string{}
	nodeAliasMu.Unlock()
}

// The passed client is the one used; the global is never consulted. Left nil
// here deliberately, so touching it would panic the test.
func TestLightningNodeAliasUsesPassedClient(t *testing.T) {
	resetAliasCache(t)
	f := &fakeLightningClient{alias: "hoppy"}

	if got := lightningNodeAlias(context.Background(), f, "02abc"); got != "hoppy" {
		t.Errorf("alias = %q, want %q", got, "hoppy")
	}
	if f.calls != 1 {
		t.Errorf("GetNodeInfo called %d times, want 1", f.calls)
	}
}

func TestLightningNodeAliasCachesHits(t *testing.T) {
	resetAliasCache(t)
	f := &fakeLightningClient{alias: "hoppy"}

	lightningNodeAlias(context.Background(), f, "02abc")
	lightningNodeAlias(context.Background(), f, "02abc")

	if f.calls != 1 {
		t.Errorf("GetNodeInfo called %d times, want 1: the second lookup should hit the cache", f.calls)
	}
}

// Misses are deliberately not cached, so a node that appears in the graph later
// resolves on a subsequent call rather than staying blank for the process life.
func TestLightningNodeAliasDoesNotCacheMisses(t *testing.T) {
	resetAliasCache(t)
	f := &fakeLightningClient{err: errors.New("unknown node")}

	if got := lightningNodeAlias(context.Background(), f, "02abc"); got != "" {
		t.Errorf("alias = %q, want empty on error", got)
	}
	f.err, f.alias = nil, "hoppy"
	if got := lightningNodeAlias(context.Background(), f, "02abc"); got != "hoppy" {
		t.Errorf("alias = %q, want %q once the node resolves", got, "hoppy")
	}
	if f.calls != 2 {
		t.Errorf("GetNodeInfo called %d times, want 2", f.calls)
	}
}

func TestLightningNodeAliasSkipsEmptyPubkey(t *testing.T) {
	resetAliasCache(t)
	f := &fakeLightningClient{alias: "hoppy"}

	if got := lightningNodeAlias(context.Background(), f, ""); got != "" {
		t.Errorf("alias = %q, want empty for an empty pubkey", got)
	}
	if f.calls != 0 {
		t.Errorf("GetNodeInfo called %d times for an empty pubkey, want 0", f.calls)
	}
}
