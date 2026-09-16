package gamingfunds

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/wire"
)

func fundingPreview(t *testing.T) (*Store, Scope, Deposit, *wire.MsgTx, PaymentPreview) {
	t.Helper()
	s, scope, terms := testStore(t)
	dep, err := s.Register(scope, terms, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{ValueIn: terms.Atoms + 1000, Sequence: wire.MaxTxInSequenceNum})
	pk, err := hex.DecodeString(dep.PkScript)
	if err != nil {
		t.Fatal(err)
	}
	tx.AddTxOut(wire.NewTxOut(terms.Atoms, pk))
	raw, err := tx.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	p := PaymentPreview{RequestedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), DepositID: dep.ID, Unsigned: hex.EncodeToString(raw), Reason: "fund stake", FeeAtoms: 1000}
	if err = s.SavePreview("approval", scope, p); err != nil {
		t.Fatal(err)
	}
	return s, scope, dep, tx, p
}

func TestFundingApprovalRetryRecoversOriginalRequest(t *testing.T) {
	s, scope, dep, tx, first := fundingPreview(t)
	tx.TxIn[0].PreviousOutPoint.Index++
	raw, err := tx.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	retry := first
	retry.Unsigned = hex.EncodeToString(raw)
	retry.Reason = "hostile replacement"
	got, err := s.EnsurePreview("replacement", scope, retry)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "approval" || got.Deposit.ID != dep.ID || got.Preview != first {
		t.Fatalf("retry returned %+v, want original approval", got)
	}
	approvals, err := s.FundingApprovals()
	if err != nil {
		t.Fatal(err)
	}
	if len(approvals) != 1 || approvals[0].ID != "approval" {
		t.Fatalf("authority holds %+v", approvals)
	}
}

func TestFundingApprovalSurvivesAuthorityRestart(t *testing.T) {
	s, _, dep, _, preview := fundingPreview(t)
	dir := s.dir
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	approvals, err := restored.FundingApprovals()
	if err != nil {
		t.Fatal(err)
	}
	if len(approvals) != 1 || approvals[0].ID != "approval" || approvals[0].Deposit.ID != dep.ID || approvals[0].Preview != preview {
		t.Fatalf("restart recovered %+v", approvals)
	}
}

func TestDepositCannotHaveTwoPendingFundingApprovals(t *testing.T) {
	s, scope, _, tx, p := fundingPreview(t)
	// A different wallet input must not permit a second approval for the same deposit.
	tx.TxIn[0].PreviousOutPoint.Index++
	raw, err := tx.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	p.Unsigned = hex.EncodeToString(raw)
	if err = s.SavePreview("second", scope, p); err == nil {
		t.Fatal("duplicate approval accepted")
	}
	if _, err = s.Preview("second", p.DepositID); err == nil {
		t.Fatal("rejected preview persisted")
	}
}

func TestFundingCommitRejectsTrailingBytes(t *testing.T) {
	s, scope, dep, tx, _ := fundingPreview(t)
	tx.TxIn[0].SignatureScript = []byte{0x51}
	raw, err := tx.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CommitFunding("approval", scope, dep.ID, append(raw, 0)); err == nil {
		t.Fatal("trailing funding bytes accepted")
	}
	deps, err := s.Deposits(scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range deps {
		if d.FundingTx != "" {
			t.Fatal("rejected funding was reserved")
		}
	}
	if err = s.CommitFunding("approval", scope, dep.ID, raw); err != nil {
		t.Fatal(err)
	}
}
