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

// bisonw's reconfigurewallet handler unmarshals newWalletPW into a byte slice
// and a null value fails outright, so the key has to be absent rather than null
// when no password change is intended. These assert on the raw JSON, since
// unmarshalling into a struct cannot tell an absent key from a null one.
func newTestWebClient(t *testing.T, h http.HandlerFunc) *WebClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := &WebClient{baseURL: srv.URL, http: srv.Client()}
	c.loggedIn = true // skip the login round trip; not what these test
	return c
}

func captureReconfigureBody(t *testing.T, newWalletPW string) map[string]json.RawMessage {
	t.Helper()
	var got map[string]json.RawMessage
	c := newTestWebClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	cfg := map[string]string{"username": "dcrwallet", "rpclisten": "127.0.0.1:9110"}
	err := c.ReconfigureWallet(context.Background(), "apppass", AssetDCR, WalletTypeDcrwalletRPC, cfg, newWalletPW)
	if err != nil {
		t.Fatalf("ReconfigureWallet: %v", err)
	}
	return got
}

func TestReconfigureWalletOmitsEmptyPassword(t *testing.T) {
	got := captureReconfigureBody(t, "")

	if raw, ok := got["newWalletPW"]; ok {
		t.Errorf("newWalletPW was sent as %s; the key must be absent, not null", raw)
	}

	// The rest of the request must be unaffected.
	for _, k := range []string{"assetID", "walletType", "config", "appPW"} {
		if _, ok := got[k]; !ok {
			t.Errorf("%q missing from the request body", k)
		}
	}
}

func TestReconfigureWalletSendsPassword(t *testing.T) {
	got := captureReconfigureBody(t, "s3cret")

	raw, ok := got["newWalletPW"]
	if !ok {
		t.Fatal("newWalletPW missing when a password was supplied")
	}
	var pw string
	if err := json.Unmarshal(raw, &pw); err != nil {
		t.Fatalf("newWalletPW is not a JSON string: %s", raw)
	}
	if pw != "s3cret" {
		t.Errorf("newWalletPW = %q, want %q", pw, "s3cret")
	}
}
