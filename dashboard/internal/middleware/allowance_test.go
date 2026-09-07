// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// An Allowance is one bucket seen from two sides: the token an MCP tool spends
// through Limiter is the token the browser route no longer has.
func TestAllowanceIsOneBucketForRouteAndTool(t *testing.T) {
	a := Allowance{"test-shared-allowance", time.Hour, 1}
	route := a.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if !a.Limiter().Allow() {
		t.Fatal("a fresh bucket refused its first token")
	}
	w := httptest.NewRecorder()
	route.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/job", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("route answered %d after the tool spent the shared token, want 429", w.Code)
	}
}
