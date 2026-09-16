package gamingfunds

import (
	"reflect"
	"strings"
	"testing"
)

func TestExternalSpendAndReorgUpdateDepositAuthority(t *testing.T) {
	s, scope, _ := payoutStore(t)
	deps, err := s.Deposits(scope)
	if err != nil || len(deps) != 1 {
		t.Fatalf("deposits: %+v %v", deps, err)
	}
	dep := deps[0]
	spend := strings.Repeat("ab", 32)
	if err = s.ObserveDeposit(scope, dep.ID, DepositObservation{SpendingTx: spend}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Deposits(scope)
	if got[0].State != "spend_pending" || got[0].SpendingTx != spend {
		t.Fatalf("pending external spend recorded as %+v", got[0])
	}
	if err = s.ObserveDeposit(scope, dep.ID, DepositObservation{SpendingTx: spend, SpendConfirmations: 2}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Deposits(scope)
	if got[0].State != "spent" {
		t.Fatalf("confirmed external spend recorded as %+v", got[0])
	}
	block := strings.Repeat("cd", 32)
	if err = s.ObserveDeposit(scope, dep.ID, DepositObservation{OutputFound: true, Confirmations: 4, FundingBlock: block, FundingHeight: 100}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Deposits(scope)
	if got[0].State != "confirmed" || got[0].SpendingTx != "" || got[0].Confirmations != 4 {
		t.Fatalf("reorg-restored output recorded as %+v", got[0])
	}
}

func TestMalformedDepositObservationDoesNotChangeLedger(t *testing.T) {
	s, scope, _ := payoutStore(t)
	deps, err := s.Deposits(scope)
	if err != nil || len(deps) != 1 {
		t.Fatalf("deposits: %+v %v", deps, err)
	}
	before := deps[0]
	for _, facts := range []DepositObservation{
		{OutputFound: true, SpendingTx: strings.Repeat("ab", 32)},
		{OutputFound: true, Confirmations: 1},
		{SpendingTx: "not-a-txid"},
		{SpendConfirmations: -1},
	} {
		if err = s.ObserveDeposit(scope, before.ID, facts); err == nil {
			t.Fatalf("accepted malformed observation %+v", facts)
		}
	}
	after, _ := s.Deposits(scope)
	if !reflect.DeepEqual(after[0], before) {
		t.Fatalf("malformed observations changed deposit\n before %+v\n after  %+v", before, after[0])
	}
}
