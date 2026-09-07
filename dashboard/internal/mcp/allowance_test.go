// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/rpcclient/v8"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
)

// These jobs spend nothing, but each is minutes long or leans on a daemon or a
// third-party service, and the dashboard route for each allows one start per
// interval for that reason. The agent path draws from the same bucket, so an
// agent cannot restart a job back-to-back, and (for the granted tools) only a
// granted agent can spend the operator's allowance at all.
var throttledTools = []struct {
	name      string
	allowance middleware.Allowance
	scopes    []string // nil: no grant needed
}{
	{"treasury_scan_start", middleware.TreasuryScan, nil},
	{"dex_wallet_rescan", middleware.DexRescan, []string{scopeDex}},
	{"dex_discover_account", middleware.DexDiscover, []string{scopeDex}},
	{"staking_sync_failed_vsp_tickets", middleware.VSPSync, []string{scopeStaking}},
	{"staking_process_unmanaged_vsp_tickets", middleware.VSPUnmanaged, []string{scopeStaking}},
	{"timestamp_refresh", middleware.TimestampRefresh, []string{scopeTimestamp}},
}

func TestThrottledToolsShareTheDashboardBucket(t *testing.T) {
	// A dcrd client every call fails on at once, so a treasury scan that does
	// start (only possible with the limiter gone) fails cleanly instead of
	// dereferencing a nil client in its goroutine.
	c, err := rpcclient.New(&rpcclient.ConnConfig{
		Host: "127.0.0.1:1", User: "u", Pass: "p", HTTPPostMode: true, DisableTLS: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Shutdown)
	prev := rpc.DcrdClient
	rpc.DcrdClient = c
	t.Cleanup(func() { rpc.DcrdClient = prev })

	domains := map[string]bool{}
	for _, d := range catalogDomains() {
		domains[d] = true
	}
	for i, tc := range throttledTools {
		t.Run(tc.name, func(t *testing.T) {
			id := "throttle-" + tc.name
			if tc.scopes != nil {
				grants.set(id, GrantSpec{Accounts: []uint32{0, 1}, WriteScopes: tc.scopes}, time.Now())
				t.Cleanup(func() { grants.revoke(id) })
			}
			cs := connectTo(t, testAgent(id, "throttle", domains))
			res, err := cs.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			var args map[string]any
			for _, tl := range res.Tools {
				if tl.Name == tc.name {
					args = minimalArgs(tl.InputSchema)
				}
			}
			if args == nil {
				t.Fatalf("tool %s is not registered", tc.name)
			}
			// Drain the bucket the way a browser click would; the registry
			// must hand back the same bucket, or nothing is shared.
			lim := tc.allowance.Limiter()
			for lim.Allow() {
			}
			if tc.allowance.Limiter() != lim {
				t.Fatal("a second bucket for the same name; the tool and the route are not sharing")
			}

			out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.name, Arguments: args})
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if !out.IsError || !strings.Contains(resultText(out), "rate limit") {
				t.Fatalf("%d: ran within the interval of the last start: %q", i, resultText(out))
			}
		})
	}
	if p, _ := services.GetScanProgress(); p != nil && p.IsScanning {
		t.Fatal("a treasury scan is running despite the refusal")
	}
}
