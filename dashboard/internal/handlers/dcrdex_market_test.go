// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import "testing"

// A DEX server's unit info is its claim. A market is sized with the local
// asset driver's factor, as bisonw's own UI does; the server's only counts for
// an asset the catalog does not know.
func TestMarketSizedWithLocalConversionFactor(t *testing.T) {
	if got := marketConvFactor(42, 10_000_000_000); got != 100_000_000 {
		t.Errorf("DCR factor = %d, want the catalog's 100000000 over the server's claim", got)
	}
	if got := marketConvFactor(4_000_000_000, 1_000); got != 1_000 {
		t.Errorf("unknown asset factor = %d, want the server's 1000", got)
	}
}

// A token's network fee is paid in its parent chain's asset.
func TestMarketFeeAssetIsTheParentChain(t *testing.T) {
	cf, sym := marketFeeAsset(60001, 1_000_000, "USDC.ETH")
	if cf != 1_000_000_000 || sym != "ETH" {
		t.Errorf("usdc.eth fee asset = %d %s, want 1000000000 ETH", cf, sym)
	}
	cf, sym = marketFeeAsset(42, 100_000_000, "DCR")
	if cf != 100_000_000 || sym != "DCR" {
		t.Errorf("dcr fee asset = %d %s, want 100000000 DCR", cf, sym)
	}
}
