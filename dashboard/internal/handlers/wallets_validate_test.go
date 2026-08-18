// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"strings"
	"testing"

	"dcrpulse/internal/types"
)

// The helper is the single enforcement point for both create routes, so the
// policy is pinned here: messages, limits, and which error wins on double
// failure.
func TestValidateCreateWalletPassphrases(t *testing.T) {
	long := strings.Repeat("x", 1025)
	tests := []struct {
		name string
		req  types.CreateWalletRequest
		want string
	}{
		{"missing private", types.CreateWalletRequest{}, "Private passphrase is required"},
		{"short private", types.CreateWalletRequest{
			PrivatePassphrase: "1234567", ConfirmPrivatePassphrase: "1234567",
		}, "Private passphrase must be at least 8 characters"},
		{"private too long", types.CreateWalletRequest{
			PrivatePassphrase: long, ConfirmPrivatePassphrase: "12345678",
		}, "Passphrase too long"},
		{"confirm private too long", types.CreateWalletRequest{
			PrivatePassphrase: "12345678", ConfirmPrivatePassphrase: long,
		}, "Passphrase too long"},
		{"public too long", types.CreateWalletRequest{
			PrivatePassphrase: "12345678", ConfirmPrivatePassphrase: "12345678",
			PublicPassphrase: long, ConfirmPublicPassphrase: "abcdefgh",
		}, "Passphrase too long"},
		{"confirm public too long", types.CreateWalletRequest{
			PrivatePassphrase: "12345678", ConfirmPrivatePassphrase: "12345678",
			PublicPassphrase: "abcdefgh", ConfirmPublicPassphrase: long,
		}, "Passphrase too long"},
		{"private mismatch", types.CreateWalletRequest{
			PrivatePassphrase: "12345678", ConfirmPrivatePassphrase: "12345679",
		}, "Private passphrases do not match"},
		{"short private beats mismatch", types.CreateWalletRequest{
			PrivatePassphrase: "1234567", ConfirmPrivatePassphrase: "different",
		}, "Private passphrase must be at least 8 characters"},
		{"short public", types.CreateWalletRequest{
			PrivatePassphrase: "12345678", ConfirmPrivatePassphrase: "12345678",
			PublicPassphrase: "abcdefg", ConfirmPublicPassphrase: "abcdefg",
		}, "Public passphrase must be at least 8 characters"},
		{"public mismatch", types.CreateWalletRequest{
			PrivatePassphrase: "12345678", ConfirmPrivatePassphrase: "12345678",
			PublicPassphrase: "abcdefgh", ConfirmPublicPassphrase: "abcdefgi",
		}, "Public passphrases do not match"},
		{"empty public skips public checks", types.CreateWalletRequest{
			PrivatePassphrase: "12345678", ConfirmPrivatePassphrase: "12345678",
			ConfirmPublicPassphrase: "ignored",
		}, ""},
		{"valid with public", types.CreateWalletRequest{
			PrivatePassphrase: "12345678", ConfirmPrivatePassphrase: "12345678",
			PublicPassphrase: "abcdefgh", ConfirmPublicPassphrase: "abcdefgh",
		}, ""},
	}
	for _, tc := range tests {
		if got := validateCreateWalletPassphrases(&tc.req); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
