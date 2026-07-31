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

func TestBlockedAgentRejectedByAuth(t *testing.T) {
	r := newRegistry()
	r.addToken("a1", "agent", "tok")
	h := r.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	send := func() int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer tok")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := send(); code != http.StatusOK {
		t.Fatalf("pre-block: want 200, got %d", code)
	}
	r.block("a1")
	if code := send(); code != http.StatusForbidden {
		t.Fatalf("blocked token: want 403, got %d", code)
	}
	if got := len(r.activeSessions(time.Now())); got != 0 {
		t.Fatalf("blocked agent should have no live session, got %d", got)
	}
	if !r.unblock("a1") {
		t.Fatal("unblock of existing agent should return true")
	}
	if code := send(); code != http.StatusOK {
		t.Fatalf("after unblock: want 200, got %d", code)
	}
}

func TestCreateListRevokeAgent(t *testing.T) {
	r := newRegistry()
	id, token, err := r.create("trading-bot")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// The freshly minted token must authenticate to the new identity.
	a, ok := r.verify(token)
	if !ok || a.id != id {
		t.Fatalf("created token did not verify to its agent")
	}
	// New agents start node-only.
	if !a.allows("node") || a.allows("wallet") {
		t.Fatalf("new agent should be node-only, got domains %v", sortedDomains(a.domainMap()))
	}
	list := r.list()
	if len(list) != 1 || list[0].ID != id || list[0].Name != "trading-bot" {
		t.Fatalf("list did not return the created agent: %+v", list)
	}
	if list[0].CreatedAt.IsZero() {
		t.Fatal("created agent missing CreatedAt")
	}
	// Revoke removes it; the token stops working and a second revoke is a no-op.
	if !r.remove(id) {
		t.Fatal("remove of existing agent returned false")
	}
	if _, ok := r.verify(token); ok {
		t.Fatal("revoked token still verifies")
	}
	if r.remove(id) {
		t.Fatal("remove of missing agent returned true")
	}
}

func TestSetDomainsAlwaysKeepsNode(t *testing.T) {
	r := newRegistry()
	id, token, err := r.create("bot")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !r.setDomains(id, []string{"wallet", "staking"}) {
		t.Fatal("setDomains on existing agent returned false")
	}
	a, _ := r.verify(token)
	if !a.allows("node") || !a.allows("wallet") || !a.allows("staking") {
		t.Fatalf("granted domains missing: %v", sortedDomains(a.domainMap()))
	}
	// Replacing the grant set always re-implies node.
	if !r.setDomains(id, []string{"wallet"}) || !a.allows("node") {
		t.Fatal("node domain must always remain implied")
	}
	if r.setDomains("nope", []string{"wallet"}) {
		t.Fatal("setDomains on missing agent returned true")
	}
}

func TestCatalogDomains(t *testing.T) {
	got := map[string]bool{}
	for _, d := range catalogDomains() {
		got[d] = true
	}
	for _, want := range []string{
		"node", "wallet", "staking", "governance", "treasury", "lightning",
		"privacy", "explorer", "timestamp", "tor", "dex", "bisonrelay",
	} {
		if !got[want] {
			t.Errorf("catalog domains missing %q (got %v)", want, catalogDomains())
		}
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
