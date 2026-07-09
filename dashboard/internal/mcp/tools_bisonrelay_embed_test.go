// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestReadChatEmbed verifies the br_embed_get containment: only the exact
// embeds/<uid16>/<file> form resolves, strictly inside brclientd's embeds
// store, and neither traversal nor symlinks can escape it.
func TestReadChatEmbed(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BRCLIENTD_DATA_DIR", root)
	const uid16 = "00aabbccddeeff11"
	embedsDir := filepath.Join(root, "data", "mainnet", "db", "embeds", uid16)
	if err := os.MkdirAll(embedsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{0}, 64)...)
	if err := os.WriteFile(filepath.Join(embedsDir, "20260709_120000.jfif"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	// A file outside the embeds root that traversal would love to reach.
	if err := os.WriteFile(filepath.Join(root, "secret.jpg"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	t.Run("valid reference resolves", func(t *testing.T) {
		got, err := readChatEmbed(ctx, "embeds/"+uid16+"/20260709_120000.jfif")
		if err != nil {
			t.Fatalf("readChatEmbed: %v", err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("bytes differ from the stored embed")
		}
	})

	t.Run("bad references are rejected", func(t *testing.T) {
		bad := []string{
			"",
			"embeds/" + uid16,                    // missing file segment
			"embeds/" + uid16 + "/a/b.jpg",       // nested path
			"embeds/" + uid16 + "/../secret.jpg", // parent escape
			"embeds/" + uid16 + "/..",            // bare dot-dot
			"embeds/00AABBCCDDEEFF11/x.jpg",      // uppercase uid
			"embeds/0aa/x.jpg",                   // short uid
			"downloads/" + uid16 + "/x.jpg",      // wrong root
			"/etc/passwd",                        // absolute
			"embeds/" + uid16 + "/bad\\name.jpg", // backslash
			"../data/mainnet/db/embeds/" + uid16 + "/20260709_120000.jfif",
		}
		for _, ref := range bad {
			if _, err := readChatEmbed(ctx, ref); err == nil {
				t.Errorf("readChatEmbed(%q) succeeded, want error", ref)
			}
		}
	})

	t.Run("symlink source is rejected", func(t *testing.T) {
		link := filepath.Join(embedsDir, "link.jfif")
		if err := os.Symlink(filepath.Join(root, "secret.jpg"), link); err != nil {
			t.Fatal(err)
		}
		if _, err := readChatEmbed(ctx, "embeds/"+uid16+"/link.jfif"); err == nil {
			t.Error("symlinked embed resolved, want error")
		}
	})

	t.Run("oversized embed is rejected", func(t *testing.T) {
		big := filepath.Join(embedsDir, "big.jfif")
		f, err := os.Create(big)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(maxEmbedGetBytes + 1); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		if _, err := readChatEmbed(ctx, "embeds/"+uid16+"/big.jfif"); err == nil {
			t.Error("oversized embed resolved, want error")
		}
	})
}
