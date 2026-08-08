// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"encoding/json"
	"testing"

	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

// verboseBlock renders a getblock reply the way dcrd does when asked for its
// transactions inline. Building it from dcrd's own result types rather than
// hand-written JSON means a renamed field breaks the test instead of slipping
// through.
func verboseBlock(t *testing.T, regular, stake []chainjson.TxRawResult) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(chainjson.GetBlockVerboseResult{
		Hash:          "11aecd7e0afdd46253166835e4ccae46134a07000d6c55d56b8bc308d06d86da",
		Height:        1104000,
		Time:          1786021850,
		Confirmations: 486,
		Size:          4096,
		RawTx:         regular,
		RawSTx:        stake,
	})
	if err != nil {
		t.Fatalf("marshal block: %v", err)
	}
	return raw
}

// regularTx is an ordinary transaction: a spending input, so no stakebase key
// survives Vin's custom marshalling.
func regularTx(txid string) chainjson.TxRawResult {
	return chainjson.TxRawResult{
		Txid: txid,
		Hex:  "0100000001de",
		Vin: []chainjson.Vin{{
			Txid:     "aa",
			AmountIn: 3.5,
		}},
		Vout: []chainjson.Vout{{
			Value:        3.4,
			ScriptPubKey: chainjson.ScriptPubKeyResult{Type: "pubkeyhash"},
		}},
	}
}

// voteTx carries a stakebase input, which is the only thing distinguishing a
// vote once Vin has been marshalled.
func voteTx(txid string) chainjson.TxRawResult {
	return chainjson.TxRawResult{
		Txid: txid,
		Hex:  "0100000001beef",
		Vin: []chainjson.Vin{{
			Stakebase: "00",
			AmountIn:  1.2,
		}},
		Vout: []chainjson.Vout{{
			Value:        1.2,
			ScriptPubKey: chainjson.ScriptPubKeyResult{Type: "stakegen-pubkeyhash"},
		}},
	}
}

func TestBlockDetailFromVerbose(t *testing.T) {
	tests := []struct {
		name          string
		regular       []chainjson.TxRawResult
		stake         []chainjson.TxRawResult
		wantTxCount   int
		wantFirstType string
	}{{
		name:          "regular and stake transactions",
		regular:       []chainjson.TxRawResult{regularTx("a1"), regularTx("a2")},
		stake:         []chainjson.TxRawResult{voteTx("b1")},
		wantTxCount:   3,
		wantFirstType: "regular",
	}, {
		name:          "stake only",
		stake:         []chainjson.TxRawResult{voteTx("b1"), voteTx("b2")},
		wantTxCount:   2,
		wantFirstType: "vote",
	}, {
		name:        "empty block",
		wantTxCount: 0,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			block, err := blockDetailFromVerbose(verboseBlock(t, tc.regular, tc.stake))
			if err != nil {
				t.Fatalf("blockDetailFromVerbose: %v", err)
			}

			// The count must come from the inline arrays. dcrd sends either
			// those or the txid arrays, never both, so reading the wrong pair
			// silently reports an empty block.
			if block.TxCount != tc.wantTxCount {
				t.Errorf("TxCount = %d, want %d", block.TxCount, tc.wantTxCount)
			}
			if len(block.Transactions) != tc.wantTxCount {
				t.Errorf("len(Transactions) = %d, want %d", len(block.Transactions), tc.wantTxCount)
			}
			if tc.wantTxCount == 0 {
				return
			}

			if got := block.Transactions[0].Type; got != tc.wantFirstType {
				t.Errorf("first transaction type = %q, want %q", got, tc.wantFirstType)
			}
			// Size is derived from the hex, which dcrd carries inline and no
			// separate size field replaces.
			if block.Transactions[0].Size == 0 {
				t.Error("first transaction size = 0, want it derived from hex")
			}
		})
	}
}

// TestBlockDetailFromVerboseIgnoresTxidArrays pins the reason the inline
// arrays are read: a reply carrying only txids must not masquerade as a block
// whose transactions were returned.
func TestBlockDetailFromVerboseIgnoresTxidArrays(t *testing.T) {
	raw, err := json.Marshal(chainjson.GetBlockVerboseResult{
		Hash:   "11aecd7e",
		Height: 1104000,
		Tx:     []string{"a1", "a2"},
		STx:    []string{"b1"},
	})
	if err != nil {
		t.Fatalf("marshal block: %v", err)
	}

	block, err := blockDetailFromVerbose(raw)
	if err != nil {
		t.Fatalf("blockDetailFromVerbose: %v", err)
	}
	if block.TxCount != 0 || len(block.Transactions) != 0 {
		t.Fatalf("txid-only reply produced TxCount=%d, %d transactions; want 0, 0",
			block.TxCount, len(block.Transactions))
	}
}
