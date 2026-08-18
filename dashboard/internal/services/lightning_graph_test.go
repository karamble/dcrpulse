// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
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
	prev := rpc.SwapDcrlndClients(rpc.DcrlndClients{Lightning: f})
	t.Cleanup(func() { rpc.SwapDcrlndClients(prev) })
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

// The search box debounces at 250ms and fires again when the modal opens, so
// typing a four-letter alias used to cost five full graph dumps. All of them
// are answered from the one snapshot the network panel already holds.
func TestSearchLightningNodesReusesTheGraphCache(t *testing.T) {
	resetGraphCache(t)
	f := &fakeGraphClient{graph: testGraph()}
	withFakeGraph(t, f)

	if _, err := GetTopLightningNodes(context.Background(), 10); err != nil {
		t.Fatalf("GetTopLightningNodes: %v", err)
	}
	for _, q := range []string{"h", "hu", "hub", "hub0"} {
		if _, err := SearchLightningNodes(context.Background(), q); err != nil {
			t.Fatalf("SearchLightningNodes(%q): %v", q, err)
		}
	}

	if f.calls != 1 {
		t.Errorf("DescribeGraph called %d times, want 1", f.calls)
	}
}

// The modal's opening request is the one that warms the panel, not the other
// way round, so the order the two arrive in must not matter.
func TestSearchLightningNodesFillsTheCacheForTopNodes(t *testing.T) {
	resetGraphCache(t)
	f := &fakeGraphClient{graph: testGraph()}
	withFakeGraph(t, f)

	if _, err := SearchLightningNodes(context.Background(), ""); err != nil {
		t.Fatalf("SearchLightningNodes: %v", err)
	}
	top, err := GetTopLightningNodes(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetTopLightningNodes: %v", err)
	}

	if f.calls != 1 {
		t.Errorf("DescribeGraph called %d times, want 1", f.calls)
	}
	if len(top) != 3 {
		t.Errorf("top nodes = %d, want 3", len(top))
	}
}

func TestSearchLightningNodesRefetchesAfterTTL(t *testing.T) {
	resetGraphCache(t)
	f := &fakeGraphClient{graph: testGraph()}
	withFakeGraph(t, f)

	if _, err := SearchLightningNodes(context.Background(), "hub"); err != nil {
		t.Fatalf("first search: %v", err)
	}
	expireGraphCache(t)
	if _, err := SearchLightningNodes(context.Background(), "hub"); err != nil {
		t.Fatalf("second search: %v", err)
	}

	if f.calls != 2 {
		t.Errorf("DescribeGraph called %d times, want 2", f.calls)
	}
}

func TestSearchLightningNodesMatches(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"empty query returns every node", "", []string{"02aa", "02bb", "03cc"}},
		{"alias substring", "hub", []string{"02aa"}},
		{"alias match ignores case", "HUB", []string{"02aa"}},
		{"pubkey substring", "02b", []string{"02bb"}},
		{"surrounding whitespace is trimmed", "  relay  ", []string{"02bb"}},
		{"no match", "zzz", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetGraphCache(t)
			f := &fakeGraphClient{graph: testGraph()}
			withFakeGraph(t, f)

			got, err := SearchLightningNodes(context.Background(), tc.query)
			if err != nil {
				t.Fatalf("SearchLightningNodes: %v", err)
			}
			// The modal maps over this directly, so it must never be null.
			if got.Matches == nil {
				t.Fatal("Matches is nil, want an empty slice")
			}
			if len(got.Matches) != len(tc.want) {
				t.Fatalf("got %d matches, want %d: %+v", len(got.Matches), len(tc.want), got.Matches)
			}
			for i, pubkey := range tc.want {
				if got.Matches[i].Pubkey != pubkey {
					t.Errorf("match %d = %q, want %q", i, got.Matches[i].Pubkey, pubkey)
				}
			}
		})
	}
}

// Results follow the snapshot's order, largest capacity first, so the cap drops
// the smallest nodes rather than whichever ones the daemon happened to list
// last.
func TestSearchLightningNodesRanksByCapacity(t *testing.T) {
	resetGraphCache(t)
	// Capacity ascends with the graph order, so ranking by capacity is the
	// reverse of the order the daemon lists them in. A test built the other way
	// round would pass whether or not the snapshot is consulted.
	graph := &lnrpc.ChannelGraph{}
	for i := 0; i < 60; i++ {
		pubkey := fmt.Sprintf("02%04d", i)
		graph.Nodes = append(graph.Nodes, &lnrpc.LightningNode{PubKey: pubkey, Alias: "node"})
		graph.Edges = append(graph.Edges, &lnrpc.ChannelEdge{
			Node1Pub: pubkey,
			Capacity: int64(i + 1),
		})
	}
	f := &fakeGraphClient{graph: graph}
	withFakeGraph(t, f)

	got, err := SearchLightningNodes(context.Background(), "node")
	if err != nil {
		t.Fatalf("SearchLightningNodes: %v", err)
	}

	if len(got.Matches) != 50 {
		t.Fatalf("got %d matches, want the cap of 50", len(got.Matches))
	}
	if got.Matches[0].Pubkey != "020059" {
		t.Errorf("first match = %q, want the largest node 020059", got.Matches[0].Pubkey)
	}
	for _, m := range got.Matches {
		if m.Pubkey < "020010" {
			t.Errorf("%q is among the ten smallest and should have been cut", m.Pubkey)
		}
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
