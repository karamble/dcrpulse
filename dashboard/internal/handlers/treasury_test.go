// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
	"github.com/decred/dcrd/rpcclient/v8"
)

func TestTreasuryScanHTTPRejectsInvalidHeight(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req struct {
			ID     json.RawMessage
			Method string
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method != "getblockcount" {
			t.Errorf("unexpected block work: %s", req.Method)
		}
		// Below activation ensures even default/clamped requests reject, without launching work.
		json.NewEncoder(w).Encode(map[string]any{"id": req.ID, "result": services.TreasuryActivationHeight - 1, "error": nil})
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
	for _, tc := range []struct {
		body string
		rpc  bool
	}{
		{`{"startHeight":9223372036854775807}`, true}, {`{"startHeight":552448}`, true},
		{``, true}, {`{}`, true}, {`{"startHeight":0}`, true}, {`{"startHeight":-1}`, true},
		{`{"startHeight":9223372036854775808}`, false}, {`{"startHeight":"552448"}`, false},
		{`{"startHeight":1.5}`, false}, {`{`, false}, {`[]`, false}, {`{} {}`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			before := calls.Load()
			w := httptest.NewRecorder()
			TriggerTSpendScanHandler(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body)))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			gotRPC := calls.Load() > before
			if gotRPC != tc.rpc {
				t.Fatalf("RPC=%v want %v", gotRPC, tc.rpc)
			}
			if tc.rpc && !strings.Contains(w.Body.String(), "exceeds current chain tip") {
				t.Fatalf("lost validation reason: %s", w.Body.String())
			}
		})
	}
	p, _ := services.GetScanProgress()
	if p.IsScanning {
		t.Fatal("invalid HTTP request launched scan")
	}
}
