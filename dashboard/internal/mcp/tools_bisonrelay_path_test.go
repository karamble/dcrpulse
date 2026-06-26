// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"path/filepath"
	"testing"
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
