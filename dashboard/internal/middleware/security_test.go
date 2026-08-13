// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostAllowedDefaults(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"", true},
		{"localhost", true},
		{"localhost:8080", true},
		{"app.localhost:8080", true},
		{"127.0.0.1", true},
		{"127.0.0.1:8080", true},
		{"192.168.1.50:8080", true},
		{"95.216.110.66:8080", true},
		{"::1", true},
		{"[::1]", true},
		{"[::1]:8080", true},
		{"dashboard:8080", true},
		{"dcrpulse-dashboard:8080", true},
		{"dcrpulse_dashboard_1:8735", true},
		{"umbrel.local", true},
		{"umbrel.local:8735", true},
		{"umbrel-dev.local:8735", true},
		{"casaos.local:8080", true},
		{"UMBREL.LOCAL:8735", true},
		{"umbrel.local.", true},
		{"pqrstuvwxyz234567.onion", true},
		{"evil.com", false},
		{"evil.com:8080", false},
		{"dcrpulse.example.com", false},
		{"dcrpulse.95.216.110.66.sslip.io", false},
		{"umbrel.local.evil.com", false},
		{"onion.evil.com", false},
	}
	for _, tt := range tests {
		if got := hostAllowed(tt.host); got != tt.want {
			t.Errorf("hostAllowed(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestHostAllowedFromEnv(t *testing.T) {
	t.Setenv("DASHBOARD_ALLOWED_HOSTS", "dcrpulse.example.com, dcrpulse.95.216.110.66.sslip.io")
	allowed := []string{
		"dcrpulse.example.com",
		"dcrpulse.example.com:8080",
		"DCRPULSE.EXAMPLE.COM",
		"dcrpulse.95.216.110.66.sslip.io",
	}
	for _, h := range allowed {
		if !hostAllowed(h) {
			t.Errorf("hostAllowed(%q) = false, want true", h)
		}
	}
	if hostAllowed("evil.com") {
		t.Error(`hostAllowed("evil.com") = true, want false`)
	}
}

func TestHostAllowedWildcard(t *testing.T) {
	t.Setenv("DASHBOARD_ALLOWED_HOSTS", "*")
	if !hostAllowed("evil.com") {
		t.Error(`hostAllowed("evil.com") = false, want true with the wildcard set`)
	}
}

// The cap used to be skipped whenever the caller said multipart, which let a
// JSON body arrive unbounded on any route just by setting a header. A caller
// picks its own Content-Type; it does not pick the route it reached, so the
// exemption keys on the path instead.
func TestLimitJSONBodyCapsWhateverTheCallerClaims(t *testing.T) {
	const cap, body = 100, 500
	const upload = "/api/br/files/send"
	multipart := "multipart/form-data; boundary=x"

	h := LimitJSONBody(cap, map[string]bool{upload: true})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n, err := io.Copy(io.Discard, r.Body)
			if err != nil {
				http.Error(w, "too large", http.StatusRequestEntityTooLarge)
				return
			}
			fmt.Fprintf(w, "%d", n)
		}))

	tests := []struct {
		name       string
		method     string
		path       string
		ctype      string
		wantCapped bool
	}{
		{"json on a session-less route", http.MethodPost, "/api/auth/login", "application/json", true},
		// The row this change exists for: the same body, one header different.
		{"the same body called multipart", http.MethodPost, "/api/auth/login", multipart, true},
		{"put", http.MethodPut, "/api/wallet/settings", multipart, true},
		{"patch", http.MethodPatch, "/api/wallet/settings", multipart, true},
		{"delete", http.MethodDelete, "/api/wallet/settings", multipart, true},
		// GET carries no body worth capping and the streaming routes are all GET.
		{"get", http.MethodGet, "/api/auth/status", "application/json", false},
		// The upload routes stream under their own larger cap, and that holds
		// whatever they claim to be carrying.
		{"an upload route", http.MethodPost, upload, multipart, false},
		{"an upload route called json", http.MethodPost, upload, "application/json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewReader(bytes.Repeat([]byte("x"), body)))
			req.Header.Set("Content-Type", tt.ctype)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if tt.wantCapped {
				if w.Code != http.StatusRequestEntityTooLarge {
					t.Fatalf("answered %d %q, want the body capped at %d", w.Code, strings.TrimSpace(w.Body.String()), cap)
				}
				return
			}
			if got := strings.TrimSpace(w.Body.String()); got != fmt.Sprint(body) {
				t.Fatalf("handler read %q of %d bytes, want the whole body uncapped", got, body)
			}
		})
	}
}

// headersFor runs SecurityHeaders over a trivial handler and returns what it set.
func headersFor(t *testing.T) http.Header {
	t.Helper()
	rec := httptest.NewRecorder()
	SecurityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	return rec.Header()
}

