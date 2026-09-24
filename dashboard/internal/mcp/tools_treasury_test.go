// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package mcp

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
	"github.com/decred/dcrd/rpcclient/v8"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTreasuryScanMCPRejectsAboveTip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID     json.RawMessage
			Method string
		}
		json.NewDecoder(r.Body).Decode(&q)
		if q.Method != "getblockcount" {
			t.Errorf("invalid request did block work: %s", q.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": q.ID, "result": 600000, "error": nil})
	}))
	defer srv.Close()
	c, err := rpcclient.New(&rpcclient.ConnConfig{Host: strings.TrimPrefix(srv.URL, "http://"), User: "u", Pass: "p", HTTPPostMode: true, DisableTLS: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Shutdown()
	old := rpc.DcrdClient
	rpc.DcrdClient = c
	defer func() { rpc.DcrdClient = old }()
	// Isolate allowance state from tests that deliberately drain the shared bucket.
	oldAllowance := middleware.TreasuryScan
	middleware.TreasuryScan = middleware.Allowance{Name: t.Name(), Every: time.Minute, Burst: 3}
	defer func() { middleware.TreasuryScan = oldAllowance }()
	cs := connectTo(t, testAgent("treasury-height", "treasury", map[string]bool{"treasury": true}))
	for _, h := range []int64{600001, math.MaxInt64 - 1023, math.MaxInt64} {
		res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "treasury_scan_start", Arguments: map[string]any{"startHeight": h}})
		if err != nil {
			t.Fatal(err)
		}
		// The SDK may round MaxInt64 through float64 and reject it at decoding.
		if !res.IsError || (h != math.MaxInt64 && !strings.Contains(resultText(res), "exceeds current chain tip 600000")) {
			t.Fatalf("lost validation error: %s", resultText(res))
		}
	}
	progress, _ := services.GetScanProgress()
	if progress.IsScanning {
		t.Fatal("MCP started invalid scan")
	}
}
