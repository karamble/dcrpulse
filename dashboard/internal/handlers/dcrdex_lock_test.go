// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// These six reach bisonw over the RPC client, which survives a lock, so nothing
// but an explicit check stops them. The test process has no session, so
// DcrdexUnlocked is false: with the check each route answers 409, without it the
// call carries on and fails later, on the RPC client instead.
func TestDexWriteRoutesRefuseWhileLocked(t *testing.T) {
	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		method  string
		path    string
		body    map[string]any
	}{
		{"cancel", CancelDcrdexOrderHandler, http.MethodPost, "/api/dcrdex/cancel",
			map[string]any{"orderID": "ab12"}},
		{"wallet close", CloseDcrdexWalletHandler, http.MethodPost, "/api/dcrdex/wallet/close",
			map[string]any{"assetID": 42}},
		{"wallet toggle", ToggleDcrdexWalletHandler, http.MethodPost, "/api/dcrdex/wallet/toggle",
			map[string]any{"assetID": 42, "disable": true}},
		{"wallet rescan", RescanDcrdexWalletHandler, http.MethodPost, "/api/dcrdex/wallet/rescan",
			map[string]any{"assetID": 42}},
		{"add peer", AddDcrdexWalletPeerHandler, http.MethodPost, "/api/dcrdex/wallet/peers",
			map[string]any{"assetID": 42, "address": "1.2.3.4:9108"}},
		{"remove peer", RemoveDcrdexWalletPeerHandler, http.MethodDelete, "/api/dcrdex/wallet/peers",
			map[string]any{"assetID": 42, "address": "1.2.3.4:9108"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("Marshal() = %v", err)
			}
			w := httptest.NewRecorder()
			tt.handler(w, httptest.NewRequest(tt.method, tt.path, bytes.NewReader(body)))
			if w.Code != http.StatusConflict {
				t.Fatalf("status = %d %q, want 409: the route reached bisonw through the lock",
					w.Code, strings.TrimSpace(w.Body.String()))
			}
			if got := strings.TrimSpace(w.Body.String()); got != "DCRDEX is locked" {
				t.Errorf("body = %q, want the lock message", got)
			}
		})
	}
}
