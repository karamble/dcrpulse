// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

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

// dcrd emits a different key set per input class, so a regular input carries no
// stakebase key at all. The map form must test presence, not value.
func TestCategorizeTransactionInputClass(t *testing.T) {
	vout := []interface{}{map[string]interface{}{
		"value":        1.0,
		"scriptPubKey": map[string]interface{}{"type": "pubkeyhash"},
	}}

	tests := []struct {
		name string
		vin  []interface{}
		want string
	}{{
		name: "stakebase key present marks a vote",
		vin:  []interface{}{map[string]interface{}{"stakebase": "00"}},
		want: "vote",
	}, {
		name: "an empty stakebase value is still a vote",
		vin:  []interface{}{map[string]interface{}{"stakebase": ""}},
		want: "vote",
	}, {
		name: "coinbase key present marks a coinbase",
		vin:  []interface{}{map[string]interface{}{"coinbase": "00"}},
		want: "coinbase",
	}, {
		name: "neither key means an ordinary input",
		vin:  []interface{}{map[string]interface{}{"txid": "ab"}},
		want: "regular",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := categorizeTransaction(test.vin, vout); got != test.want {
				t.Fatalf("categorizeTransaction = %q, want %q", got, test.want)
			}
		})
	}
}

// The first output that says anything decides, so an earlier stake output wins
// over a later treasury one.
func TestCategorizeTransactionFirstOutputWins(t *testing.T) {
	vin := []interface{}{map[string]interface{}{"txid": "ab"}}
	vout := []interface{}{
		map[string]interface{}{"value": 1.0, "scriptPubKey": map[string]interface{}{"type": "stakesubmission"}},
		map[string]interface{}{"value": 1.0, "scriptPubKey": map[string]interface{}{"type": "treasurygen"}},
	}
	if got := categorizeTransaction(vin, vout); got != "ticket" {
		t.Fatalf("categorizeTransaction = %q, want ticket", got)
	}
}
