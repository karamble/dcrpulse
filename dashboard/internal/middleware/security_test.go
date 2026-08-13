// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package middleware

import (
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
