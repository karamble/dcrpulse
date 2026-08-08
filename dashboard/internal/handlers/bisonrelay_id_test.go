// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A Bison Relay identifier is a zkidentity.ShortID rendered as hex, which the
// daemon decodes as exactly 32 bytes. Anything else is refused here rather
// than forwarded into a request URL.
func TestBrID(t *testing.T) {
	const valid = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"a 64 character hex id", valid, true},
		{"upper case hex", strings.ToUpper(valid), true},
		{"surrounding whitespace is trimmed", "  " + valid + "  ", true},
		{"empty", "", false},
		{"one character short", valid[:63], false},
		{"one character long", valid + "0", false},
		{"not hex", strings.Repeat("g", 64), false},

		// The characters that motivated the check: each one reshapes a URL
		// the identifier is interpolated into.
		{"fragment", valid[:60] + "#foo", false},
		{"parameter separator", valid[:60] + "&a=b", false},
		{"query separator", valid[:60] + "?a=b", false},
		{"percent", valid[:60] + "%2e%2e", false},
		{"space", valid[:60] + " abc", false},
		{"path separator", valid[:60] + "/../", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			got, ok := brID(rec, tc.input, "uid")
			if ok != tc.want {
				t.Fatalf("brID(%q) = %v, want %v", tc.input, ok, tc.want)
			}
			if !tc.want {
				if rec.Code != http.StatusBadRequest {
					t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
				}
				if got != "" {
					t.Errorf("rejected input returned %q, want empty", got)
				}
				return
			}
			if got != strings.TrimSpace(tc.input) {
				t.Errorf("returned %q, want the trimmed input", got)
			}
		})
	}
}

// An optional identifier may be absent, but a malformed one is still refused:
// the point is the shape, not the presence.
func TestBrIDOpt(t *testing.T) {
	const valid = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	if got, ok := brIDOpt(httptest.NewRecorder(), "", "uid"); !ok || got != "" {
		t.Errorf("empty optional id = (%q, %v), want (\"\", true)", got, ok)
	}
	if got, ok := brIDOpt(httptest.NewRecorder(), valid, "uid"); !ok || got != valid {
		t.Errorf("valid optional id = (%q, %v), want (%q, true)", got, ok, valid)
	}
	rec := httptest.NewRecorder()
	if _, ok := brIDOpt(rec, "nope", "uid"); ok {
		t.Error("malformed optional id was accepted")
	} else if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
