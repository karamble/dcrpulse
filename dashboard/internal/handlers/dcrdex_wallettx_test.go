// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import "testing"

// A token wallet pays its network fees in the parent chain's coin, so its
// history fees are sized with that coin's factor and say which coin they are
// in, as bisonw's own wallet page does; the amount stays in the token.
func TestWalletTxFeesUseTheFeeAssetUnit(t *testing.T) {
	const usdcEth, eth, dcr = 60001, 60, 42
	token := uint32(usdcEth)
	for _, tc := range []struct {
		name      string
		wallet    uint32
		tx        rawWalletTx
		amount    float64
		fees      float64
		feeSymbol string
	}{
		{"token wallet", usdcEth, rawWalletTx{Amount: 5_000_000, Fees: 2_000_000}, 5, 0.002, "eth"},
		{"parent wallet, token transfer", eth, rawWalletTx{Amount: 5_000_000, Fees: 2_000_000, TokenID: &token}, 5, 0.002, ""},
		{"dcr wallet", dcr, rawWalletTx{Amount: 150_000_000, Fees: 2_980}, 1.5, 0.0000298, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := convWalletTx(tc.wallet, tc.tx)
			if got.Amount != tc.amount || got.Fees != tc.fees || got.FeeSymbol != tc.feeSymbol {
				t.Errorf("convWalletTx = amount %v fees %v %q, want %v fees %v %q",
					got.Amount, got.Fees, got.FeeSymbol, tc.amount, tc.fees, tc.feeSymbol)
			}
		})
	}
}
