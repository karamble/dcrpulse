// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/decred/dcrd/rpcclient/v8"
)

// dcrdRPCServer installs rpc.DcrdClient pointed at a JSON-RPC server that
// answers every request with an error, and returns the request counter.
func dcrdRPCServer(t *testing.T) *atomic.Int32 {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		rpcError(w, req.ID)
	}))
	t.Cleanup(srv.Close)
	c, err := rpcclient.New(&rpcclient.ConnConfig{
		Host: strings.TrimPrefix(srv.URL, "http://"), User: "u", Pass: "p",
		HTTPPostMode: true, DisableTLS: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Shutdown)
	withDcrdClient(t, c)
	return &n
}

// The agent tools hand their context to these accessors so that an agent that
// hangs up mid-call does not leave the node RPCs running. A cancelled context
// must stop each accessor before its first request leaves.
func TestNodeAccessorsHonourTheCallersContext(t *testing.T) {
	calls := dcrdRPCServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	accessors := []struct {
		name string
		call func(context.Context) error
	}{
		{"FetchNodeStatus", func(ctx context.Context) error { _, err := FetchNodeStatus(ctx); return err }},
		{"FetchBlockchainInfo", func(ctx context.Context) error { _, err := FetchBlockchainInfo(ctx); return err }},
		{"FetchNetworkInfo", func(ctx context.Context) error { _, err := FetchNetworkInfo(ctx); return err }},
		{"FetchPeers", func(ctx context.Context) error { _, err := FetchPeers(ctx); return err }},
		{"FetchSupplyInfo", func(ctx context.Context) error { _, err := FetchSupplyInfo(ctx); return err }},
		{"FetchStakingInfo", func(ctx context.Context) error { _, err := FetchStakingInfo(ctx); return err }},
		{"FetchMempoolInfo", func(ctx context.Context) error { _, err := FetchMempoolInfo(ctx); return err }},
	}
	for _, a := range accessors {
		t.Run(a.name, func(t *testing.T) {
			// Some sections report a node failure as an empty section by
			// design, so the error is not asserted; the wire is.
			calls.Store(0)
			start := time.Now()
			_ = a.call(ctx)
			if took := time.Since(start); took > time.Second {
				t.Fatalf("took %v on a cancelled context", took)
			}
			if got := calls.Load(); got != 0 {
				t.Fatalf("%d request(s) reached dcrd on a context that was already cancelled", got)
			}
		})
	}
}
