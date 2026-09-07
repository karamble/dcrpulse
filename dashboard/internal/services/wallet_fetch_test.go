// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/decred/dcrd/rpcclient/v8"
)

// FetchWalletStatus folds every RPC failure into a "no_wallet" status rather
// than an error, so an assertion on its result proves nothing about whether an
// RPC was attempted: a wrong port and a cancelled context look the same. These
// tests watch the wire instead - a JSON-RPC server that counts what arrives.

// walletRPCServer installs rpc.WalletClient pointed at a JSON-RPC server whose
// every request runs reply, and returns the request counter.
func walletRPCServer(t *testing.T, reply func(w http.ResponseWriter, id json.RawMessage)) *atomic.Int32 {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		reply(w, req.ID)
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
	withWalletClient(t, c)
	return &n
}

func rpcError(w http.ResponseWriter, id json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"result":null,"error":{"code":-1,"message":"test"},"id":%s}`, id)
}

// A caller that has already given up must not cost the wallet an RPC.
func TestFetchWalletStatusHonoursTheCallersContext(t *testing.T) {
	withDcrdClient(t, nil)
	calls := walletRPCServer(t, rpcError)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := FetchWalletStatus(ctx); err != nil {
		t.Fatalf("FetchWalletStatus() = %v; it folds failures into the status, so this should be nil", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("a cancelled context still took %v", took)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("%d RPC(s) reached the wallet on a context that was already cancelled", got)
	}
}

// Nothing above FetchWalletStatus carries a deadline, so it has to keep one of
// its own even now that it honours the caller's context.
func TestFetchWalletStatusKeepsItsOwnCeiling(t *testing.T) {
	withDcrdClient(t, nil)
	release := make(chan struct{})
	walletRPCServer(t, func(w http.ResponseWriter, id json.RawMessage) {
		<-release
		rpcError(w, id)
	})
	// Registered after the server, so it runs before srv.Close (cleanups are
	// LIFO); otherwise Close waits forever on the handler parked above.
	t.Cleanup(func() { close(release) })

	prev := walletStatusTimeout
	walletStatusTimeout = 50 * time.Millisecond
	t.Cleanup(func() { walletStatusTimeout = prev })

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = FetchWalletStatus(context.Background()) // no deadline of its own
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("FetchWalletStatus ran past its ceiling on a context with no deadline")
	}
}

// A dashboard fetch cut short by its deadline still returns a struct - with
// zeroed balances and an empty account list - and every inner fetch reports
// nil, because they treat "gave up" as "found nothing". The caller must be told
// the difference, or those zeros get rendered as the wallet's balance.
func TestDashboardDataReportsACutShortFetch(t *testing.T) {
	withDcrdClient(t, nil)
	walletRPCServer(t, rpcError)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data, err := FetchWalletDashboardData(ctx)
	if data == nil {
		t.Fatal("no data returned at all; the partial struct is still expected")
	}
	if err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled so the caller can refuse to show zeros", err)
	}
	if data.Accounts == nil {
		t.Fatal("Accounts is nil; the JSON would say null instead of []")
	}
}
