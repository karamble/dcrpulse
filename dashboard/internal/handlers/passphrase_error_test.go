// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"fmt"
	"testing"

	"dcrpulse/internal/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsWrongPassphrase(t *testing.T) {
	// What dcrwallet sends for errors.Passphrase (translateError).
	walletRejects := status.Error(codes.InvalidArgument, "wallet.UnlockAccount: invalid passphrase")
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"wallet rejects the passphrase",
			fmt.Errorf("unlock source account: %w", walletRejects), true},
		{"Lightning daemon cannot decrypt",
			fmt.Errorf("UnlockWallet: %w", status.Error(codes.Unknown, "unable to decrypt the wallet")), true},
		{"no account unlocks",
			fmt.Errorf("unlock change account: %w", services.ErrWrongPassphrase), true},
		{"Politeia reply quoted in the error",
			fmt.Errorf("castballot: politeia /castballot: status 400: invalid passphrase in ballot"), false},
		{"new account not encrypted for another reason",
			fmt.Errorf("account created but failed to set per-account passphrase: %w",
				status.Error(codes.DeadlineExceeded, "context deadline exceeded")), false},
		{"rotation that went through for some accounts",
			&services.PartialPassphraseChangeError{Accounts: []uint32{2}, Err: walletRejects}, false},
		{"daemon reply hidden by %v",
			fmt.Errorf("sign the attestation: %v", walletRejects), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isWrongPassphrase(tc.err); got != tc.want {
				t.Fatalf("isWrongPassphrase(%q) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
