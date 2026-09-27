package gamingcore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"dcrpulse/internal/gamingfunds"
)

func TestFundingIsDeadOnlyWhenAnotherSpendIsDeepEnough(t *testing.T) {
	op := strings.Repeat("0f", 32)
	other := strings.Repeat("e1", 32)
	inputs := []string{"aa:0", "bb:1"}
	for _, tc := range []struct {
		name   string
		spends map[string]observedGamingSpend
		want   string
	}{
		{"no spend found", map[string]observedGamingSpend{}, ""},
		{"spent by the funding itself", map[string]observedGamingSpend{"aa:0": {txid: op, confirmations: 9}}, ""},
		{"other spend one block deep", map[string]observedGamingSpend{"bb:1": {txid: other, confirmations: 1}}, ""},
		{"other spend two blocks deep", map[string]observedGamingSpend{"bb:1": {txid: other, confirmations: 2}}, other},
		{"conflicting spends", map[string]observedGamingSpend{"aa:0": {txid: other, confirmations: 5, conflict: true}}, ""},
	} {
		if got := fundingSpender(op, inputs, tc.spends); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestOnlyARejectedBroadcastIsCheckedOnChain(t *testing.T) {
	for _, err := range []error{status.Error(codes.Unavailable, "down"), status.Error(codes.DeadlineExceeded, "slow"), status.Error(codes.Canceled, "gone")} {
		if !transientBroadcastError(err) {
			t.Errorf("%v counted as a rejection", err)
		}
	}
	for _, err := range []error{status.Error(codes.Unknown, "transaction already spent"), status.Error(codes.InvalidArgument, "rejected"), errors.New("wallet gRPC client not initialized")} {
		if transientBroadcastError(err) {
			t.Errorf("%v counted as no answer", err)
		}
	}
}

func TestAFailedRequestWithAReleasedDepositCanBeAskedAgain(t *testing.T) {
	free := gamingfunds.Deposit{ID: "dep1"}
	bound := gamingfunds.Deposit{ID: "dep1", FundingTx: strings.Repeat("0f", 32)}
	for _, tc := range []struct {
		prior  GamingSpend
		dep    gamingfunds.Deposit
		stands bool
	}{
		{GamingSpend{DepositID: "dep1", State: GamingSpendFailed}, free, false},
		{GamingSpend{DepositID: "dep1", State: GamingSpendFailed}, bound, true},
		{GamingSpend{DepositID: "dep1", State: GamingSpendPending}, free, true},
		{GamingSpend{DepositID: "dep1", State: GamingSpendApproved}, free, true},
		{GamingSpend{DepositID: "dep2", State: GamingSpendPending}, free, false},
	} {
		if got := fundingSpendStands(tc.prior, tc.dep); got != tc.stands {
			t.Errorf("prior %s for %s with funding %q: stands=%v", tc.prior.State, tc.prior.DepositID, tc.dep.FundingTx, got)
		}
	}
}

func fundingLedger(t *testing.T, br *Bridge, tableClosed, depositClosed bool, table string) (*gamingfunds.Store, gamingfunds.Operation) {
	t.Helper()
	br.dataDir = t.TempDir()
	t.Cleanup(func() {
		br.financeStores.Lock()
		path := filepath.Join(br.dataDir, "financial-authority")
		if s := br.financeStores.stores[path]; s != nil {
			s.Close()
			delete(br.financeStores.stores, path)
		}
		br.financeStores.Unlock()
	})
	scope := gamingfunds.Scope{Game: "poker", Network: "mainnet", Wallet: "fp"}
	key, _ := json.Marshal(struct {
		Scope gamingfunds.Scope
		Table string
	}{scope, "0123456789abcdef"})
	opID := strings.Repeat("0f", 32)
	ledger := map[string]any{
		"version": gamingfunds.Version, "rosterCommits": map[string]any{}, "peers": map[string]any{}, "keys": map[string]any{},
		"previews": map[string]any{}, "quotes": map[string]any{}, "settlements": map[string]any{},
		"tables":     map[string]any{string(key): map[string]any{"scope": scope, "table": "0123456789abcdef", "group": pruneGCA, "seats": 2, "until": 1, "closed": tableClosed}},
		"deposits":   map[string]any{"dep1": map[string]any{"id": "dep1", "scope": scope, "terms": map[string]any{"table": table, "kind": "stake"}, "fundingTx": opID, "closed": depositClosed, "state": "publishing"}},
		"operations": map[string]any{opID: map[string]any{"id": opID, "scope": scope, "kind": "funding", "depositIDs": []string{"dep1"}, "inputs": []string{"aa:0"}, "raw": "00", "approved": true, "state": "publishing"}},
	}
	raw, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(br.dataDir, "financial-authority")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "authority.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := br.gamingFundsStore()
	if err != nil {
		t.Fatal(err)
	}
	ops, err := store.Operations()
	if err != nil || len(ops) != 1 {
		t.Fatalf("operations = %+v %v", ops, err)
	}
	return store, ops[0]
}

func TestFundingOfAClosedTableIsNotBroadcastAgain(t *testing.T) {
	br := newTestBridge(t)
	for _, tc := range []struct {
		name                       string
		tableClosed, depositClosed bool
		table                      string
		wanted                     bool
	}{
		{"open table", false, false, "0123456789abcdef", true},
		{"closed table", true, false, "0123456789abcdef", false},
		{"closed deposit", false, true, "0123456789abcdef", false},
		{"identity deposit", false, false, "", true},
	} {
		store, op := fundingLedger(t, br, tc.tableClosed, tc.depositClosed, tc.table)
		if got := fundingStillWanted(store, op); got != tc.wanted {
			t.Errorf("%s: wanted=%v", tc.name, got)
		}
	}
}

func TestDeadFundingFailsOnlyItsOwnRequest(t *testing.T) {
	br := newTestBridge(t)
	spendSeams(t, br)
	op := gamingfunds.Operation{ID: strings.Repeat("0f", 32), DepositIDs: []string{"dep1"}}
	now := time.Now().Unix()
	if err := br.writeSpendLog(spendLog{Spends: []GamingSpend{
		{ID: "a1", Game: "poker", DepositID: "dep1", State: GamingSpendPublishing, AmountAtoms: 5},
		{ID: "b2", Game: "poker", DepositID: "dep2", State: GamingSpendPublishing, AmountAtoms: 5},
		{ID: "c3", Game: "poker", DepositID: "dep1", State: GamingSpendApproved, TxID: strings.Repeat("aa", 32), AmountAtoms: 5, DecidedAt: now},
	}}, now); err != nil {
		t.Fatal(err)
	}
	br.failAbandonedFundingSpend(op, strings.Repeat("e1", 32))
	got := mustReadSpendLog(t, br).Spends
	if got[0].State != GamingSpendFailed || !strings.Contains(got[0].Error, strings.Repeat("e1", 32)) {
		t.Fatalf("the dead funding's request = %+v", got[0])
	}
	if got[1].State != GamingSpendPublishing || got[2].State != GamingSpendApproved {
		t.Fatalf("other requests changed: %+v", got[1:])
	}
}
