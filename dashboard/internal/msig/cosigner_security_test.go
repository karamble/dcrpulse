package msig

import (
	"bytes"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/sign"
)

func TestActiveWalletIgnoresLateHandshakeFailure(t *testing.T) {
	for _, kind := range []string{TypeDecline, TypeReady} {
		t.Run(kind, func(t *testing.T) {
			sh, id := newSpendHarness(t, 2, "alice", "bob")
			sh.as("alice")
			s := sh.store("alice")
			before := sh.record("alice", id)
			peer := before.Peers[0]
			msg := &Message{Type: kind, WalletID: before.Address, TempID: id, Reason: "late decline"}
			if kind == TypeDecline {
				inboundDecline(s, before, msg, peer.UID, peer.Nick)
			} else {
				inboundReady(s, before, msg, peer.UID, time.Now())
			}
			after, _ := s.Wallet(id)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("late %s changed active wallet: status %s -> %s", kind, before.Status, after.Status)
			}
			reopened, err := openStore(s.path, s.walletName)
			if err != nil {
				t.Fatal(err)
			}
			persisted, _ := reopened.Wallet(id)
			if persisted.Status != StatusActive {
				t.Fatalf("persisted status: %s", persisted.Status)
			}
		})
	}
}

func TestCompletedSignatureStackMustExecute(t *testing.T) {
	w := newTestWallet(t, 2, 3)
	tx := w.spendTx(w.fundingTx(100_000_000), 99_990_000, 0)
	w.signInput(tx, 0, 0)
	w.signInput(tx, 0, 1)
	pushes, err := sigPushes(tx.TxIn[0].SignatureScript, w.redeem)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.engineErr(tx, 0); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"reverse", "leading-empty", "trailing-empty", "duplicate"} {
		t.Run(variant, func(t *testing.T) {
			bad := tx.Copy()
			b := txscript.NewScriptBuilder()
			switch variant {
			case "reverse":
				b.AddData(pushes[1]).AddData(pushes[0])
			case "leading-empty":
				b.AddOp(txscript.OP_0).AddData(pushes[0]).AddData(pushes[1])
			case "trailing-empty":
				b.AddData(pushes[0]).AddData(pushes[1]).AddOp(txscript.OP_0)
			case "duplicate":
				b.AddData(pushes[0]).AddData(pushes[0]).AddData(pushes[1])
			}
			bad.TxIn[0].SignatureScript, err = b.AddData(w.redeem).Script()
			if err != nil {
				t.Fatal(err)
			}
			if bad.TxHash() != tx.TxHash() {
				t.Fatal("test changed transaction prefix")
			}
			if bytes.Equal(bad.TxIn[0].SignatureScript, tx.TxIn[0].SignatureScript) {
				t.Fatal("test did not corrupt script")
			}
			if err := w.engineErr(bad, 0); err == nil {
				t.Fatal("test script unexpectedly executes")
			}
			if _, err := VerifyProposalUpdateHD(bad, tx.TxHash().String(), w.stubResolver()); err == nil {
				t.Fatal("accepted individually valid signatures in an unexecutable stack")
			}
		})
	}
}

