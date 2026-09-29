// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/decred/dcrlnd/lnrpc"
)

func mAtomRoute() *lnrpc.Route {
	return &lnrpc.Route{
		TotalAmtMAtoms:  250_999,
		TotalFeesMAtoms: 1_500,
		Hops: []*lnrpc.Hop{
			{PubKey: "02aa", FeeMAtoms: 1_500, AmtToForwardMAtoms: 249_499},
			{PubKey: "03bb", FeeMAtoms: 0, AmtToForwardMAtoms: 249_000},
		},
	}
}

func TestRouteToTypeReadsMAtoms(t *testing.T) {
	r := routeToType(mAtomRoute())
	if r.TotalAmtAtoms != 250 || r.TotalFeesAtoms != 1 {
		t.Fatalf("totals = %d/%d, want 250/1", r.TotalAmtAtoms, r.TotalFeesAtoms)
	}
	if len(r.Hops) != 2 || r.Hops[0].FeeAtoms != 1 || r.Hops[0].AmtToForward != 249 || r.Hops[1].AmtToForward != 249 {
		t.Fatalf("hops = %+v", r.Hops)
	}
}

func TestPaymentToTypeReadsMAtoms(t *testing.T) {
	p := paymentToType(&lnrpc.Payment{Htlcs: []*lnrpc.HTLCAttempt{{Route: mAtomRoute()}}})
	if len(p.HTLCs) != 1 {
		t.Fatalf("htlcs = %d, want 1", len(p.HTLCs))
	}
	h := p.HTLCs[0]
	if h.TotalAmt != 250 || h.TotalFees != 1 || h.Hops[0].FeeAtoms != 1 || h.Hops[0].AmtToForward != 249 {
		t.Fatalf("htlc = %+v", h)
	}
	if p.Destination != "03bb" {
		t.Fatalf("destination = %q, want 03bb", p.Destination)
	}
}

func TestNodePolicyHtlcInAtoms(t *testing.T) {
	p := nodePolicyToType(&lnrpc.RoutingPolicy{MinHtlc: 1_000, MaxHtlcMAtoms: 1_500})
	if p.MinHtlcAtoms != 1 || p.MaxHtlcAtoms != 1 {
		t.Fatalf("min/max = %d/%d, want 1/1", p.MinHtlcAtoms, p.MaxHtlcAtoms)
	}
}

func TestEdgeLastUpdateIsLaterPolicy(t *testing.T) {
	cases := []struct {
		name string
		edge *lnrpc.ChannelEdge
		want uint32
	}{
		{"node2 later", &lnrpc.ChannelEdge{Node1Policy: &lnrpc.RoutingPolicy{LastUpdate: 100}, Node2Policy: &lnrpc.RoutingPolicy{LastUpdate: 200}}, 200},
		{"node1 later", &lnrpc.ChannelEdge{Node1Policy: &lnrpc.RoutingPolicy{LastUpdate: 300}, Node2Policy: &lnrpc.RoutingPolicy{LastUpdate: 200}}, 300},
		{"one policy", &lnrpc.ChannelEdge{Node2Policy: &lnrpc.RoutingPolicy{LastUpdate: 50}}, 50},
		{"no policies", &lnrpc.ChannelEdge{}, 0},
	}
	for _, c := range cases {
		if got := edgeLastUpdate(c.edge); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}
