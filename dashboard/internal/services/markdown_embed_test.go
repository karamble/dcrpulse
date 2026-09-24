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

// TestBuildAudioNoteEmbedTag locks the voice-note tag to bruig's: alt before
// type, then filename, then data, and "Audio note" with its literal space.
func TestBuildAudioNoteEmbedTag(t *testing.T) {
	got := BuildAudioNoteEmbedTag("2026-09-24-20_33_52-audionote.opus", "T2dnUw==")
	want := "--embed[alt=Audio note,type=audio/ogg,filename=2026-09-24-20_33_52-audionote.opus,data=T2dnUw==]--"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if got := BuildAudioNoteEmbedTag("a,b=c.opus", "X"); got != "--embed[alt=Audio note,type=audio/ogg,filename=abc.opus,data=X]--" {
		t.Fatalf("filename separators not stripped: %q", got)
	}
}
