// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingfunds

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/karamble/dcrgaming-sdk/pkg/finance"
)

// signedPayout is a payout both seats have signed, assembled and ready to go.
func signedPayout(t *testing.T) (*Store, Scope, Settlement) {
	t.Helper()
	s, scope, proposal := payoutStore(t)
	p, err := s.ProposeSettlement(scope, proposal, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := hex.DecodeString(p.Raw)
	tx, err := finance.DecodeTransaction(raw)
	if err != nil {
		t.Fatal(err)
	}
	signatures := make([][]byte, len(p.Inputs))
	for i, input := range p.Inputs {
		hash, err := finance.SignatureHash(tx, i, input)
		if err != nil {
			t.Fatal(err)
		}
		signatures[i] = ecdsa.Sign(private(3), hash).Serialize()
	}
	if _, err = s.AddSettlementSignatures(scope, p.ID, fmt.Sprintf("%064x", 3), signatures, chaincfg.SimNetParams()); err != nil {
		t.Fatal(err)
	}
	sigs, err := s.ApproveSettlement(scope, p.ID, func(_ WalletKey, hash []byte) ([]byte, error) { return ecdsa.Sign(private(2), hash).Serialize(), nil })
	if err != nil {
		t.Fatal(err)
	}
	if ready, err := s.AddSettlementSignatures(scope, p.ID, fmt.Sprintf("%064x", 2), sigs, chaincfg.SimNetParams()); err != nil || !ready {
		t.Fatalf("payout not assembled: %v", err)
	}
	return s, scope, p
}

// payoutDeposit is our own stake among the payout's inputs.
func payoutDeposit(t *testing.T, s *Store) Deposit {
	t.Helper()
	deps, err := s.AllDeposits()
	if err != nil || len(deps) != 1 {
		t.Fatalf("deposits = %+v, %v", deps, err)
	}
	return deps[0]
}

func observe(t *testing.T, s *Store, id string, facts ChainObservation) {
	t.Helper()
	if err := s.ObserveOperation(id, facts); err != nil {
		t.Fatal(err)
	}
}

func TestPayoutInMempoolMarksItsInputsSpendPending(t *testing.T) {
	s, _, p := signedPayout(t)
	block := strings.Repeat("a", 64)

	observe(t, s, p.ID, ChainObservation{Known: true})
	if dep := payoutDeposit(t, s); dep.State != "spend_pending" || dep.SpendingTx != p.ID {
		t.Fatalf("stake spent by our payout in the mempool = %s/%s", dep.State, dep.SpendingTx)
	}
	observe(t, s, p.ID, ChainObservation{Known: true, Confirmations: 1, BlockHash: block, Height: 101})
	if dep := payoutDeposit(t, s); dep.State != "spent" || dep.SpendingTx != p.ID {
		t.Fatalf("stake after the payout confirmed = %s/%s", dep.State, dep.SpendingTx)
	}

	// A reorg takes the confirmation back. That needs a person, and it stays
	// that way while the payout sits in the mempool again.
	for i := 0; i < 2; i++ {
		observe(t, s, p.ID, ChainObservation{Known: true})
		if dep := payoutDeposit(t, s); dep.State != "needs_attention" || dep.SpendingTx != "" {
			t.Fatalf("stake after a reorg, pass %d = %s/%s", i, dep.State, dep.SpendingTx)
		}
	}
}

func TestPayoutDroppedFromMempoolNeedsAttention(t *testing.T) {
	s, _, p := signedPayout(t)
	observe(t, s, p.ID, ChainObservation{Known: true})
	observe(t, s, p.ID, ChainObservation{})
	if dep := payoutDeposit(t, s); dep.State != "needs_attention" || dep.SpendingTx != "" {
		t.Fatalf("stake after the payout left the mempool = %s/%s", dep.State, dep.SpendingTx)
	}
}
