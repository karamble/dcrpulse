// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegistryVerify(t *testing.T) {
	r := newRegistry()
	r.addToken("a1", "agent-one", "secret-token-123")
	if _, ok := r.verify("secret-token-123"); !ok {
		t.Fatal("valid token rejected")
	}
	if _, ok := r.verify("wrong"); ok {
		t.Fatal("invalid token accepted")
	}
	if _, ok := r.verify(""); ok {
		t.Fatal("empty token accepted")
	}
}

func TestAuthMiddleware(t *testing.T) {
	r := newRegistry()
	r.addToken("a1", "agent-one", "tok")
	var reached bool
	h := r.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	// Missing token -> 401, handler not reached.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without token, got %d", rec.Code)
	}
	if reached {
		t.Fatal("handler reached without auth")
	}

	// Valid token -> passes and records a live session.
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !reached {
		t.Fatalf("valid token did not pass: code=%d reached=%v", rec.Code, reached)
	}
	if got := len(r.activeSessions(time.Now())); got != 1 {
		t.Fatalf("want 1 active session, got %d", got)
	}
}