func TestInvalidPersistedProposalRetainsInputsAndAcceptsCorrection(t *testing.T) {
	sh, id := newSpendHarness(t, 2, "alice", "bob", "carol")
	sh.as("alice")
	rec := sh.record("alice", id)
	s := sh.store("alice")
	sh.fund(rec.Address, 500_000_000, 0)
	prop, err := ProposeSpend(sh.ctx, id, []Recipient{{Address: rec.Address, Atoms: 100_000_000}}, false, []string{sh.nodeByNick("bob").uid}, "", 0, []byte("pass"))
	if err != nil {
		t.Fatal(err)
	}
	good, err := sh.signAs(t, sh.nodeByNick("bob"), prop.RawTx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := DecodeTxHex(good)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := entryAt(rec, BranchExternal, 0)
	if err != nil {
		t.Fatal(err)
	}
	pushes, err := sigPushes(tx.TxIn[0].SignatureScript, entry.Script)
	if err != nil {
		t.Fatal(err)
	}
	tx.TxIn[0].SignatureScript, err = txscript.NewScriptBuilder().AddData(pushes[0]).AddData(pushes[0]).AddData(pushes[1]).AddData(entry.Script).Script()
	if err != nil {
		t.Fatal(err)
	}
	bad, err := tx.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateProposal(id, prop.TxID, false, func(_ *WalletRecord, p *Proposal) error {
		p.Status = ProposalReady
		p.RawTx = hex.EncodeToString(bad)
		p.SigCount = 3
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := RebroadcastProposal(sh.ctx, id, prop.TxID); err == nil {
		t.Fatal("manual broadcast reported success for invalid stack")
	}
	r, p, _ := s.Proposal(id, prop.TxID)
	if p.Status != ProposalInvalid || len(sh.broadcasts) != 0 || !p.Live() || len(lockedInputs(r, "")) != len(p.Inputs) {
		t.Fatal("invalid stack broadcast or released reservation")
	}
	persisted := reopenHints(t, s)
	_, saved, _ := persisted.Proposal(id, prop.TxID)
	if saved.Status != ProposalInvalid {
		t.Fatal("invalid status not persisted")
	}
	// A partial correction must not release the invalid state or its lock.
	inboundSig(s, r, &Message{TxID: prop.TxID, RawTx: prop.RawTx}, sh.nodeByNick("bob").uid)
	_, p, _ = s.Proposal(id, prop.TxID)
	if p.Status != ProposalInvalid {
		t.Fatal("partial correction escaped invalid state")
	}
	// A fresh executable threshold wins, even below the poisoned old count.
	inboundSig(s, r, &Message{TxID: prop.TxID, RawTx: good}, sh.nodeByNick("bob").uid)
	_, p, _ = s.Proposal(id, prop.TxID)
	if p.Status != ProposalBroadcast || p.SigCount != 2 || len(sh.broadcasts) != 1 {
		t.Fatalf("correction did not recover: %s/%d", p.Status, p.SigCount)
	}
}

func TestLateReadyAndDeclineUseCurrentHandshakePhase(t *testing.T) {
	sh, id := newSpendHarness(t, 2, "alice", "bob")
	sh.as("alice")
	s := sh.store("alice")
	active := sh.record("alice", id)
	stale := cloneRecord(active)
	stale.Status = StatusActivating
	peer := active.Peers[0]
	inboundDecline(s, stale, &Message{Reason: "delayed"}, peer.UID, peer.Nick)
	inboundReady(s, stale, &Message{WalletID: active.Address, Attest: "invalid"}, peer.UID, time.Now())
	after, _ := s.Wallet(id)
	if !reflect.DeepEqual(active, after) {
		t.Fatal("stale snapshot downgraded active record")
	}
	// A verified ready can still be redelivered after activation.
	inboundReady(s, active, &Message{WalletID: active.Address, Attest: peer.AttestSig}, peer.UID, time.Now())
	after, _ = s.Wallet(id)
	if after.Status != StatusActive || after.Peers[0].AttestSig != peer.AttestSig {
		t.Fatal("late valid ready broke active state")
	}
	// A bad ready really does fail a round still awaiting activation.
	if err := s.UpdateWallet(id, func(r *WalletRecord) error { r.Status = StatusActivating; return nil }); err != nil {
		t.Fatal(err)
	}
	activating, _ := s.Wallet(id)
	inboundReady(s, activating, &Message{WalletID: active.Address}, peer.UID, time.Now())
	failed, _ := s.Wallet(id)
	if failed.Status != StatusFailed {
		t.Fatal("invalid activation was accepted")
	}
	inboundReady(s, activating, &Message{WalletID: active.Address, Attest: peer.AttestSig}, peer.UID, time.Now())
	failed, _ = s.Wallet(id)
	if failed.Status != StatusFailed {
		t.Fatal("late ready revived failed round")
	}
}

func TestSignatureEncodingAndSupportedHashTypes(t *testing.T) {
	w := newTestWallet(t, 2, 3)
	for _, hashType := range []txscript.SigHashType{txscript.SigHashAll, txscript.SigHashNone, txscript.SigHashSingle, txscript.SigHashAll | txscript.SigHashAnyOneCanPay, txscript.SigHashNone | txscript.SigHashAnyOneCanPay, txscript.SigHashSingle | txscript.SigHashAnyOneCanPay} {
		t.Run(fmt.Sprint(hashType), func(t *testing.T) {
			tx := w.spendTx(w.fundingTx(100_000_000), 99_990_000, 0)
			for _, signer := range []int{0, 1} {
				script, err := sign.SignTxOutput(w.params, tx, 0, w.pkScript, hashType, w.kdbFor(signer), w.sdb(), tx.TxIn[0].SignatureScript, false)
				if err != nil {
					t.Fatal(err)
				}
				tx.TxIn[0].SignatureScript = script
			}
			if err := w.engineErr(tx, 0); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyProposalUpdateHD(tx, tx.TxHash().String(), w.stubResolver()); err != nil {
				t.Fatalf("supported hash rejected: %v", err)
			}
		})
	}
	tx := w.spendTx(w.fundingTx(100_000_000), 99_990_000, 0)
	w.signInput(tx, 0, 0)
	pushes, err := sigPushes(tx.TxIn[0].SignatureScript, w.redeem)
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"high-S", "unsupported-hash", "trailing-DER"} {
		t.Run(variant, func(t *testing.T) {
			sig := append([]byte(nil), pushes[0]...)
			switch variant {
			case "high-S":
				var pair struct{ R, S *big.Int }
				if _, err := asn1.Unmarshal(sig[:len(sig)-1], &pair); err != nil {
					t.Fatal(err)
				}
				order, ok := new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
				if !ok {
					t.Fatal("order")
				}
				pair.S.Sub(order, pair.S)
				der, err := asn1.Marshal(pair)
				if err != nil {
					t.Fatal(err)
				}
				sig = append(der, sig[len(sig)-1])
			case "unsupported-hash":
				sig[len(sig)-1] = 0x04
			case "trailing-DER":
				sig = append(sig[:len(sig)-1], 0, byte(txscript.SigHashAll))
			}
			bad := tx.Copy()
			bad.TxIn[0].SignatureScript, err = txscript.NewScriptBuilder().AddData(sig).AddData(w.redeem).Script()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyProposalUpdateHD(bad, tx.TxHash().String(), w.stubResolver()); err == nil {
				t.Fatal("accepted malformed partial encoding")
			}
		})
	}
}

func TestEveryCompletedInputExecutesBeforeCommonThreshold(t *testing.T) {
	w := newTestWallet(t, 2, 3)
	tx := w.spendTx(w.fundingTx(50_000_000, 50_000_000), 99_990_000, 0, 1)
	w.signInput(tx, 0, 0)
	w.signInput(tx, 0, 1)
	w.signInput(tx, 1, 1)
	w.signInput(tx, 1, 2)
	// Each input executes, but the common participant set has only one member.
	common, err := VerifyProposalUpdateHD(tx, tx.TxHash().String(), w.stubResolver())
	if err != nil || len(common) != 1 {
		t.Fatalf("mixed participants: %v %v", common, err)
	}
	pushes, err := sigPushes(tx.TxIn[1].SignatureScript, w.redeem)
	if err != nil {
		t.Fatal(err)
	}
	tx.TxIn[1].SignatureScript, err = txscript.NewScriptBuilder().AddData(pushes[1]).AddData(pushes[0]).AddData(w.redeem).Script()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyProposalUpdateHD(tx, tx.TxHash().String(), w.stubResolver()); err == nil {
		t.Fatal("deferred per-input execution until common threshold")
	}
}

func TestStaleRosterCannotDowngradeActiveWallet(t *testing.T) {
	sh, id := newSpendHarness(t, 2, "alice", "bob", "carol")
	sh.as("bob")
	s := sh.store("bob")
	active := sh.record("bob", id)
	stale := cloneRecord(active)
	stale.Status = StatusPendingImport
	msg := rosterMessage(sh.record("alice", id))
	msg.Xpubs = swapLastXpub(t, active.Xpubs, "mallory")
	inboundRosterHD(sh.ctx, s, stale, msg, active.InitiatorUID)
	after, _ := s.Wallet(id)
	if !reflect.DeepEqual(active, after) {
		t.Fatalf("stale conflicting roster changed active wallet: %s", after.Status)
	}
	stale.Status = StatusAccepted
	inboundRosterHD(sh.ctx, s, stale, rosterMessage(sh.record("alice", id)), active.InitiatorUID)
	inboundCancel(s, stale, active.InitiatorUID)
	after, _ = s.Wallet(id)
	if !reflect.DeepEqual(active, after) {
		t.Fatalf("stale first roster/cancellation changed active wallet: %s", after.Status)
	}
}