// Realtime calls capture the mic in our own document. An empty allowlist denies
// every origin including ours, so getUserMedia fails before a call can start.
func TestSecurityHeadersAllowOurOwnMicrophone(t *testing.T) {
	pp := headersFor(t).Get("Permissions-Policy")

	if !strings.Contains(pp, "microphone=(self)") {
		t.Errorf("Permissions-Policy = %q, want the microphone allowed to our own origin", pp)
	}
	for _, denied := range []string{"camera=()", "geolocation=()", "payment=()", "usb=()"} {
		if !strings.Contains(pp, denied) {
			t.Errorf("Permissions-Policy = %q, lost the %s denial", pp, denied)
		}
	}
}

// The mic-tap worklet is served from our own origin precisely so script-src can
// stay closed. Admitting blob: would let any injected string become a script.
func TestSecurityHeadersKeepScriptsToOurOwnOrigin(t *testing.T) {
	csp := headersFor(t).Get("Content-Security-Policy")

	var scriptSrc string
	for _, d := range strings.Split(csp, ";") {
		if d = strings.TrimSpace(d); strings.HasPrefix(d, "script-src") {
			scriptSrc = d
		}
	}
	if scriptSrc == "" {
		t.Fatalf("Content-Security-Policy = %q, has no script-src at all", csp)
	}
	if strings.Contains(scriptSrc, "blob:") {
		t.Errorf("%q admits blob: scripts; serve the worklet from 'self' instead", scriptSrc)
	}
	if strings.Contains(scriptSrc, "unsafe-inline") || strings.Contains(scriptSrc, "unsafe-eval") {
		t.Errorf("%q weakens script execution", scriptSrc)
	}
}

// Which scripts are inline, decided the way the browser decides it.
//
// This is not a parsing nicety. The hashes here go into a script-src, so a
// wrong answer either blocks the page it is serving or allows a body nobody
// intended - and one of those failures looks exactly like the proxy being
// broken.
func TestScriptsInsideAScriptAreText(t *testing.T) {
	// The shape that caused this to be rewritten: React's bundle contains
	// the literal string "<script><\/script>", which a pattern hunting for
	// opening tags read as a second script element that does not exist.
	html := []byte(`<!doctype html><html><head>` +
		`<script type="module" crossorigin>const a="<script><\/script>";run(a)</script>` +
		`</head><body></body></html>`)

	got := InlineScriptHashesFor(html)
	if len(got) != 1 {
		t.Fatalf("found %d scripts in a document with one: %v", len(got), got)
	}
	want := InlineScriptHash([]byte(`const a="<script><\/script>";run(a)`))
	if got[0] != want {
		t.Fatalf("hashed something other than the script's own body:\n got %s\n want %s", got[0], want)
	}
}

// A script that loads its content from elsewhere has an empty body, so there is
// nothing to hash and hashing the empty string would allow every empty script
// anywhere.
func TestAScriptWithASourceIsNotHashed(t *testing.T) {
	html := []byte(`<script type="module" src="/assets/app.js"></script>`)
	if got := InlineScriptHashesFor(html); len(got) != 0 {
		t.Fatalf("hashed a script that has a src: %v", got)
	}
}

// Several genuinely separate scripts are all hashed, in order, once each.
func TestEveryInlineScriptIsHashed(t *testing.T) {
	html := []byte(`<script>one()</script><script src="x.js"></script><script>two()</script>`)
	got := InlineScriptHashesFor(html)
	if len(got) != 2 {
		t.Fatalf("found %d scripts, want 2: %v", len(got), got)
	}
	if got[0] != InlineScriptHash([]byte("one()")) || got[1] != InlineScriptHash([]byte("two()")) {
		t.Fatalf("hashed the wrong bodies: %v", got)
	}
}

// A document that does not close a script is not one to write a policy for, and
// this must not loop or read past the end trying.
func TestAnUnterminatedScriptIsNotAScript(t *testing.T) {
	for _, html := range []string{
		`<script>never closed`,
		`<script`,
		`<script>a</script><script>never closed`,
	} {
		got := InlineScriptHashesFor([]byte(html))
		if len(got) > 1 {
			t.Errorf("%q produced %d hashes", html, len(got))
		}
	}
}

// Nothing is framed here any more, and a policy that still allowed it would be
// describing a proxy this dashboard no longer runs.
func TestTheDocumentPolicyFramesNothing(t *testing.T) {
	policy := buildCSP(nil)
	if strings.Contains(policy, "frame-src") {
		t.Errorf("the document policy still allows framing: %s", policy)
	}
	if !strings.Contains(policy, "frame-ancestors 'self'") {
		t.Errorf("the document policy no longer says who may frame it: %s", policy)
	}
}
