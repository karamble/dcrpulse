// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http/httptest"
	"testing"
)

// TestServeBRBytesHeaders pins the content-type clamp on peer-supplied bytes.
// A Bison Relay peer picks the type these endpoints are asked to serve, so
// anything outside the inline-image allowlist must come back as an opaque
// download: a type of text/html or text/javascript on the dashboard's own
// origin is script execution under script-src 'self'.
func TestServeBRBytesHeaders(t *testing.T) {
	tests := []struct {
		name         string
		declared     string
		wantType     string
		wantAttached bool
	}{
		{"png renders inline", "image/png", "image/png", false},
		{"jpeg renders inline", "image/jpeg", "image/jpeg", false},
		{"gif renders inline", "image/gif", "image/gif", false},
		{"webp renders inline", "image/webp", "image/webp", false},
		{"parameters are stripped", "image/png; charset=utf-8", "image/png", false},
		{"case is normalized", "IMAGE/PNG", "image/png", false},

		{"html is not served as html", "text/html", "application/octet-stream", true},
		{"javascript is not served as javascript", "text/javascript", "application/octet-stream", true},
		{"legacy javascript type", "application/x-javascript", "application/octet-stream", true},
		{"xhtml is not served as a document", "application/xhtml+xml", "application/octet-stream", true},
		{"svg scripts, so it is not inline", "image/svg+xml", "application/octet-stream", true},
		{"xml is not served as a document", "text/xml", "application/octet-stream", true},
		{"pdf is a download", "application/pdf", "application/octet-stream", true},
		{"empty declares nothing", "", "application/octet-stream", true},
		{"unparseable falls back", "not a mime type at all", "application/octet-stream", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			serveBRBytesHeaders(rec, tt.declared)

			if got := rec.Header().Get("Content-Type"); got != tt.wantType {
				t.Fatalf("declared %q: content type %q, want %q", tt.declared, got, tt.wantType)
			}
			attached := rec.Header().Get("Content-Disposition") != ""
			if attached != tt.wantAttached {
				t.Fatalf("declared %q: attachment %v, want %v", tt.declared, attached, tt.wantAttached)
			}
			// The sandbox policy rides every response: an opaque origin is the
			// backstop for any type that slips through the allowlist.
			if got := rec.Header().Get("Content-Security-Policy"); got != "default-src 'none'; sandbox" {
				t.Fatalf("declared %q: csp %q", tt.declared, got)
			}
		})
	}
}

// TestServeBRBytesHeadersNoPeerFilename pins that the attachment disposition
// carries no filename parameter. These names come from the peer, and quoting
// them into a header is how header injection starts.
func TestServeBRBytesHeadersNoPeerFilename(t *testing.T) {
	rec := httptest.NewRecorder()
	serveBRBytesHeaders(rec, "text/html")
	if got := rec.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("disposition %q, want a bare attachment", got)
	}
}
