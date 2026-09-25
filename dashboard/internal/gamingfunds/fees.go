// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingfunds

import (
	"decred.org/dcrwallet/v5/wallet/txrules"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/karamble/dcrgaming-sdk/pkg/finance"
)

// feeRules are dcrwallet's default relay fee and dust rules. Every bridge
// applies the same ones, so co-signers derive the same settlement.
func feeRules() finance.FeeRules {
	return finance.FeeRules{
		Fee: func(size int) int64 {
			return int64(txrules.FeeForSerializeSize(txrules.DefaultRelayFeePerKb, size))
		},
		Dust: func(atoms int64, script []byte) bool {
			return txrules.IsDustAmount(dcrutil.Amount(atoms), len(script), txrules.DefaultRelayFeePerKb)
		},
	}
}
