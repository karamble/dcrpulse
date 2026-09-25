package gamingfunds

import (
	"decred.org/dcrwallet/v5/wallet/txrules"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
	"github.com/karamble/dcrgaming-sdk/pkg/finance"
)

func private(n byte) *secp256k1.PrivateKey {
	b := make([]byte, 32)
	b[31] = n
	return secp256k1.PrivKeyFromBytes(b)
}
func payoutStore(t *testing.T) (*Store, Scope, finance.Payout) {
	t.Helper()
	s, scope, terms := testStore(t)
	// Replace only the test fixture's destinations with real simnet addresses.
	s.mu.Lock()
	d, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	k, _ := scopeKey(scope, terms.Table)
	for uid, p := range d.Peers[k] {
		key, _ := hex.DecodeString(p.Key)
		a, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(stdaddr.Hash160(key), chaincfg.SimNetParams())
		if err != nil {
			t.Fatal(err)
		}
		p.Destination = a.String()
		d.Peers[k][uid] = p
	}
	d.RosterCommits[k] = map[string]string{}
	for uid := range d.Peers[k] {
		d.RosterCommits[k][uid] = rosterHash(d.Peers[k])
	}
	if err = s.save(d); err != nil {
		t.Fatal(err)
	}
	s.mu.Unlock()
	dep, err := s.Register(scope, terms, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{ValueIn: terms.Atoms + 1000, Sequence: wire.MaxTxInSequenceNum})
	pk, _ := hex.DecodeString(dep.PkScript)
	tx.AddTxOut(wire.NewTxOut(terms.Atoms, pk))
	raw, _ := tx.Bytes()
	now := time.Now()
	if err = s.SavePreview("approval", scope, PaymentPreview{RequestedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), DepositID: dep.ID, Unsigned: hex.EncodeToString(raw), Reason: "fund stake"}); err != nil {
		t.Fatal(err)
	}
	tx.TxIn[0].SignatureScript = []byte{txscript.OP_TRUE}
	raw, _ = tx.Bytes()
	if err = s.CommitFunding("approval", scope, dep.ID, raw); err != nil {
		t.Fatal(err)
	}
	our := finance.Input{Terms: terms, Outpoint: wire.OutPoint{Hash: tx.TxHash(), Index: 0}}
	other := our
	other.Terms.Recovery = public(3)
	other.Outpoint.Index = 1
	p := finance.Payout{Table: terms.Table, Inputs: []finance.Input{other, our}, Payments: []finance.Payment{{Key: public(2), Atoms: 2 * terms.Atoms}}}
	return s, scope, p
}
func TestPayoutRequiresDashboardApprovalAndEveryPeer(t *testing.T) {
	s, scope, proposal := payoutStore(t)
	p, err := s.ProposeSettlement(scope, proposal, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	if p.State != "awaiting_approval" || len(p.Signatures) != 0 {
		t.Fatal("proposal authorized money")
	}
	if p.FeeAtoms <= 0 || len(p.Payments) != 1 || p.Payments[0].Atoms != 2_000_000-p.FeeAtoms {
		t.Fatal("bridge did not derive and persist exact payout fee")
	}
	raw, _ := hex.DecodeString(p.Raw)
	tx, err := finance.DecodeTransaction(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ApprovedBroadcast(scope, raw); err == nil {
		t.Fatal("unapproved transaction broadcast")
	}
	signatures := make([][]byte, len(p.Inputs))
	for i, input := range p.Inputs {
		hash, err := finance.SignatureHash(tx, i, input)
		if err != nil {
			t.Fatal(err)
		}
		signatures[i] = ecdsa.Sign(private(3), hash).Serialize()
	}
	if _, err = s.AddSettlementSignatures(scope, p.ID, fmt.Sprintf("%064x", 99), signatures, chaincfg.SimNetParams()); err == nil {
		t.Fatal("forged sender accepted")
	}
	ready, err := s.AddSettlementSignatures(scope, p.ID, fmt.Sprintf("%064x", 3), signatures, chaincfg.SimNetParams())
	if err != nil || ready {
		t.Fatalf("peer signature bypassed local approval: %v", err)
	}
	sigs, err := s.ApproveSettlement(scope, p.ID, func(key WalletKey, hash []byte) ([]byte, error) { return ecdsa.Sign(private(2), hash).Serialize(), nil })
	if err != nil {
		t.Fatal(err)
	}
	ready, err = s.AddSettlementSignatures(scope, p.ID, fmt.Sprintf("%064x", 2), sigs, chaincfg.SimNetParams())
	if err != nil || !ready {
		t.Fatalf("approved payout not assembled: %v", err)
	}
	s.mu.Lock()
	d, err := s.load()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	assembled, _ := hex.DecodeString(d.Operations[p.ID].Raw)
	if err = s.ApprovedBroadcast(scope, assembled); err != nil {
		t.Fatal(err)
	}
	signed, err := finance.DecodeTransaction(assembled)
	if err != nil {
		t.Fatal(err)
	}
	if err = finance.VerifySpend(signed, p.Inputs, chaincfg.SimNetParams()); err != nil {
		t.Fatal(err)
	}
	// BR delivery is asynchronous and may repeat after chain confirmation.
	block := strings.Repeat("a", 64)
	if err = s.ObserveOperation(p.ID, ChainObservation{Known: true, Confirmations: 3, BlockHash: block, Height: 101}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddSettlementSignatures(scope, p.ID, fmt.Sprintf("%064x", 3), signatures, chaincfg.SimNetParams()); err != nil {
		t.Fatal(err)
	}
	confirmed, err := s.Settlement(scope, p.ID)
	if err != nil || confirmed.State != "confirmed" {
		t.Fatalf("duplicate signature reset confirmation: %+v %v", confirmed, err)
	}
	// A reorg is an actual node observation and may reverse confirmation.
	if err = s.ObserveOperation(p.ID, ChainObservation{Known: true}); err != nil {
		t.Fatal(err)
	}
	pending, err := s.Settlement(scope, p.ID)
	if err != nil || pending.State != "publishing" {
		t.Fatalf("reorg not recorded: %+v %v", pending, err)
	}
	deposits, err := s.AllDeposits()
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range deposits {
		if dep.SpendingTx == p.ID || dep.State == "spent" {
			t.Fatal("reorg left a deposit marked spent")
		}
	}
	// Retries preserve the exact durable transaction bytes.
	ops, err := s.Operations()
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if op.ID == p.ID && op.Raw != hex.EncodeToString(assembled) {
			t.Fatal("retry changed signed transaction")
		}
	}
	signed.TxOut[0].Value--
	changed, _ := signed.Bytes()
	if err = s.ApprovedBroadcast(scope, changed); err == nil {
		t.Fatal("altered payout accepted")
	}
}
func TestStalledPayoutDoesNotPreventOwnerRefund(t *testing.T) {
	s, scope, proposal := payoutStore(t)
	p, err := s.ProposeSettlement(scope, proposal, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	sign := func(key WalletKey, hash []byte) ([]byte, error) { return ecdsa.Sign(private(2), hash).Serialize(), nil }
	if _, err = s.ApproveSettlement(scope, p.ID, sign); err != nil {
		t.Fatal(err)
	}
	if err = s.CloseTable(scope, proposal.Table); err != nil {
		t.Fatal(err)
	}
	deposits, err := s.Deposits(scope)
	if err != nil {
		t.Fatal(err)
	}
	quote, err := s.QuoteRecovery(scope, deposits[0].ID, p.Destinations[public(2)], chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.ApproveRecovery(scope, quote.ID, chaincfg.SimNetParams(), sign)
	if err != nil {
		t.Fatal(err)
	}
	// The fee is dcrwallet's relay fee for the refund.
	// It is priced for the largest signature, at most two bytes over this one.
	signed := int64(txrules.FeeForSerializeSize(txrules.DefaultRelayFeePerKb, len(raw)))
	largest := int64(txrules.FeeForSerializeSize(txrules.DefaultRelayFeePerKb, len(raw)+2))
	if quote.FeeAtoms < signed || quote.FeeAtoms > largest {
		t.Fatalf("recovery fee %d, relay fee for the %d-byte signed refund is %d", quote.FeeAtoms, len(raw), signed)
	}
	if quote.ReturnAtoms != deposits[0].Terms.Atoms-quote.FeeAtoms {
		t.Fatalf("return %d does not deduct the fee from %d", quote.ReturnAtoms, deposits[0].Terms.Atoms)
	}
	if err = s.ApprovedBroadcast(scope, raw); err != nil {
		t.Fatal(err)
	}
	retry, err := s.ApproveRecovery(scope, quote.ID, chaincfg.SimNetParams(), nil)
	if err != nil || hex.EncodeToString(retry) != hex.EncodeToString(raw) {
		t.Fatalf("refund retry changed: %v", err)
	}
}

func TestExpiredPayoutCanBeProposedAgainWithoutChangingIntent(t *testing.T) {
	s, scope, proposal := payoutStore(t)
	p, err := s.ProposeSettlement(scope, proposal, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	d, err := s.load()
	if err == nil {
		row := d.Settlements[p.ID]
		row.ExpiresAt = time.Now().Add(-time.Second).Unix()
		d.Settlements[p.ID] = row
		err = s.save(d)
	}
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	expired, err := s.Settlement(scope, p.ID)
	if err != nil || expired.State != "expired" {
		t.Fatalf("expired proposal is %+v: %v", expired, err)
	}
	retry, err := s.ProposeSettlement(scope, proposal, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	if retry.ID != p.ID || retry.Raw != p.Raw || retry.State != "awaiting_approval" || retry.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("retry changed intent or did not reopen approval: %+v", retry)
	}
}

func TestRejectedPayoutCannotReopenOrCollectSignatures(t *testing.T) {
	s, scope, proposal := payoutStore(t)
	p, err := s.ProposeSettlement(scope, proposal, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := s.RejectSettlement(scope, p.ID)
	if err != nil || rejected.State != "rejected" {
		t.Fatalf("reject: %+v %v", rejected, err)
	}
	retry, err := s.ProposeSettlement(scope, proposal, chaincfg.SimNetParams())
	if err != nil || retry.State != "rejected" {
		t.Fatalf("identical retry reopened rejection: %+v %v", retry, err)
	}
	called := false
	if _, err = s.ApproveSettlement(scope, p.ID, func(WalletKey, []byte) ([]byte, error) {
		called = true
		return nil, nil
	}); err == nil {
		t.Fatal("rejected payout was approved")
	}
	if called {
		t.Fatal("rejected payout reached the wallet signer")
	}
	if _, err = s.AddSettlementSignatures(scope, p.ID, fmt.Sprintf("%064x", 3), make([][]byte, len(p.Inputs)), chaincfg.SimNetParams()); err == nil {
		t.Fatal("rejected payout accepted peer signatures")
	}
}

func TestMalformedChainObservationsDoNotChangeLedger(t *testing.T) {
	s, scope, proposal := payoutStore(t)
	p, err := s.ProposeSettlement(scope, proposal, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	for _, facts := range []ChainObservation{
		{Known: true, Confirmations: 1},
		{Known: true, Confirmations: 1, Height: 100, BlockHash: "nothex"},
		{Known: false, Height: 100},
		{Known: true, Confirmations: -1},
		{Known: true, BlockHash: strings.Repeat("a", 64)},
	} {
		if err := s.ObserveOperation(p.ID, facts); err == nil {
			t.Fatal("invalid chain observation accepted")
		}
	}
	got, err := s.Settlement(scope, p.ID)
	if err != nil || got.State != "awaiting_approval" {
		t.Fatalf("malformed observation changed payout: %+v %v", got, err)
	}
}

func TestClosedTableDoesNotAssembleLatePayoutSignatures(t *testing.T) {
	s, scope, proposal := payoutStore(t)
	p, err := s.ProposeSettlement(scope, proposal, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApproveSettlement(scope, p.ID, func(key WalletKey, hash []byte) ([]byte, error) { return ecdsa.Sign(private(2), hash).Serialize(), nil }); err != nil {
		t.Fatal(err)
	}
	if err = s.CloseTable(scope, p.Table); err != nil {
		t.Fatal(err)
	}
	raw, _ := hex.DecodeString(p.Raw)
	tx, err := finance.DecodeTransaction(raw)
	if err != nil {
		t.Fatal(err)
	}
	sigs := make([][]byte, len(p.Inputs))
	for i, input := range p.Inputs {
		hash, err := finance.SignatureHash(tx, i, input)
		if err != nil {
			t.Fatal(err)
		}
		sigs[i] = ecdsa.Sign(private(3), hash).Serialize()
	}
	if _, err = s.AddSettlementSignatures(scope, p.ID, fmt.Sprintf("%064x", 3), sigs, chaincfg.SimNetParams()); err == nil {
		t.Fatal("closed table assembled a new payout")
	}
	// Closure cannot revoke signatures that were already released to a peer.
	got, err := s.Settlement(scope, p.ID)
	if err != nil || len(got.Signatures[public(2)]) != len(p.Inputs) {
		t.Fatal("released signature record lost")
	}
}
