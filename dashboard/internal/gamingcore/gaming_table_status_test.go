// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dcrpulse/internal/gamingfunds"
)

func statusTableKey(t *testing.T, scope gamingfunds.Scope, table string) string {
	t.Helper()
	key, err := json.Marshal(struct {
		Scope gamingfunds.Scope
		Table string
	}{scope, table})
	if err != nil {
		t.Fatal(err)
	}
	return string(key)
}

func TestGamingTableStatusWithoutLedgerCreatesNothing(t *testing.T) {
	br := newTestBridge(t)
	spendSeams(t, br)
	got, err := br.ReadGamingTableStatus("stakewars", statusSID)
	if err != nil || got.Accepted || got.SeatBond != nil || got.Payout != nil {
		t.Fatalf("status without a ledger = %+v, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(br.dataDir, "financial-authority", "authority.json")); !os.IsNotExist(err) {
		t.Fatalf("reading the status created a ledger: %v", err)
	}
}

func TestGamingTableStatusReadsTheLedger(t *testing.T) {
	br := newTestBridge(t)
	spendSeams(t, br)
	scope := gamingfunds.Scope{Game: "stakewars", Network: "mainnet", Wallet: "fp"}
	poker := gamingfunds.Scope{Game: "poker", Network: "mainnet", Wallet: "fp"}
	table := func(s gamingfunds.Scope, sid string, closed bool) map[string]any {
		return map[string]any{"scope": s, "table": sid, "seats": 2, "until": 1, "closed": closed}
	}
	payout, expired, stale, elsewhere := strings.Repeat("8e", 32), strings.Repeat("11", 32), strings.Repeat("33", 32), strings.Repeat("22", 32)
	writeStatusLedger(t, br,
		map[string]any{
			statusTableKey(t, scope, statusSID):          table(scope, statusSID, false),
			statusTableKey(t, poker, statusSID):          table(poker, statusSID, true),
			statusTableKey(t, scope, "0123456789abcdef"): table(scope, "0123456789abcdef", false),
		},
		map[string]any{
			expired:   map[string]any{"id": expired, "scope": scope, "table": statusSID, "state": "expired"},
			stale:     map[string]any{"id": stale, "scope": scope, "table": statusSID, "state": "awaiting_signatures", "expiresAt": time.Now().Add(time.Hour).Unix()},
			payout:    map[string]any{"id": payout, "scope": scope, "table": statusSID, "state": "publishing"},
			elsewhere: map[string]any{"id": elsewhere, "scope": scope, "table": "0123456789abcdef", "state": "confirmed"},
		},
		map[string]any{
			payout: map[string]any{"id": payout, "scope": scope, "kind": "settlement", "state": "mempool"},
		})

	now := time.Now().Unix()
	bondTx, stakeTx := strings.Repeat("4c", 32), strings.Repeat("82", 32)
	if err := br.writeSpendLog(spendLog{Spends: []GamingSpend{
		{ID: "b1", Game: "stakewars", TableID: statusSID, DepositKind: "seatbond", State: GamingSpendFailed, RequestedAt: now - 50},
		{ID: "b2", Game: "stakewars", TableID: statusSID, DepositKind: "seatbond", State: GamingSpendApproved, TxID: bondTx, RequestedAt: now - 40},
		{ID: "s1", Game: "stakewars", TableID: statusSID, DepositKind: "stake", State: GamingSpendApproved, TxID: stakeTx, RequestedAt: now - 30},
		{ID: "x1", Game: "stakewars", TableID: "0123456789abcdef", DepositKind: "seatbond", State: GamingSpendDenied, RequestedAt: now - 20},
		{ID: "x2", Game: "poker", TableID: statusSID, DepositKind: "stake", State: GamingSpendDenied, RequestedAt: now - 10},
		{ID: "x3", Game: "stakewars", TableID: statusSID, State: GamingSpendDenied, RequestedAt: now},
		{ID: "x4", Game: "stakewars", TableID: "aaaa", DepositKind: "seatbond", State: GamingSpendApproved, TxID: bondTx, RequestedAt: now},
	}}, now); err != nil {
		t.Fatal(err)
	}

	got, err := br.ReadGamingTableStatus("stakewars", statusSID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Accepted || got.Closed {
		t.Fatalf("accepted/closed = %v/%v, want true/false (another game's closed table is not this one)", got.Accepted, got.Closed)
	}
	if got.SeatBond == nil || got.SeatBond.State != GamingSpendApproved || got.SeatBond.TxID != bondTx {
		t.Fatalf("seat bond = %+v, want the newer approved request", got.SeatBond)
	}
	if got.Stake == nil || got.Stake.State != GamingSpendApproved || got.Stake.TxID != stakeTx {
		t.Fatalf("stake = %+v", got.Stake)
	}
	if got.Payout == nil || got.Payout.ID != payout || got.Payout.State != "publishing" {
		t.Fatalf("payout = %+v, want the published one over the expired and unsigned ones", got.Payout)
	}
	if got.Payout.Chain == nil || got.Payout.Chain.State != "mempool" {
		t.Fatalf("payout chain = %+v, want mempool", got.Payout.Chain)
	}

	other, err := br.ReadGamingTableStatus("poker", statusSID)
	if err != nil || !other.Accepted || !other.Closed || other.Stake == nil || other.Stake.State != GamingSpendDenied || other.SeatBond != nil || other.Payout != nil {
		t.Fatalf("closed poker table = %+v, %v", other, err)
	}
	// A spend for a table the operator never accepted says nothing about it.
	none, err := br.ReadGamingTableStatus("stakewars", "aaaa")
	if err != nil || none.Accepted || none.SeatBond != nil || none.Payout != nil {
		t.Fatalf("unaccepted table = %+v, %v", none, err)
	}
}
