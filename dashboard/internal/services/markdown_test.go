// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBRQuoteSnippet(t *testing.T) {
	words := strings.TrimSpace(strings.Repeat("word ", 60)) // 299 bytes, single-spaced
	cjk := strings.Repeat("世", 100)                         // 300 bytes, 3 per rune
	emoji := strings.Repeat("\U0001f600", 80)               // 320 bytes, 4 per rune

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"short passthrough", "hello world", "hello world"},
		{"word boundary cut", words, strings.TrimSpace(strings.Repeat("word ", 48)) + "..."},
		{"ascii prefix cjk split", "x" + cjk, "x" + strings.Repeat("世", 79) + "..."},
		{"ascii prefix emoji split", "x" + emoji, "x" + strings.Repeat("\U0001f600", 59) + "..."},
		{"aligned cjk keeps every rune", cjk, strings.Repeat("世", 80) + "..."},
		{"space exactly at half stays byte cut", strings.Repeat("a", 120) + " " + strings.Repeat("b", 200),
			strings.Repeat("a", 120) + " " + strings.Repeat("b", 119) + "..."},
	}
	for _, tc := range tests {
		got := BRQuoteSnippet(tc.in, 240)
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("%s: result is not valid UTF-8", tc.name)
		}
	}
}
