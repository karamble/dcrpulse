// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"testing"

	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

// A treasury spend's total is summed in atoms: adding the outputs as float
// DCR gives 0.30000000000000004 for 0.1 + 0.2.
func TestTSpendAmountIsExact(t *testing.T) {
	tx := chainjson.TxRawResult{
		Txid: "ab",
		Vout: []chainjson.Vout{
			{Value: 0.1, ScriptPubKey: chainjson.ScriptPubKeyResult{Addresses: []string{"Dsfirst"}}},
			{Value: 0.2, ScriptPubKey: chainjson.ScriptPubKeyResult{Addresses: []string{"Dspayee"}}},
			{Value: 0},
		},
	}
	got := extractTSpendInfo(tx, 100)
	if got.Amount != 0.3 {
		t.Errorf("Amount = %v, want exactly 0.3", got.Amount)
	}
	if got.Payee != "Dspayee" {
		t.Errorf("Payee = %q, want the last address-bearing output", got.Payee)
	}
}
