// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"dcrpulse/internal/rpc"

	"github.com/decred/dcrlnd/lnrpc"
	"google.golang.org/grpc"
)

type fakeGraphClient struct {
	lnrpc.LightningClient // embedded: anything unexpected panics

	graph  *lnrpc.ChannelGraph
	err    error
	calls  int
	ctxErr error
}

func (f *fakeGraphClient) DescribeGraph(ctx context.Context, in *lnrpc.ChannelGraphRequest, _ ...grpc.CallOption) (*lnrpc.ChannelGraph, error) {
	f.calls++
	f.ctxErr = ctx.Err()
	if f.err != nil {
		return nil, f.err
	}
	return f.graph, nil
}

func withFakeGraph(t *testing.T, f *fakeGraphClient) {
	t.Helper()
	prev := rpc.LightningClient
	rpc.LightningClient = f
	t.Cleanup(func() { rpc.LightningClient = prev })
}

func resetGraphCache(t *testing.T) {
	t.Helper()
	describeGraphMu.Lock()
	describeGraphData, describeGraphTime = nil, time.Time{}
	describeGraphMu.Unlock()
}

// expireGraphCache backdates the fill time rather than shortening the TTL, so
// no global is left modified and the real expiry comparison is exercised.
func expireGraphCache(t *testing.T) {
	t.Helper()
	describeGraphMu.Lock()
	describeGraphTime = time.Now().Add(-describeGraphTTL - time.Second)
	describeGraphMu.Unlock()
}

// testGraph aggregates to 02aa 500 atoms over 2 channels, 02bb 300 over 1 and
// 03cc 200 over 1, so the snapshot sorts 02aa, 02bb, 03cc. 03cc carries no
// alias, which is the only way to reach the pubkey-only match path.
func testGraph() *lnrpc.ChannelGraph {
	return &lnrpc.ChannelGraph{
		Nodes: []*lnrpc.LightningNode{
			{PubKey: "02aa", Alias: "Hub One", Color: "#ff0000"},
			{PubKey: "02bb", Alias: "relay"},
			{PubKey: "03cc"},
		},
		Edges: []*lnrpc.ChannelEdge{
			{Node1Pub: "02aa", Node2Pub: "02bb", Capacity: 300},
			{Node1Pub: "02aa", Node2Pub: "03cc", Capacity: 200},
		},
	}
}

// Each edge credits both of its endpoints, so a node's capacity is the sum over
// every channel it appears in.
func TestGetTopLightningNodesAggregatesEdgeCapacity(t *testing.T) {
	resetGraphCache(t)
	f := &fakeGraphClient{graph: testGraph()}
	withFakeGraph(t, f)

	top, err := GetTopLightningNodes(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetTopLightningNodes: %v", err)
	}

	if len(top) != 3 {
		t.Fatalf("got %d nodes, want 3", len(top))
	}
	if top[0].Pubkey != "02aa" || top[0].CapacityAtoms != 500 || top[0].NumChannels != 2 {
		t.Errorf("top node = %+v, want 02aa with 500 atoms over 2 channels", top[0])
	}
	for i := 1; i < len(top); i++ {
		if top[i-1].CapacityAtoms < top[i].CapacityAtoms {
			t.Errorf("nodes are not sorted by capacity: %+v", top)
		}
	}
}

// The fetch is shared, so a caller that gives up must not cancel it out from
// under the callers still waiting on it.
func TestGetTopLightningNodesIgnoresCallerCancellation(t *testing.T) {
	resetGraphCache(t)
	f := &fakeGraphClient{graph: testGraph()}
	withFakeGraph(t, f)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	top, err := GetTopLightningNodes(ctx, 10)
	if err != nil {
		t.Fatalf("GetTopLightningNodes: %v", err)
	}
	if f.calls != 1 {
		t.Fatalf("DescribeGraph called %d times, want 1", f.calls)
	}
	if f.ctxErr != nil {
		t.Errorf("the fetch saw the caller's cancellation: %v", f.ctxErr)
	}
	if len(top) != 3 {
		t.Errorf("got %d nodes, want 3", len(top))
	}
}
