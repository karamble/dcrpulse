// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"strings"
	"testing"
)

// The probe appends its request path to whatever this returns, so anything
// that would carry the path away with it has to be refused here.
func TestVSPBaseURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"a bare host gets https", "vsp.example.com", "https://vsp.example.com"},
		{"https is kept", "https://vsp.example.com", "https://vsp.example.com"},
		{"http is upgraded", "http://vsp.example.com", "https://vsp.example.com"},
		{"a trailing slash is dropped", "https://vsp.example.com/", "https://vsp.example.com"},
		{"surrounding whitespace is trimmed", "  vsp.example.com  ", "https://vsp.example.com"},
		{"a port is kept", "vsp.example.com:8443", "https://vsp.example.com:8443"},

		{"a query is refused", "vsp.example.com/x?a=b", ""},
		{"a fragment is refused", "vsp.example.com#frag", ""},
		{"an empty host is refused", "", ""},
		{"a scheme with no host is refused", "https://", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := vspBaseURL(tc.input)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("vspBaseURL(%q) = %q, want an error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("vspBaseURL(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("vspBaseURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// The concrete shape the old concatenation produced: a host carrying a query
// swallowed the request path, so the probe asked for something else entirely.
func TestVSPBaseURLKeepsTheRequestPath(t *testing.T) {
	if _, err := vspBaseURL("vsp.example.com/x?a="); err == nil {
		t.Fatal("a host carrying a query was accepted; the request path would be swallowed")
	}

	base, err := vspBaseURL("vsp.example.com")
	if err != nil {
		t.Fatalf("vspBaseURL: %v", err)
	}
	if got := base + vspInfoPathV3; !strings.HasSuffix(got, vspInfoPathV3) {
		t.Errorf("probe URL = %q, want it to end in %q", got, vspInfoPathV3)
	}
}
