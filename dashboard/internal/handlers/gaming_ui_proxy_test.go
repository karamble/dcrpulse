// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/mux"
)

// What reaches the game, and what comes back.
//
// The stand-in below is what a game would see. Everything asserted here is
// about the boundary rather than about poker: a cookie that must not travel, a
// token that must be swapped, a Set-Cookie that must not survive, and a policy
// the game does not get to choose.

type standIn struct {
	mu   sync.Mutex
	got  []*http.Request
	srv  *httptest.Server
	body string
	// setCookie and policy are things a hostile or careless game might try
	// to send back.
	setCookie string
	policy    string
}

func newStandIn(t *testing.T) *standIn {
	t.Helper()
	g := &standIn{body: "<!doctype html><div id=root></div><script>window.x=1</script>"}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clone := r.Clone(r.Context())
		g.mu.Lock()
		g.got = append(g.got, clone)
		g.mu.Unlock()

		if g.setCookie != "" {
			w.Header().Set("Set-Cookie", g.setCookie)
		}
		if g.policy != "" {
			w.Header().Set("Content-Security-Policy", g.policy)
			w.Header().Set("X-Frame-Options", "DENY")
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, g.body)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *standIn) last() *http.Request {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.got) == 0 {
		return nil
	}
	return g.got[len(g.got)-1]
}

// proxyTo builds the document handler against a stand-in, bypassing the
// portal's state file - the resolution of which game runs where is tested
// separately and is not what this file is about.
func proxyTo(g *standIn) http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/gameui/{game}/", func(w http.ResponseWriter, req *http.Request) {
		proxyDocumentTo(w, req, g.srv.URL, "the-game-token")
	}).Methods("GET")
	return r
}

// A dashboard session must not reach the sandbox.
//
// This is the one that is silent when it goes wrong. A ReverseProxy forwards
// Cookie verbatim by default, so the game would receive a live dashboard
// session and could call every /api route as the user - which would make the
// isolated network, the container and the token all decorative.
func TestTheDashboardSessionDoesNotReachTheGame(t *testing.T) {
	g := newStandIn(t)

	req := httptest.NewRequest(http.MethodGet, "/gameui/poker/", nil)
	req.AddCookie(&http.Cookie{Name: "dcrpulse_session", Value: "a-real-session"})
	rec := httptest.NewRecorder()
	proxyTo(g).ServeHTTP(rec, req)

	got := g.last()
	if got == nil {
		t.Fatal("nothing reached the game")
	}
	if c := got.Header.Get("Cookie"); c != "" {
		t.Fatalf("the game received Cookie: %q", c)
	}
	if _, err := got.Cookie("dcrpulse_session"); err == nil {
		t.Fatal("the game received the dashboard's session cookie")
	}
}

// The game is spoken to as the game, never as the panel.
func TestTheGameIsSpokenToWithItsOwnToken(t *testing.T) {
	g := newStandIn(t)

	req := httptest.NewRequest(http.MethodGet, "/gameui/poker/", nil)
	req.Header.Set("Authorization", "Bearer a-panel-token")
	rec := httptest.NewRecorder()
	proxyTo(g).ServeHTTP(rec, req)

	got := g.last()
	if got == nil {
		t.Fatal("nothing reached the game")
	}
	if auth := got.Header.Get("Authorization"); auth != "Bearer the-game-token" {
		t.Fatalf("the game was sent %q", auth)
	}
	if strings.Contains(got.Header.Get("Authorization"), "a-panel-token") {
		t.Fatal("the panel's own token was forwarded")
	}
}

// A game does not get to set cookies on this dashboard's origin, or to say
// whether it may be framed.
func TestTheGameCannotSetCookiesOrItsOwnPolicy(t *testing.T) {
	g := newStandIn(t)
	g.setCookie = "dcrpulse_session=forged; Path=/"
	g.policy = "default-src *"

	req := httptest.NewRequest(http.MethodGet, "/gameui/poker/", nil)
	rec := httptest.NewRecorder()
	proxyTo(g).ServeHTTP(rec, req)

	if c := rec.Header().Get("Set-Cookie"); c != "" {
		t.Fatalf("a cookie the game chose reached the browser: %q", c)
	}
	if p := rec.Header().Get("Content-Security-Policy"); strings.Contains(p, "default-src *") {
		t.Fatalf("the game chose its own policy: %q", p)
	}
	if x := rec.Header().Get("X-Frame-Options"); x != "" {
		t.Fatalf("X-Frame-Options survived as %q, which would stop the page being framed", x)
	}
}

