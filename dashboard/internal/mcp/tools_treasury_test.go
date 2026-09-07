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

// The scan spends nothing, but it wipes the previous results and keeps dcrd busy
// for minutes, and the dashboard route allows one start a minute for that
// reason. The agent path draws from that same bucket, so an agent cannot restart
// the scan back-to-back just because the tool carries the read-only hint.
func TestTreasuryScanStartSharesTheDashboardLimiter(t *testing.T) {
	// A client every call fails on at once, so a scan that does start (only
	// possible with the limiter gone) fails cleanly instead of dereferencing
	// a nil client in its goroutine.
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

	// Drain the bucket the way a browser click would.
	lim := middleware.Limiter("treasury-scan", 60*time.Second, 1)
	for lim.Allow() {
	}
	if middleware.Limiter("treasury-scan", 60*time.Second, 1) != lim {
		t.Fatal("the registry handed out a second bucket for the same name; nothing is shared")
	}

	domains := map[string]bool{}
	for _, d := range catalogDomains() {
		domains[d] = true
	}
	cs := connectTo(t, testAgent("scan-limit", "scan", domains))
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "treasury_scan_start", Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !out.IsError || !strings.Contains(resultText(out), "rate limit") {
		t.Fatalf("a scan started within the minute of the last one: %q", resultText(out))
	}
	if p, _ := services.GetScanProgress(); p != nil && p.IsScanning {
		t.Fatal("a scan is running despite the refusal")
	}
}
