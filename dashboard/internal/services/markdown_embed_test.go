// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

// TestBuildEmbedTag locks the bruig --embed[...]-- wire format that both the
// HTTP PM/GC handlers and the MCP image-send tools depend on: field order
// (name, type, data), defensive stripping of commas and '=' from name/mime
// (the bruig parser does no escaping), and omission of empty fields.
func TestBuildEmbedTag(t *testing.T) {
	cases := []struct {
		name, mime, data string
		want             string
	}{
		{"photo.jpg", "image/jpeg", "QUJD", "--embed[name=photo.jpg,type=image/jpeg,data=QUJD]--"},
		{"a,b=c.png", "image/png", "DATA", "--embed[name=abc.png,type=image/png,data=DATA]--"},
		{"", "", "X", "--embed[data=X]--"},
		{"only.png", "", "", "--embed[name=only.png]--"},
	}
	for _, c := range cases {
		if got := BuildEmbedTag(c.name, c.mime, c.data); got != c.want {
			t.Fatalf("BuildEmbedTag(%q,%q,%q) = %q, want %q", c.name, c.mime, c.data, got, c.want)
		}
	}
}
