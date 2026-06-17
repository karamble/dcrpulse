// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"testing"

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
	a := &agent{id: "rn", name: "node-only", domains: map[string]bool{"node": true}}
	uris := resourceURIs(t, connectTo(t, a))
	if !uris[resNodeSync] {
		t.Errorf("node-only agent missing %s", resNodeSync)
	}
	for _, deny := range []string{resWalletSync, resWalletBal, resBRMessages, resLightning, resStaking, resMixer, resAudit} {
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
	a := &agent{id: "rall", name: "all", domains: domains}
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

// TestNotifyResourceUpdatedNoSubscribersNoPanic ensures the fan-out is a safe
// no-op when no per-agent servers are registered.
func TestNotifyResourceUpdatedNoSubscribersNoPanic(t *testing.T) {
	notifyResourceUpdated(resNodeSync)
	notifyResourceUpdated(resAudit)
}

// TestResourceSubscribeGating verifies an agent cannot subscribe to a resource
// outside its granted domains but can subscribe within them.
func TestResourceSubscribeGating(t *testing.T) {
	a := &agent{id: "rs", name: "node-only", domains: map[string]bool{"node": true}}
	cs := connectTo(t, a)
	ctx := context.Background()
	if err := cs.Subscribe(ctx, &mcp.SubscribeParams{URI: resNodeSync}); err != nil {
		t.Errorf("subscribe to granted node resource failed: %v", err)
	}
	if err := cs.Subscribe(ctx, &mcp.SubscribeParams{URI: resAudit}); err == nil {
		t.Error("subscribe to ungranted audit resource should be refused")
	}
}
