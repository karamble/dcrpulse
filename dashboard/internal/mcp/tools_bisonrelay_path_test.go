// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestResolveOutboxPath verifies the br_file_send_path sandbox guard: legitimate
// relative names resolve under the configured outbox, while absolute paths,
// "..", and any path escaping the outbox are rejected. This is the tool-layer
// defense against reading arbitrary host files.
func TestResolveOutboxPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MCP_AGENT_OUTBOX_DIR", root)

	t.Run("valid names resolve under the outbox", func(t *testing.T) {
		for _, rel := range []string{"a.png", "sub/b.png", "deep/dir/c.jpg"} {
			got, err := resolveOutboxPath(rel)
			if err != nil {
				t.Fatalf("resolveOutboxPath(%q) unexpected error: %v", rel, err)
			}
			if want := filepath.Join(root, rel); got != want {
				t.Fatalf("resolveOutboxPath(%q) = %q, want %q", rel, got, want)
			}
		}
	})

	t.Run("escapes and absolute paths are rejected", func(t *testing.T) {
		bad := []string{
			"",                 // empty
			"/etc/passwd",      // absolute
			"../secret",        // parent escape
			"a/../../b",        // traversal via segments
			"../../etc/passwd", // deep escape
			"a\\b",             // backslash segment
			"foo/../../../bar", // multi escape
		}
		for _, rel := range bad {
			if got, err := resolveOutboxPath(rel); err == nil {
				t.Fatalf("resolveOutboxPath(%q) = %q, want error", rel, got)
			}
		}
	})
}

// TestSafeStoreMediaName covers the guard that keeps a storefront path - most
// importantly a product's sendfilename, which the store delivers to a buyer on
// purchase - inside the store directory.
func TestSafeStoreMediaName(t *testing.T) {
	t.Run("store-relative names are accepted", func(t *testing.T) {
		for _, p := range []string{
			"manual.pdf",
			"goods/manual.pdf",
			"goods/sub/dir/manual.pdf",
			"cover.png",
		} {
			if !safeStoreMediaName(p) {
				t.Errorf("safeStoreMediaName(%q) = false, want true", p)
			}
		}
	})

	t.Run("escapes and templates are rejected", func(t *testing.T) {
		bad := []string{
			"",                                // empty
			"/app-data/dcrlnd/admin.macaroon", // the S-01 payload
			"/etc/passwd",                     // absolute
			"goods/../../../app-data/dcrlnd/tls.cert", // traversal via segments
			"..",                     // bare parent
			"../secret.pdf",          // parent escape
			"a\\b",                   // backslash segment
			"a\x00b",                 // NUL
			strings.Repeat("a", 256), // over the length cap
			"index.tmpl",             // template, executed by the store
			"index.tmp",              // template scratch
			"INDEX.TMPL",             // case-insensitive
			"index.tmpl. ",           // trailing dot/space evasion
		}
		for _, p := range bad {
			if safeStoreMediaName(p) {
				t.Errorf("safeStoreMediaName(%q) = true, want false", p)
			}
		}
	})
}

// TestStoreProductSendFilenameWiring checks that br_store_save_product actually
// applies the sendfilename guard, which TestSafeStoreMediaName alone cannot see:
// the validator could be correct and simply never called. A rejected name must
// fail before the tool reaches brclientd, so this needs no daemon.
func TestStoreProductSendFilenameWiring(t *testing.T) {
	const agentID = "br-sendfilename-wiring"
	grants.set(agentID, GrantSpec{WriteScopes: []string{scopeBR}}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	cs := connectTo(t, testAgent(agentID, "wiring", map[string]bool{"bisonrelay": true}))
	call := func(t *testing.T, sendfilename string) string {
		t.Helper()
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "br_store_save_product",
			Arguments: map[string]any{
				"sku": "s1", "title": "t", "price": 0.1, "sendfilename": sendfilename,
			},
		})
		if err != nil {
			t.Fatalf("CallTool(%q) transport error: %v", sendfilename, err)
		}
		return resultText(res)
	}

	t.Run("an escaping sendfilename is refused by the tool", func(t *testing.T) {
		for _, bad := range []string{
			"/app-data/dcrlnd/admin.macaroon",
			"goods/../../../app-data/dcrlnd/tls.cert",
			"index.tmpl",
		} {
			if txt := call(t, bad); !strings.Contains(txt, "invalid sendfilename") {
				t.Errorf("sendfilename %q: got %q, want it refused", bad, txt)
			}
		}
	})

	// A good name gets past the guard and then fails further down in the
	// brclientd client (no daemon under test), so assert on the guard's message
	// rather than on IsError - the same carve-out gating_test.go uses for ln_pay.
	t.Run("a store-relative sendfilename passes the guard", func(t *testing.T) {
		for _, ok := range []string{"", "manual.pdf", "goods/sub/manual.pdf"} {
			if txt := call(t, ok); strings.Contains(txt, "invalid sendfilename") {
				t.Errorf("sendfilename %q: refused by the guard, want accepted", ok)
			}
		}
	})
}
