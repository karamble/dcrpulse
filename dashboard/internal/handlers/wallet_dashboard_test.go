// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/rpcclient/v8"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// disconnectedRPC is a *rpcclient.Client whose every call fails at once: HTTP
// post mode to a port nothing listens on, so New does no I/O.
func disconnectedRPC(t *testing.T) *rpcclient.Client {
	t.Helper()
	c, err := rpcclient.New(&rpcclient.ConnConfig{
		Host: "127.0.0.1:1", User: "u", Pass: "p",
		HTTPPostMode: true, DisableTLS: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Shutdown)
	return c
}

func withRPCClients(t *testing.T, dcrd, wallet *rpcclient.Client) {
	t.Helper()
	prevD, prevW := rpc.DcrdClient, rpc.WalletClient
	rpc.DcrdClient, rpc.WalletClient = dcrd, wallet
	t.Cleanup(func() { rpc.DcrdClient, rpc.WalletClient = prevD, prevW })
}

// A fetch that outlives the request's deadline comes back with zeroed
// balances and no error, because each inner call treats "gave up" as "found
// nothing". Answering 200 would replace the figures on screen with 0.00; the
// page keeps its last good data only on a 408, which is what the frontend
// branches on.
func TestDashboardTimeoutIsA408NotZeroBalances(t *testing.T) {
	withRPCClients(t, nil, disconnectedRPC(t))
	prev := fetchWalletDashboard
	fetchWalletDashboard = func(ctx context.Context) (*types.WalletDashboardData, error) {
		<-ctx.Done()
		return &types.WalletDashboardData{Accounts: []types.AccountInfo{}}, nil
	}
	t.Cleanup(func() { fetchWalletDashboard = prev })

	// The handler's 20s derives from the request context, so a shorter one
	// here is the one that fires.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/api/wallet/dashboard", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	GetWalletDashboardHandler(w, r)

	if w.Code != http.StatusRequestTimeout {
		t.Fatalf("status = %d, want 408; body: %s", w.Code, w.Body.String())
	}
}

func TestDcrdReadyForWalletRefusesDuringIBD(t *testing.T) {
	withRPCClients(t, disconnectedRPC(t), disconnectedRPC(t))
	t.Cleanup(services.SetNodeSyncSnapshotForTest(services.NodeSyncSnapshot{Status: "syncing"}))

	w := httptest.NewRecorder()
	GetWalletStatusHandler(w, httptest.NewRequest(http.MethodGet, "/api/wallet/status", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 during initial block download", w.Code)
	}
	if !strings.Contains(w.Body.String(), "still downloading the blockchain") {
		t.Fatalf("body = %q, want the IBD message", w.Body.String())
	}
}

func TestDcrdReadyForWalletReportsTheStartupNote(t *testing.T) {
	withRPCClients(t, disconnectedRPC(t), disconnectedRPC(t))
	t.Cleanup(services.SetNodeSyncSnapshotForTest(services.NodeSyncSnapshot{
		Status: "upgrading", StartupNote: "dcrd is upgrading its database",
	}))

	w := httptest.NewRecorder()
	GetWalletStatusHandler(w, httptest.NewRequest(http.MethodGet, "/api/wallet/status", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "upgrading its database") {
		t.Fatalf("status = %d body = %q, want 503 with the poller's note", w.Code, w.Body.String())
	}
}

// Before the poller's first refresh returns the snapshot is empty even with a
// dcrd client present (the no-credentials case, where it stays empty forever,
// is pinned in services). The wallet routes must answer through that window.
func TestWalletRoutesStillAnswerWhenTheSnapshotIsUnseeded(t *testing.T) {
	withRPCClients(t, disconnectedRPC(t), disconnectedRPC(t))
	t.Cleanup(services.SetNodeSyncSnapshotForTest(services.NodeSyncSnapshot{}))

	w := httptest.NewRecorder()
	GetWalletStatusHandler(w, httptest.NewRequest(http.MethodGet, "/api/wallet/status", nil))
	if w.Code == http.StatusServiceUnavailable {
		t.Fatalf("an unseeded snapshot blocked the wallet route: %s", w.Body.String())
	}
}
