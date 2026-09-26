package services

import (
	"encoding/json"
	"strings"
	"testing"

	"dcrpulse/internal/gamingfunds"

	"github.com/karamble/dcrgaming-sdk/pkg/finance"
)

func stakeInput(owner string, atoms int64) finance.Input {
	return finance.Input{Terms: finance.Terms{Kind: "stake", Recovery: owner, Atoms: atoms}}
}

func TestPayoutShareFindsTheOperatorsStakeAndPayment(t *testing.T) {
	key := gamingfunds.WalletKey{Public: "02aa", Address: "DsOurs"}
	p := gamingfunds.Settlement{
		Inputs:   []finance.Input{stakeInput("02aa", 100000000), stakeInput("02bb", 100000000)},
		Payments: []finance.Payment{{Key: "02aa", Atoms: 199990000}},
		// The destination of the other owner, never used to pick ours.
		Destinations: map[string]string{"02aa": "DsOurs", "02bb": "DsTheirs"},
	}
	share, ok := payoutShare(p, key)
	if !ok || share != (GamingPayoutShare{Key: "02aa", Address: "DsOurs", StakeAtoms: 100000000, ReceiveAtoms: 199990000}) {
		t.Fatalf("win: %+v, %v", share, ok)
	}

	p.Payments = []finance.Payment{{Key: "02bb", Atoms: 199990000}}
	share, ok = payoutShare(p, key)
	if !ok || share.StakeAtoms != 100000000 || share.ReceiveAtoms != 0 {
		t.Fatalf("loss: %+v, %v", share, ok)
	}

	if _, ok = payoutShare(p, gamingfunds.WalletKey{Public: "02cc"}); ok {
		t.Fatal("a table without our stake reported a share")
	}
}

func TestPayoutViewKeepsTheSettlementFieldsAndAddsMine(t *testing.T) {
	view := GamingPayoutView{Settlement: gamingfunds.Settlement{ID: "tx1", Table: "t"}, Mine: &GamingPayoutShare{Key: "02aa", StakeAtoms: 5}}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id":"tx1"`, `"table":"t"`, `"mine":{"key":"02aa","address":"","stakeAtoms":5,"receiveAtoms":0}`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("%s missing from %s", want, raw)
		}
	}
}
