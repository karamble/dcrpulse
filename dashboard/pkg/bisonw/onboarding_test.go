// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestRPCClient serves the positional-RPC surface and hands each request's
// decoded params to record.
func newTestRPCClient(t *testing.T, record func(rawParams)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg wireMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("decode request: %v", err)
		}
		var p rawParams
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			t.Errorf("decode params: %v", err)
		}
		record(p)
		payload, _ := json.Marshal(responsePayload{Result: json.RawMessage(`{}`)})
		_ = json.NewEncoder(w).Encode(wireMessage{Type: msgTypeResponse, ID: msg.ID, Payload: payload})
	}))
	t.Cleanup(srv.Close)
	return &Client{url: srv.URL, http: srv.Client()}
}

// Asset id 0 is Bitcoin's real BIP-44 id, not "unset"; PostBond must send it
// verbatim instead of rewriting it to DCR (callers default absent ids to DCR).
func TestPostBondSendsAssetIDVerbatim(t *testing.T) {
	for _, tc := range []struct {
		assetID uint32
		want    string
	}{
		{0, "0"},
		{AssetDCR, "42"},
	} {
		var got rawParams
		c := newTestRPCClient(t, func(p rawParams) { got = p })
		_, err := c.PostBond(context.Background(), PostBondParams{
			AppPass: "pw", Host: "dex.example.org:7232", Bond: 100e8, AssetID: tc.assetID,
		})
		if err != nil {
			t.Fatalf("PostBond(asset %d): %v", tc.assetID, err)
		}
		wantArgs := []string{"dex.example.org:7232", "10000000000", tc.want}
		if len(got.Args) != len(wantArgs) {
			t.Fatalf("asset %d: args %v, want %v", tc.assetID, got.Args, wantArgs)
		}
		for i := range wantArgs {
			if got.Args[i] != wantArgs[i] {
				t.Errorf("asset %d: arg[%d] = %q, want %q", tc.assetID, i, got.Args[i], wantArgs[i])
			}
		}
	}
}
