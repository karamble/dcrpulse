// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// testAgent builds an agent with its domain set installed (domains is atomic,
// so it cannot be set in a struct literal).
func testAgent(id, name string, domains map[string]bool) *agent {
	a := &agent{id: id, name: name}
	a.setDomainMap(domains)
	return a
}

// connectTo wires an in-memory MCP client to a server scoped for agent a.
func connectTo(t *testing.T, a *agent) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	srvT, cliT := mcp.NewInMemoryTransports()
	ss, err := buildServer(a).Connect(ctx, srvT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, cliT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func toolNames(t *testing.T, cs *mcp.ClientSession) map[string]bool {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	m := map[string]bool{}
	for _, tl := range res.Tools {
		m[tl.Name] = true
	}
	return m
}

func TestNodeOnlyAgentSeesOnlyNodeTools(t *testing.T) {
	a := testAgent("n", "node-only", map[string]bool{"node": true})
	names := toolNames(t, connectTo(t, a))
	// capabilities is always available so any agent can introspect itself.
	for _, want := range []string{"node_status", "node_dashboard", "node_blockchain_info", "capabilities"} {
		if !names[want] {
			t.Errorf("node-only agent missing expected tool %q", want)
		}
	}
	for _, deny := range []string{"wallet_dashboard", "wallet_accounts", "staking_tickets", "wallet_send"} {
		if names[deny] {
			t.Errorf("node-only agent must NOT expose %q", deny)
		}
	}
}

func TestGrantedAgentSeesMoreTools(t *testing.T) {
	a := testAgent("g", "granted", map[string]bool{"node": true, "wallet": true, "staking": true})
	names := toolNames(t, connectTo(t, a))
	for _, want := range []string{"node_status", "wallet_dashboard", "wallet_accounts", "staking_tickets", "wallet_send"} {
		if !names[want] {
			t.Errorf("granted agent missing expected tool %q", want)
		}
	}
}

// TestFullCatalogRegistersUniquely grants every domain and confirms the server
// exposes exactly one tool per catalog entry. A duplicate tool name would make
// the count mismatch (or panic during registration), so this guards against it.
func TestFullCatalogRegistersUniquely(t *testing.T) {
	domains := map[string]bool{}
	for _, d := range catalogDomains() {
		domains[d] = true
	}
	a := testAgent("all", "all-domains", domains)
	names := toolNames(t, connectTo(t, a))
	if len(names) != len(toolCatalog) {
		t.Fatalf("expected %d unique tools, server exposed %d (duplicate tool name?)", len(toolCatalog), len(names))
	}
}