// The policy a framed page gets must name an origin, because 'self' matches
// nothing in an opaque one - a page carrying a copied policy could fetch
// nothing and could not be framed, and the failure would look like this proxy
// being broken.
func TestTheFramePolicyNamesAnOriginAndNotSelf(t *testing.T) {
	g := newStandIn(t)

	req := httptest.NewRequest(http.MethodGet, "/gameui/poker/", nil)
	req.Host = "pulse.example:8080"
	rec := httptest.NewRecorder()
	proxyTo(g).ServeHTTP(rec, req)

	policy := rec.Header().Get("Content-Security-Policy")
	if policy == "" {
		t.Fatal("a framed page was served with no policy at all")
	}
	if strings.Contains(policy, "'self'") {
		t.Fatalf("the policy uses 'self', which matches nothing here: %q", policy)
	}
	for _, want := range []string{
		"default-src 'none'",
		"connect-src http://pulse.example:8080",
		"frame-ancestors http://pulse.example:8080",
		"sandbox allow-scripts",
	} {
		if !strings.Contains(policy, want) {
			t.Errorf("the policy is missing %q: %q", want, policy)
		}
	}
	// The inline script the bundle is made of has to be allowed, by hash
	// rather than by 'unsafe-inline'.
	if !strings.Contains(policy, "script-src 'sha256-") {
		t.Errorf("the page's own script is not allowed by hash: %q", policy)
	}
	if strings.Contains(policy, "unsafe-inline") && !strings.Contains(policy, "style-src 'unsafe-inline'") {
		t.Errorf("scripts are allowed inline without a hash: %q", policy)
	}
}

// A preflight is answered without a token, because a preflight is what asks
// permission to send one - and identically whatever it names, so it cannot be
// used to find out which games are installed.
func TestAPreflightNeedsNoTokenAndTellsNothing(t *testing.T) {
	r := mux.NewRouter()
	r.HandleFunc("/gameui/{game}/api/{rest:.*}", GameUIPreflightHandler).Methods("OPTIONS")

	answers := map[string]string{}
	for _, path := range []string{
		"/gameui/poker/api/tables",
		"/gameui/chess/api/tables",
		"/gameui/poker/api/identity/backup",
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, path, nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("preflight for %s answered %d", path, rec.Code)
		}
		answers[path] = fmt.Sprint(rec.Code, rec.Header())
	}

	var first string
	for path, answer := range answers {
		if first == "" {
			first = answer
			continue
		}
		if answer != first {
			t.Fatalf("preflights differ, so they can be used to probe: %s", path)
		}
	}
}

// The credentials pair that would undo the whole arrangement.
//
// A browser refuses Allow-Origin '*' together with Allow-Credentials, but
// accepts 'null' with them - and any page anywhere can produce a null origin by
// opening a sandboxed frame of its own. Using '*' makes that mistake
// unrepresentable, so the absence of the credentials header is the property to
// pin.
func TestCredentialsAreNeverAllowedOnTheProxy(t *testing.T) {
	r := mux.NewRouter()
	r.HandleFunc("/gameui/{game}/api/{rest:.*}", GameUIPreflightHandler).Methods("OPTIONS")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/gameui/poker/api/tables", nil))

	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("credentials are allowed (%q), which makes a null origin enough", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin is %q; it must be * so credentials can never be added", got)
	}
}

// A repeated Allow-Origin fails CORS in every browser, and the proxy sets one
// on the writer while the upstream response carries another into it.
func TestAllowOriginIsSentOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	gamingUICORS(rec)
	// What ReverseProxy does with the upstream headers after ModifyResponse.
	upstream := http.Header{}
	upstream.Set("Access-Control-Allow-Origin", "*")
	upstream.Del("Access-Control-Allow-Origin")
	for k, vs := range upstream {
		for _, v := range vs {
			rec.Header().Add(k, v)
		}
	}
	if got := rec.Header().Values("Access-Control-Allow-Origin"); len(got) != 1 {
		t.Fatalf("Allow-Origin sent %d times: %v", len(got), got)
	}
}
