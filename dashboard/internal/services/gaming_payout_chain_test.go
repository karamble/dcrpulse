// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dcrpulse/internal/gamingfunds"
)

const statusSID = "8be1b656de75b8d54a12b0ffa4f08ff6"

// writeStatusLedger puts a ledger holding the given tables, settlements and
// operations where the bridge reads it.
func writeStatusLedger(t *testing.T, tables, settlements, operations map[string]any) {
	t.Helper()
	t.Cleanup(func() {
		financeStores.Lock()
		path := filepath.Join(GamingStateDir, "financial-authority")
		if s := financeStores.stores[path]; s != nil {
			s.Close()
			delete(financeStores.stores, path)
		}
		financeStores.Unlock()
	})
	raw, err := json.Marshal(map[string]any{
		"version": gamingfunds.Version, "rosterCommits": map[string]any{}, "peers": map[string]any{}, "keys": map[string]any{},
		"previews": map[string]any{}, "quotes": map[string]any{}, "deposits": map[string]any{},
		"tables": tables, "settlements": settlements, "operations": operations,
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(GamingStateDir, "financial-authority")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "authority.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGamingPayoutsCarryTheirChainState(t *testing.T) {
	spendSeams(t)
	scope := gamingfunds.Scope{Game: "stakewars", Network: "mainnet", Wallet: "fp"}
	pending, mined, unsigned := strings.Repeat("8e", 32), strings.Repeat("9f", 32), strings.Repeat("a0", 32)
	writeStatusLedger(t,
		map[string]any{},
		map[string]any{
			pending:  map[string]any{"id": pending, "scope": scope, "table": statusSID, "state": "publishing"},
			mined:    map[string]any{"id": mined, "scope": scope, "table": "0123456789abcdef", "state": "confirmed"},
			unsigned: map[string]any{"id": unsigned, "scope": scope, "table": "fedcba9876543210", "state": "awaiting_approval", "expiresAt": time.Now().Add(time.Hour).Unix()},
		},
		map[string]any{
			pending: map[string]any{"id": pending, "scope": scope, "kind": "settlement", "state": "mempool"},
			mined:   map[string]any{"id": mined, "scope": scope, "kind": "settlement", "state": "confirmed", "confirmations": 3},
		})

	views, err := GamingPayouts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	chains := map[string]*GamingPayoutChain{}
	for _, v := range views {
		chains[v.ID] = v.Chain
	}
	if len(chains) != 3 {
		t.Fatalf("payouts = %d, want 3", len(chains))
	}
	// The game still reads publishing; the operator sees the mempool.
	if c := chains[pending]; c == nil || c.State != "mempool" || c.Confirmations != 0 {
		t.Fatalf("pending payout chain = %+v", c)
	}
	if c := chains[mined]; c == nil || c.State != "confirmed" || c.Confirmations != 3 {
		t.Fatalf("mined payout chain = %+v", c)
	}
	if c := chains[unsigned]; c != nil {
		t.Fatalf("a payout without a transaction has chain state %+v", c)
	}
}
