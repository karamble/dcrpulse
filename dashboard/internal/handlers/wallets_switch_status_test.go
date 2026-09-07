// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"dcrpulse/internal/services"
)

// A busy-wallet refusal must reach the UI as a conflict, not as the 500 that
// every other switch failure gets and not as the 401 that would pop a
// passphrase prompt the operator cannot satisfy.
func TestWalletSwitchStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"mixer running", services.ErrSwitchWhileMixing, http.StatusConflict},
		{"purchase running", services.ErrSwitchWhilePurchasing, http.StatusConflict},
		{"wrapped refusal", fmt.Errorf("select wallet: %w", services.ErrSwitchWhileMixing), http.StatusConflict},
		{"wrong public passphrase", fmt.Errorf("open wallet: invalid passphrase"), http.StatusUnauthorized},
		{"anything else", fmt.Errorf("wallet \"beta\" not found"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := walletSwitchStatus(tc.err); got != tc.want {
				t.Errorf("walletSwitchStatus(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
