// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"testing"

	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

func TestClassifyScriptType(t *testing.T) {
	tests := []struct{ scriptType, want string }{
		{"treasurygen", "tspend"},
		{"treasurybase", "treasurybase"},
		{"treasuryadd", "treasurybase"},
		{"stakesubmission", "ticket"},
		{"stakegen", "vote"},
		{"stakerevoke", "revocation"},
		{"pubkeyhash", ""},
		{"", ""},
	}
	for _, test := range tests {
		if got := classifyScriptType(test.scriptType); got != test.want {
			t.Errorf("classifyScriptType(%q) = %q, want %q", test.scriptType, got, test.want)
		}
	}
}

// dcrd never emits an empty discriminator value, so a non-empty Stakebase or
// Coinbase field is the input-class test.
func TestCategorizeTransactionInputClass(t *testing.T) {
	vout := []chainjson.Vout{{
		Value:        1.0,
		ScriptPubKey: chainjson.ScriptPubKeyResult{Type: "pubkeyhash"},
	}}

	tests := []struct {
		name string
		vin  []chainjson.Vin
		want string
	}{{
		name: "stakebase input marks a vote",
		vin:  []chainjson.Vin{{Stakebase: "00"}},
		want: "vote",
	}, {
		name: "coinbase input marks a coinbase",
		vin:  []chainjson.Vin{{Coinbase: "00"}},
		want: "coinbase",
	}, {
		name: "neither marks an ordinary input",
		vin:  []chainjson.Vin{{Txid: "ab"}},
		want: "regular",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := categorizeTransactionTyped(test.vin, vout); got != test.want {
				t.Fatalf("categorizeTransactionTyped = %q, want %q", got, test.want)
			}
		})
	}
}

// The first output that says anything decides, so an earlier stake output wins
// over a later treasury one.
func TestCategorizeTransactionFirstOutputWins(t *testing.T) {
	vin := []chainjson.Vin{{Txid: "ab"}}
	vout := []chainjson.Vout{
		{Value: 1.0, ScriptPubKey: chainjson.ScriptPubKeyResult{Type: "stakesubmission"}},
		{Value: 1.0, ScriptPubKey: chainjson.ScriptPubKeyResult{Type: "treasurygen"}},
	}
	if got := categorizeTransactionTyped(vin, vout); got != "ticket" {
		t.Fatalf("categorizeTransactionTyped = %q, want ticket", got)
	}
}
