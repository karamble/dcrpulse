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

// The upload tool guarded its directory with the media rule but its filename
// with the base one, so an agent could put a .tmpl into the storefront that the
// dashboard's own upload route refuses. Both reach the same brclientd call, so
// the agent surface must not be the more permissive of the two.
func TestStoreFileUploadRejectsTemplateNames(t *testing.T) {
	const agentID = "br-upload-tmpl"
	grants.set(agentID, GrantSpec{WriteScopes: []string{scopeBR}}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	cs := connectTo(t, testAgent(agentID, "upload", map[string]bool{"bisonrelay": true}))
	call := func(t *testing.T, filename string) string {
		t.Helper()
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "br_store_file_upload",
			Arguments: map[string]any{"filename": filename, "dataB64": "aGk="},
		})
		if err != nil {
			t.Fatalf("CallTool(%q) transport error: %v", filename, err)
		}
		return resultText(res)
	}

	for _, bad := range []string{"evil.tmpl", "EVIL.TMPL", "evil.tmpl. ", "evil.tmp"} {
		if txt := call(t, bad); !strings.Contains(txt, "invalid filename") {
			t.Errorf("filename %q: got %q, want it refused", bad, txt)
		}
	}

	// A good name passes the guard and then fails in the brclientd client (no
	// daemon under test), so assert on the guard's own message.
	for _, ok := range []string{"manual.pdf", "cover.png"} {
		if txt := call(t, ok); strings.Contains(txt, "invalid filename") {
			t.Errorf("filename %q: refused by the guard, want accepted", ok)
		}
	}
}

// The asymmetry is deliberate: templates are the one place a .tmpl name is
// correct, so tightening these tools to the media guard would break saving one.
func TestStoreTemplateToolsStillAcceptTmpl(t *testing.T) {
	const agentID = "br-template-tmpl"
	grants.set(agentID, GrantSpec{WriteScopes: []string{scopeBR}}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	cs := connectTo(t, testAgent(agentID, "template", map[string]bool{"bisonrelay": true}))
	for _, tool := range []string{"br_store_template_save", "br_store_template_delete"} {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      tool,
			Arguments: map[string]any{"name": "index.tmpl", "content": "hi"},
		})
		if err != nil {
			t.Fatalf("%s transport error: %v", tool, err)
		}
		if txt := resultText(res); strings.Contains(txt, "invalid name") {
			t.Errorf("%s refused index.tmpl: %q", tool, txt)
		}
	}
}
