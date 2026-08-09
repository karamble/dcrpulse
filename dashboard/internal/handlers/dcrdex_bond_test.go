// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import "testing"

// bondAssetID 0 is Bitcoin's real BIP-44 id AND the zero value of an account
// that never configured a bond asset. bisonw disambiguates by tier maintenance
// (targetTier >= 1), never by the id, so the resolver must do the same.
func TestResolveBondAsset(t *testing.T) {
	dcr := dexBondOffer{ID: 42, Confs: 2, Amt: 200e8}
	btc := dexBondOffer{ID: 0, Confs: 2, Amt: 5e5}
	tests := []struct {
		name    string
		tier    uint64
		assetID uint32
		offered map[string]dexBondOffer
		wantSym string
		wantID  uint32
		wantAmt uint64
	}{
		{"unset on a dcr-only server", 0, 0, map[string]dexBondOffer{"dcr": dcr}, "dcr", 42, 200e8},
		{"unset must not read as btc", 0, 0, map[string]dexBondOffer{"btc": btc, "dcr": dcr}, "dcr", 42, 200e8},
		{"a maintained btc bond survives", 3, 0, map[string]dexBondOffer{"btc": btc, "dcr": dcr}, "btc", 0, 5e5},
		{"maintained but no longer offered", 1, 60, map[string]dexBondOffer{"dcr": dcr}, "dcr", 42, 200e8},
		{"unset and nothing offered", 0, 0, map[string]dexBondOffer{}, "dcr", 0, 0},
	}
	for _, tc := range tests {
		sym, offer := resolveBondAsset(tc.tier, tc.assetID, tc.offered)
		if sym != tc.wantSym || offer.ID != tc.wantID || offer.Amt != tc.wantAmt {
			t.Errorf("%s: got %q id=%d amt=%d, want %q id=%d amt=%d",
				tc.name, sym, offer.ID, offer.Amt, tc.wantSym, tc.wantID, tc.wantAmt)
		}
	}
}
