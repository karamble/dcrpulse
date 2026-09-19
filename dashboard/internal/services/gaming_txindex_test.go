// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"dcrpulse/internal/rpc"

	"github.com/decred/dcrd/rpcclient/v8"
)

// fakeDcrd answers getinfo with whatever the caller currently wants, so a test
// can change the node's mind between calls the way restarting dcrd does.
func fakeDcrd(t *testing.T, txIndex *atomic.Bool) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method != "getinfo" {
			http.Error(w, "unexpected method "+req.Method, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "1.0",
			"id":      req.ID,
			"result":  map[string]any{"txindex": txIndex.Load()},
			"error":   nil,
		})
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	client, err := rpcclient.New(&rpcclient.ConnConfig{
		Host: u.Host, User: "u", Pass: "p",
		HTTPPostMode: true, DisableTLS: true, DisableConnectOnNew: true,
	}, nil)
	if err != nil {
		t.Fatalf("test rpc client: %v", err)
	}
	prev := rpc.DcrdClient
	rpc.DcrdClient = client
	t.Cleanup(func() { rpc.DcrdClient = prev; client.Shutdown() })
}

// The operator's fix is to set txindex=1 and restart dcrd, and dcrpulse keeps
// running across that. A cached answer would go on refusing after the work was
// already done, which is a worse failure than the one the gate prevents - and
// an invisible one, since nothing about a stale "no" looks wrong.
func TestTheIndexIsReadFromTheNodeEveryTime(t *testing.T) {
	var txIndex atomic.Bool
	fakeDcrd(t, &txIndex)
	ctx := context.Background()

	if TxIndexActive(ctx) {
		t.Fatal("a node with no index read as having one")
	}

	// dcrd restarted with txindex=1. dcrpulse did not.
	txIndex.Store(true)
	if !TxIndexActive(ctx) {
		t.Fatal("the index stayed cached off after the node was fixed, so the bridge would never switch on")
	}

	// And back, so an index that goes away is noticed too.
	txIndex.Store(false)
	if TxIndexActive(ctx) {
		t.Fatal("the index stayed cached on after the node lost it")
	}
}

// A dcrd that cannot be asked is not a dcrd that answered yes. Refusing is
// recoverable and says what to do; enabling on a guess parks the first payout
// at publishing with nothing to read.
func TestAnUnreachableNodeReadsAsNoIndex(t *testing.T) {
	prev := rpc.DcrdClient
	rpc.DcrdClient = nil
	t.Cleanup(func() { rpc.DcrdClient = prev })

	if _, err := DcrdHasTxIndex(context.Background()); err == nil {
		t.Fatal("an absent dcrd answered without an error")
	}
	if TxIndexActive(context.Background()) {
		t.Fatal("an absent dcrd was taken as having the index")
	}
}
