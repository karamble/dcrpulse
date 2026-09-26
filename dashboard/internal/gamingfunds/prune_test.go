package gamingfunds

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const (
	pruneNow   = int64(1_800_000_000)
	pruneDepth = int64(144)
	pruneGroup = "group-settled"
	pruneOther = "group-other"
)

var (
	pruneScope   = Scope{Game: "poker", Network: "mainnet", Wallet: "fp", Account: 0}
	pruneSpend   = strings.Repeat("5a", 32)
	pruneFunding = strings.Repeat("f0", 32)
)

// prunedLedger holds one settled table in pruneGroup whose single deposit was
// spent by pruneSpend, plus an unrelated settled table in pruneOther.
func prunedLedger(t *testing.T, edit func(d *diskState)) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	d := emptyState()
	for i, table := range []struct{ id, group, dep string }{
		{"table-1", pruneGroup, "dep-1"},
		{"table-9", pruneOther, "dep-9"},
	} {
		key, _ := scopeKey(pruneScope, table.id)
		d.Tables[key] = TableAuthorization{Scope: pruneScope, Table: table.id, Group: table.group}
		terms := Terms{Table: table.id}
		d.Deposits[table.dep] = Deposit{ID: table.dep, Scope: pruneScope, Terms: terms, State: "spent",
			SpendingTx: pruneSpend, Outpoint: fmt.Sprintf("%s:%d", pruneFunding, i), FundingTx: pruneFunding}
	}
	d.Settlements[pruneSpend] = Settlement{ID: pruneSpend, Scope: pruneScope, Table: "table-1", State: "confirmed"}
	d.Operations[pruneSpend] = Operation{ID: pruneSpend, Scope: pruneScope, Kind: "settlement",
		Inputs: []string{pruneFunding + ":0"}, State: "confirmed"}
	if edit != nil {
		edit(&d)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.save(d); err != nil {
		t.Fatal(err)
	}
	return s
}

func depthOf(n int64) func(string) (int64, error) {
	return func(txid string) (int64, error) {
		if txid != pruneSpend {
			return 0, errors.New("unexpected txid " + txid)
		}
		return n, nil
	}
}

func TestPrunableGroupsOnlySettledAndDeep(t *testing.T) {
	s := prunedLedger(t, nil)
	got, err := s.PrunableGroups(pruneNow, pruneDepth, depthOf(pruneDepth))
	if err != nil || !reflect.DeepEqual(got, []string{pruneOther, pruneGroup}) {
		t.Fatalf("settled 144 deep = %v, %v", got, err)
	}
	got, err = s.PrunableGroups(pruneNow, pruneDepth, depthOf(pruneDepth-1))
	if err != nil || len(got) != 0 {
		t.Fatalf("settled 143 deep = %v, %v", got, err)
	}
	got, err = s.PrunableGroups(pruneNow, pruneDepth, func(string) (int64, error) { return 0, errors.New("dcrd down") })
	if err == nil || got != nil {
		t.Fatalf("dcrd failure = %v, %v", got, err)
	}
}

func TestPrunableGroupsKeepsOpenMoney(t *testing.T) {
	for name, edit := range map[string]func(d *diskState){
		"deposit confirmed": func(d *diskState) {
			dep := d.Deposits["dep-1"]
			dep.State, dep.SpendingTx = "confirmed", ""
			d.Deposits["dep-1"] = dep
		},
		"spend pending": func(d *diskState) {
			dep := d.Deposits["dep-1"]
			dep.State = "spend_pending"
			d.Deposits["dep-1"] = dep
		},
		"spent without a spender": func(d *diskState) {
			dep := d.Deposits["dep-1"]
			dep.SpendingTx = ""
			d.Deposits["dep-1"] = dep
		},
		"second deposit unspent": func(d *diskState) {
			d.Deposits["dep-2"] = Deposit{ID: "dep-2", Scope: pruneScope, Terms: Terms{Table: "table-1"}, State: "recovery_pending", SpendingTx: pruneSpend}
		},
		"table never funded": func(d *diskState) {
			key, _ := scopeKey(pruneScope, "table-2")
			d.Tables[key] = TableAuthorization{Scope: pruneScope, Table: "table-2", Group: pruneGroup}
		},
		"settlement awaiting signatures": func(d *diskState) {
			d.Settlements["other"] = Settlement{ID: "other", Scope: pruneScope, Table: "table-1", State: "awaiting_signatures"}
		},
		"settlement publishing": func(d *diskState) {
			p := d.Settlements[pruneSpend]
			p.State = "publishing"
			d.Settlements[pruneSpend] = p
		},
		"operation spending the deposit in mempool": func(d *diskState) {
			op := d.Operations[pruneSpend]
			op.State = "mempool"
			d.Operations[pruneSpend] = op
		},
		"recovery publishing": func(d *diskState) {
			d.Operations["rec"] = Operation{ID: "rec", Scope: pruneScope, Kind: "recovery", DepositIDs: []string{"dep-1"}, State: "publishing"}
		},
		"live funding preview": func(d *diskState) {
			d.Previews["pv"] = PaymentPreview{DepositID: "dep-1", RequestedAt: pruneNow - 10, ExpiresAt: pruneNow + 10}
		},
		"live recovery quote": func(d *diskState) {
			d.Quotes["q"] = RecoveryQuote{ID: "q", DepositID: "dep-1", ExpiresAt: pruneNow + 10}
		},
	} {
		s := prunedLedger(t, edit)
		got, err := s.PrunableGroups(pruneNow, pruneDepth, depthOf(pruneDepth))
		if err != nil || !reflect.DeepEqual(got, []string{pruneOther}) {
			t.Fatalf("%s: prunable = %v, %v", name, got, err)
		}
	}
}

func TestPrunableGroupsIgnoresClosedPaperwork(t *testing.T) {
	s := prunedLedger(t, func(d *diskState) {
		d.Settlements["old"] = Settlement{ID: "old", Scope: pruneScope, Table: "table-1", State: "expired"}
		d.Settlements["no"] = Settlement{ID: "no", Scope: pruneScope, Table: "table-1", State: "rejected"}
		d.Previews["pv"] = PaymentPreview{DepositID: "dep-1", RequestedAt: pruneNow - 20, ExpiresAt: pruneNow}
		d.Quotes["used"] = RecoveryQuote{ID: "used", DepositID: "dep-1", ExpiresAt: pruneNow + 10, TxID: pruneSpend}
		d.Quotes["stale"] = RecoveryQuote{ID: "stale", DepositID: "dep-1", ExpiresAt: pruneNow}
		d.Operations["elsewhere"] = Operation{ID: "elsewhere", Scope: pruneScope, Kind: "recovery", DepositIDs: []string{"dep-x"}, Inputs: []string{strings.Repeat("11", 32) + ":0"}, State: "publishing"}
		key, _ := scopeKey(pruneScope, "table-0")
		d.Tables[key] = TableAuthorization{Scope: pruneScope, Table: "table-0"}
		d.Deposits["dep-0"] = Deposit{ID: "dep-0", Scope: pruneScope, Terms: Terms{Table: "table-0"}, State: "spent", SpendingTx: pruneSpend}
	})
	got, err := s.PrunableGroups(pruneNow, pruneDepth, depthOf(pruneDepth))
	if err != nil || !reflect.DeepEqual(got, []string{pruneOther, pruneGroup}) {
		t.Fatalf("prunable = %v, %v", got, err)
	}
}
