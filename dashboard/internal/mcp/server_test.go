// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	a := &agent{id: "n", name: "node-only", domains: map[string]bool{"node": true}}
	names := toolNames(t, connectTo(t, a))
	for _, want := range []string{"node_status", "node_dashboard", "node_blockchain_info"} {
		if !names[want] {
			t.Errorf("node-only agent missing expected tool %q", want)
		}
	}
	for _, deny := range []string{"wallet_dashboard", "wallet_accounts", "staking_tickets"} {
		if names[deny] {
			t.Errorf("node-only agent must NOT expose %q", deny)
		}
	}
}

func TestGrantedAgentSeesMoreTools(t *testing.T) {
	a := &agent{id: "g", name: "granted", domains: map[string]bool{"node": true, "wallet": true, "staking": true}}
	names := toolNames(t, connectTo(t, a))
	for _, want := range []string{"node_status", "wallet_dashboard", "wallet_accounts", "staking_tickets"} {
		if !names[want] {
			t.Errorf("granted agent missing expected tool %q", want)
		}
	}
}
