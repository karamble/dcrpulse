// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utils

import (
	"strings"
	"testing"
)

// One implementation guards both the dashboard routes and the agent tools, so
// its table lives with it rather than in either caller.

func TestSafeBRPath(t *testing.T) {
	for _, p := range []string{
		"manual.pdf",
		"goods/manual.pdf",
		"goods/sub/dir/manual.pdf",
		"index.tmpl", // a template is a legitimate name for the base guard
		strings.Repeat("a", 255),
	} {
		if !SafeBRPath(p) {
			t.Errorf("SafeBRPath(%q) = false, want true", p)
		}
	}

	for _, p := range []string{
		"",                                // empty
		"/app-data/dcrlnd/admin.macaroon", // absolute, the S-01 payload
		"/etc/passwd",
		"goods/../../../app-data/dcrlnd/tls.cert", // traversal via segments
		"..",                     // bare parent
		"../secret.pdf",          // parent escape
		"a\\b",                   // backslash segment
		"a\x00b",                 // NUL
		strings.Repeat("a", 256), // over the length cap
	} {
		if SafeBRPath(p) {
			t.Errorf("SafeBRPath(%q) = true, want false", p)
		}
	}
}

// The store parses and executes *.tmpl, so a name that reaches the media
// endpoints must never end there, however it is dressed up.
func TestSafeStoreMediaPath(t *testing.T) {
	for _, p := range []string{"manual.pdf", "goods/manual.pdf", "cover.png"} {
		if !SafeStoreMediaPath(p) {
			t.Errorf("SafeStoreMediaPath(%q) = false, want true", p)
		}
	}

	for _, p := range []string{
		"index.tmpl",   // template, executed by the store
		"index.tmp",    // template scratch
		"INDEX.TMPL",   // case-insensitive
		"index.tmpl. ", // trailing dot/space evasion
		"../secret.pdf",
		"",
	} {
		if SafeStoreMediaPath(p) {
			t.Errorf("SafeStoreMediaPath(%q) = true, want false", p)
		}
	}
}
