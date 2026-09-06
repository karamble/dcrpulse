// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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

func TestParseAllowedIPs(t *testing.T) {
	valid := []struct {
		in   []string
		want []string
	}{
		{[]string{"192.0.2.1"}, []string{"192.0.2.1"}},
		{[]string{"::ffff:192.0.2.1"}, []string{"192.0.2.1"}},
		{[]string{"fe80::1%eth0"}, []string{"fe80::1"}},
		{[]string{"10.0.0.7/8"}, []string{"10.0.0.0/8"}},
		{[]string{"2001:db8::/32"}, []string{"2001:db8::/32"}},
		{[]string{"::ffff:192.0.2.0/120"}, []string{"192.0.2.0/24"}},
		{[]string{" ", "", " 192.0.2.1 "}, []string{"192.0.2.1"}},
		{nil, []string{}},
	}
	for _, tc := range valid {
		got, err := ParseAllowedIPs(tc.in)
		if err != nil {
			t.Fatalf("ParseAllowedIPs(%v): %v", tc.in, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("ParseAllowedIPs(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
	for _, bad := range []string{"banana", "300.1.2.3", "10.0.0.0/33", "::ffff:1.2.3.0/90"} {
		_, err := ParseAllowedIPs([]string{bad})
		if err == nil || !strings.Contains(err.Error(), bad) {
			t.Fatalf("ParseAllowedIPs(%q): want error naming the entry, got %v", bad, err)
		}
	}
}

func TestAuthMiddlewareAllowedIPs(t *testing.T) {
	newHandler := func(r *registry, reached *bool) http.Handler {
		return r.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			*reached = true
			w.WriteHeader(http.StatusOK)
		}))
	}
	send := func(h http.Handler, token, remote string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if remote != "" {
			req.RemoteAddr = remote
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// An empty list means no restriction.
	r := newRegistry()
	r.addToken("a1", "agent", "tok")
	var reached bool
	h := newHandler(r, &reached)
	if rec := send(h, "tok", ""); rec.Code != http.StatusOK {
		t.Fatalf("empty allowlist: want 200, got %d", rec.Code)
	}

	// A matching single IP passes and records the session's remote.
	if !r.setAllowedIPs("a1", []string{"192.0.2.1"}) {
		t.Fatal("setAllowedIPs on existing agent returned false")
	}
	if rec := send(h, "tok", "192.0.2.1:1234"); rec.Code != http.StatusOK {
		t.Fatalf("allowed IP: want 200, got %d", rec.Code)
	}
	if s := r.activeSessions(time.Now()); len(s) != 1 || s[0].Remote != "192.0.2.1:1234" {
		t.Fatalf("session remote not recorded: %+v", s)
	}

	// A non-matching IP is answered byte-identically to a bad token and leaves
	// no trace: no session, inner handler unreached.
	r2 := newRegistry()
	r2.addToken("a1", "agent", "tok")
	r2.setAllowedIPs("a1", []string{"10.0.0.1"})
	reached = false
	h2 := newHandler(r2, &reached)
	badTok := send(h2, "nope", "")
	wrongIP := send(h2, "tok", "")
	if wrongIP.Code != http.StatusUnauthorized {
		t.Fatalf("wrong IP: want 401, got %d", wrongIP.Code)
	}
	if wrongIP.Code != badTok.Code || wrongIP.Body.String() != badTok.Body.String() ||
		!reflect.DeepEqual(wrongIP.Header(), badTok.Header()) {
		t.Fatalf("wrong-IP response differs from bad-token response:\n%d %q %v\n%d %q %v",
			wrongIP.Code, wrongIP.Body.String(), wrongIP.Header(),
			badTok.Code, badTok.Body.String(), badTok.Header())
	}
	if reached {
		t.Fatal("handler reached from non-allowed IP")
	}
	if got := len(r2.activeSessions(time.Now())); got != 0 {
		t.Fatalf("denied request must not record a session, got %d", got)
	}

	// CIDR ranges and v4-mapped remotes match.
	r2.setAllowedIPs("a1", []string{"192.0.2.0/24"})
	if rec := send(h2, "tok", "192.0.2.77:9"); rec.Code != http.StatusOK {
		t.Fatalf("CIDR match: want 200, got %d", rec.Code)
	}
	r2.setAllowedIPs("a1", []string{"192.0.2.1"})
	if rec := send(h2, "tok", "[::ffff:192.0.2.1]:5555"); rec.Code != http.StatusOK {
		t.Fatalf("v4-mapped remote: want 200, got %d", rec.Code)
	}

	// A restricted agent fails closed on an unparseable remote; an
	// unrestricted one never parses it at all.
	if rec := send(h2, "tok", "garbage"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("malformed remote with allowlist: want 401, got %d", rec.Code)
	}
	r2.setAllowedIPs("a1", nil)
	if rec := send(h2, "tok", "garbage"); rec.Code != http.StatusOK {
		t.Fatalf("malformed remote without allowlist: want 200, got %d", rec.Code)
	}

	// The IP check runs before the blocked check, so a wrong-IP caller can
	// never learn that a valid token is merely blocked.
	r2.block("a1")
	r2.setAllowedIPs("a1", []string{"10.0.0.1"})
	if rec := send(h2, "tok", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("blocked + wrong IP: want 401, got %d", rec.Code)
	}
	r2.setAllowedIPs("a1", []string{"192.0.2.1"})
	if rec := send(h2, "tok", "192.0.2.1:1"); rec.Code != http.StatusForbidden {
		t.Fatalf("blocked + allowed IP: want 403, got %d", rec.Code)
	}
}

func TestAllowedIPDenialRecordedAndCleared(t *testing.T) {
	r := newRegistry()
	r.addToken("a1", "agent", "tok")
	r.setAllowedIPs("a1", []string{"10.0.0.1"})
	h := r.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	send := func(remote string) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer tok")
		if remote != "" {
			req.RemoteAddr = remote
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	// A denial surfaces the bare canonical IP (port stripped, v4-mapped
	// unmapped) so the operator can allow exactly that string.
	send("[::ffff:192.0.2.9]:4444")
	list := r.list()
	if len(list) != 1 || list[0].LastDenied == nil || list[0].LastDenied.IP != "192.0.2.9" {
		t.Fatalf("denial not recorded as bare IP: %+v", list)
	}
	if list[0].LastDenied.At.IsZero() {
		t.Fatal("denial missing timestamp")
	}

	// The next successful request clears the notice.
	r.setAllowedIPs("a1", []string{"192.0.2.9"})
	send("192.0.2.9:4444")
	if got := r.list()[0].LastDenied; got != nil {
		t.Fatalf("denial not cleared by a successful request: %+v", got)
	}
}

func TestAllowedIPDenialAlwaysLogged(t *testing.T) {
	sb := captureMCPLog(t)
	logGate.Store(false)
	r := newRegistry()
	r.addToken("a1", "loggy", "tok")
	r.setAllowedIPs("a1", []string{"10.9.9.9"})
	h := r.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	out := sb.String()
	if !strings.Contains(out, "loggy") || !strings.Contains(out, "192.0.2.1:1234") {
		t.Fatalf("denial not logged with agent and remote: %q", out)
	}
}
