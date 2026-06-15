// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

// requiredSet infers the JSON schema for a tool input type and returns its set
// of required property names.
func requiredSet[T any](t *testing.T) map[string]bool {
	t.Helper()
	s, err := jsonschema.For[T](nil)
	if err != nil {
		t.Fatalf("infer schema: %v", err)
	}
	m := map[string]bool{}
	for _, r := range s.Required {
		m[r] = true
	}
	return m
}

// TestToolInputOptionalParams verifies that fields meant to be optional are not
// marked required in the generated input schema (omitempty), while genuinely
// required parameters still are. Regression guard: without omitempty the SDK
// marks every field required, so calls omitting page/count/etc. were rejected.
func TestToolInputOptionalParams(t *testing.T) {
	// Group-chat history: id required, paging optional.
	gc := requiredSet[brGCHistoryInput](t)
	if !gc["gcid"] {
		t.Error("br_groupchat_history: gcid should be required")
	}
	if gc["page"] || gc["pageSize"] {
		t.Errorf("br_groupchat_history: page/pageSize should be optional, required=%v", gc)
	}

	// PM history: uid required, paging optional.
	pm := requiredSet[brPmHistoryInput](t)
	if !pm["uid"] {
		t.Error("br_pm_history: uid should be required")
	}
	if pm["page"] || pm["pageSize"] {
		t.Errorf("br_pm_history: page/pageSize should be optional, required=%v", pm)
	}

	// Fully-optional inputs should require nothing.
	if tx := requiredSet[txListInput](t); len(tx) != 0 {
		t.Errorf("wallet_transactions: expected no required fields, got %v", tx)
	}
	if no := requiredSet[brNotificationsInput](t); len(no) != 0 {
		t.Errorf("br_notifications: expected no required fields, got %v", no)
	}
	if dx := requiredSet[dexOrdersInput](t); len(dx) != 0 {
		t.Errorf("dex_orders: expected no required fields, got %v", dx)
	}

	// Explorer lookups need their target, so it stays required.
	if !requiredSet[explorerAddressInput](t)["address"] {
		t.Error("explorer_address: address should be required")
	}
	if !requiredSet[explorerTxInput](t)["txHash"] {
		t.Error("explorer_transaction: txHash should be required")
	}
}
