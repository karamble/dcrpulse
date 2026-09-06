// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func resourceURIs(t *testing.T, cs *mcp.ClientSession) map[string]bool {
	t.Helper()
	res, err := cs.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	m := map[string]bool{}
	for _, r := range res.Resources {
		m[r.URI] = true
	}
	return m
}

// TestNodeOnlyAgentSeesOnlyNodeResources mirrors the tool gating: a node-only
// agent sees the node resource and none of the others.
func TestNodeOnlyAgentSeesOnlyNodeResources(t *testing.T) {
	a := testAgent("rn", "node-only", map[string]bool{"node": true})
	uris := resourceURIs(t, connectTo(t, a))
	if !uris[resNodeSync] {
		t.Errorf("node-only agent missing %s", resNodeSync)
	}
	for _, deny := range []string{resWalletSync, resWalletBal, resBRMessages, resLightning, resStaking, resMixer, resAudit, resBRMCP} {
		if uris[deny] {
			t.Errorf("node-only agent must NOT expose %s", deny)
		}
	}
}

// TestAllDomainsAgentSeesAllResources confirms every catalog resource registers
// for an agent granted all domains (including the resource-only audit domain).
func TestAllDomainsAgentSeesAllResources(t *testing.T) {
	domains := map[string]bool{}
	for _, d := range catalogDomains() {
		domains[d] = true
	}
	a := testAgent("rall", "all", domains)
	uris := resourceURIs(t, connectTo(t, a))
	if len(uris) != len(resourceCatalog) {
		t.Fatalf("expected %d resources, server exposed %d", len(resourceCatalog), len(uris))
	}
}

// TestAuditIsResourceOnlyDomain checks the audit domain is offered as a grantable
// capability (so the UI shows a toggle) yet contributes no tools.
func TestAuditIsResourceOnlyDomain(t *testing.T) {
	found := false
	for _, d := range catalogDomains() {
		if d == domainAudit {
			found = true
		}
	}
	if !found {
		t.Fatal("audit domain missing from catalogDomains")
	}
	for _, td := range toolCatalog {
		if td.domain == domainAudit {
			t.Fatal("audit domain should have no tools")
		}
	}
}

// TestBRMCPIsResourceOnlyDomain checks the same for the bridge domain: grantable,
// so the UI shows a toggle, yet no tool can act on the bridge through it.
func TestBRMCPIsResourceOnlyDomain(t *testing.T) {
	found := false
	for _, d := range catalogDomains() {
		if d == domainBRMCP {
			found = true
		}
	}
	if !found {
		t.Fatal("brmcp domain missing from catalogDomains")
	}
	for _, td := range toolCatalog {
		if td.domain == domainBRMCP {
			t.Fatal("brmcp domain should have no tools")
		}
	}
}

// TestNotifyResourceUpdatedNoSubscribersNoPanic ensures the fan-out is a safe
// no-op when no per-agent servers are registered.
func TestNotifyResourceUpdatedNoSubscribersNoPanic(t *testing.T) {
	notifyResourceUpdated(resNodeSync)
	notifyResourceUpdated(resAudit)
}

// TestResourceSubscribeGating verifies an agent cannot subscribe to a resource
// outside its granted domains but can subscribe within them. On the 2026-07-28
// wire ClientSession.Subscribe opens its subscriptions/listen stream
// asynchronously and swallows a server refusal, so the deny is asserted on the
// gate itself here, on the legacy wire in TestLegacyWireHTTP, and behaviorally
// in TestResourceSubscribePushNewWire.
func TestResourceSubscribeGating(t *testing.T) {
	a := testAgent("rs", "node-only", map[string]bool{"node": true})
	cs := connectTo(t, a)
	if err := cs.Subscribe(context.Background(), &mcp.SubscribeParams{URI: resNodeSync}); err != nil {
		t.Errorf("subscribe to granted node resource failed: %v", err)
	}
	if err := allowResourceSub(a, resNodeSync); err != nil {
		t.Errorf("gate refused granted node resource: %v", err)
	}
	if err := allowResourceSub(a, resAudit); err == nil {
		t.Error("gate allowed the ungranted audit resource")
	}
}

// TestResourceSubscribePushNewWire proves the 2026-07-28 subscription path end
// to end: a granted subscription receives ResourceUpdated pushes over its
// subscriptions/listen stream while a denied one stays silent. Subscribe
// registers asynchronously on this wire, so the triggers retry until a push
// lands.
func TestResourceSubscribePushNewWire(t *testing.T) {
	surfaceUpForTest(t)
	a := testAgent("rp", "node-only", map[string]bool{"node": true})
	var mu sync.Mutex
	got := map[string]int{}
	srv, cs := connectToWithOptions(t, a, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			mu.Lock()
			got[req.Params.URI]++
			mu.Unlock()
		},
	})
	ctx := context.Background()
	count := func(uri string) int {
		mu.Lock()
		defer mu.Unlock()
		return got[uri]
	}

	if err := cs.Subscribe(ctx, &mcp.SubscribeParams{URI: resNodeSync}); err != nil {
		t.Fatalf("subscribe granted: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for count(resNodeSync) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no resources/updated push for the granted subscription")
		}
		srv.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: resNodeSync})
		time.Sleep(20 * time.Millisecond)
	}

	// The denied listen errors before any subscription registers, so audit
	// pushes can never arrive even though Subscribe reported nothing.
	if err := cs.Subscribe(ctx, &mcp.SubscribeParams{URI: resAudit}); err != nil {
		t.Fatalf("async subscribe reported: %v", err)
	}
	base := count(resNodeSync)
	deadline = time.Now().Add(10 * time.Second)
	for count(resNodeSync) < base+2 {
		if time.Now().After(deadline) {
			t.Fatal("granted subscription stopped receiving pushes")
		}
		srv.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: resAudit})
		srv.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: resNodeSync})
		time.Sleep(20 * time.Millisecond)
	}
	if n := count(resAudit); n != 0 {
		t.Fatalf("denied subscription received %d pushes", n)
	}
}
