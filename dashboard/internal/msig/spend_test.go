// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/sign"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"

	"dcrpulse/internal/services"
)

// spendHarness extends the handshake harness with a fake chain: it holds
// the shared UTXO set and signs with each node's own key, emulating
// dcrwallet's per-wallet key custody.
type spendHarness struct {
	*hsHarness
	utxos      map[string][]UTXO // shared address -> unspent
	privByPub  map[string]*secp256k1.PrivateKey
	broadcasts []string
}

func newSpendHarness(t *testing.T, m int, names ...string) (*spendHarness, string) {
	t.Helper()
	h := newHsHarness(t, names...)
	sh := &spendHarness{
		hsHarness: h,
		utxos:     make(map[string][]UTXO),
		privByPub: make(map[string]*secp256k1.PrivateKey),
	}

	// Capture the private key behind every derived pubkey.
	for _, n := range h.nodes {
		node := n
		node.nextKey = func() *OwnKey {
			priv, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Fatalf("generate key: %v", err)
			}
			pub := hex.EncodeToString(priv.PubKey().SerializeCompressed())
			sh.privByPub[pub] = priv
			node.keyIdx++
			return &OwnKey{PubKey: pub, Address: "Ss-" + node.nick, Index: node.keyIdx - 1}
		}
	}

	origList, origSign, origBroadcast := listUTXOsSeam, signSeam, broadcastSeam
	t.Cleanup(func() {
		listUTXOsSeam, signSeam, broadcastSeam = origList, origSign, origBroadcast
	})
	listUTXOsSeam = func(ctx context.Context, address string) ([]UTXO, error) {
		return append([]UTXO(nil), sh.utxos[address]...), nil
	}
	signSeam = func(ctx context.Context, rawTxHex string, prevs []services.MsigPrevInput, account uint32, pass []byte) (string, error) {
		return sh.signAs(t, sh.current, rawTxHex)
	}
	broadcastSeam = func(ctx context.Context, signedTx []byte) (string, error) {
		tx, err := DecodeTxHex(hex.EncodeToString(signedTx))
		if err != nil {
			return "", err
		}
		txid := tx.TxHash().String()
		sh.broadcasts = append(sh.broadcasts, txid)
		sh.spend(t, tx)
		return txid, nil
	}

	// Run the handshake to an active wallet.
	cosigners := names[1:]
	tempID := h.create(t, m, names[0], cosigners...)
	h.pump()
	for _, nick := range cosigners {
		h.as(nick)
		if err := AcceptInvite(h.ctx, tempID, 0); err != nil {
			t.Fatalf("%s accept: %v", nick, err)
		}
		h.pump()
	}
	rec := h.record(names[0], tempID)
	if rec.Status != StatusActive {
		t.Fatalf("setup: wallet not active: %s (%s)", rec.Status, rec.FailReason)
	}
	return sh, tempID
}

// signAs adds the signature of whichever roster key the given node owns.
func (sh *spendHarness) signAs(t *testing.T, node *hsNode, rawTxHex string) (string, error) {
	t.Helper()
	rec := sh.recordFor(node)
	if rec == nil {
		t.Fatalf("no shared wallet record for %s", node.nick)
	}
	script, err := hex.DecodeString(rec.ScriptHex)
	if err != nil {
		return "", err
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		return "", err
	}
	addr, err := stdaddr.NewAddressScriptHashV0(script, params)
	if err != nil {
		return "", err
	}
	_, pkScript := addr.PaymentScript()
	tx, err := DecodeTxHex(rawTxHex)
	if err != nil {
		return "", err
	}
	priv, ok := sh.privByPub[rec.Own.PubKey]
	if !ok {
		t.Fatalf("no private key for %s", node.nick)
	}
	pubAddr, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(priv.PubKey(), params)
	if err != nil {
		return "", err
	}
	kdb := sign.KeyClosure(func(a stdaddr.Address) ([]byte, dcrec.SignatureType, bool, error) {
		if a.String() != pubAddr.String() {
			return nil, 0, false, errNoKey
		}
		return priv.Serialize(), dcrec.STEcdsaSecp256k1, true, nil
	})
	sdb := sign.ScriptClosure(func(stdaddr.Address) ([]byte, error) { return script, nil })
	for i := range tx.TxIn {
		sigScript, err := sign.SignTxOutput(params, tx, i, pkScript, txscript.SigHashAll,
			kdb, sdb, tx.TxIn[i].SignatureScript, false)
		if err != nil {
			return "", err
		}
		tx.TxIn[i].SignatureScript = sigScript
	}
	raw, err := tx.Bytes()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func (sh *spendHarness) recordFor(node *hsNode) *WalletRecord {
	s, err := manager("simnet").StoreFor(node.wallet)
	if err != nil {
		return nil
	}
	for _, r := range s.Wallets() {
		if r.Status == StatusActive {
			return r
		}
	}
	return nil
}

// fund adds an unspent output to the shared address.
func (sh *spendHarness) fund(address string, atoms int64, index uint32) {
	sh.utxos[address] = append(sh.utxos[address], UTXO{
		TxID: strings.Repeat("11", 32), Vout: index, Tree: 0, Atoms: atoms,
	})
}

// spend removes the inputs a broadcast transaction consumed.
func (sh *spendHarness) spend(t *testing.T, tx *wire.MsgTx) {
	t.Helper()
	for addr, set := range sh.utxos {
		kept := set[:0]
		for _, u := range set {
			used := false
			for _, in := range tx.TxIn {
				if in.PreviousOutPoint.Hash.String() == u.TxID && in.PreviousOutPoint.Index == u.Vout {
					used = true
				}
			}
			if !used {
				kept = append(kept, u)
			}
		}
		sh.utxos[addr] = kept
	}
}

func (sh *spendHarness) proposal(t *testing.T, nick, tempID, txid string) *Proposal {
	t.Helper()
	_, p, ok := sh.store(nick).Proposal(tempID, txid)
	if !ok {
		t.Fatalf("%s has no proposal %s", nick, txid[:12])
	}
	return p
}

func TestSpendTwoOfThreeRelay(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob", "carol")
	rec := sh.record("alice", tempID)
	sh.fund(rec.Address, 500_000_000, 0)

	sh.as("alice")
	dest := sh.record("alice", tempID).Address
	prop, err := ProposeSpend(sh.ctx, rec.Address,
		[]Recipient{{Address: dest, Atoms: 100_000_000}},
		[]string{sh.nodeByNick("bob").uid, sh.nodeByNick("carol").uid},
		"rent", 0, 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if prop.SigCount != 1 || prop.Status != ProposalCollecting {
		t.Fatalf("after propose: %d sigs, status %s", prop.SigCount, prop.Status)
	}
	txid := prop.TxID
	sh.pump()

	// Bob sees a reviewable request with resolved amounts.
	incoming := sh.proposal(t, "bob", tempID, txid)
	if incoming.Status != ProposalIncoming || incoming.Role != RoleCosigner {
		t.Fatalf("bob's view: %s/%s", incoming.Role, incoming.Status)
	}
	if incoming.FeeAtoms <= 0 || len(incoming.Inputs) != 1 {
		t.Fatalf("bob's view lacks resolved values: %+v", incoming)
	}
	var change *ProposalOutput
	for i := range incoming.Outputs {
		if incoming.Outputs[i].IsChange {
			change = &incoming.Outputs[i]
		}
	}
	if change == nil || change.Address != rec.Address {
		t.Fatalf("change does not return to the shared address")
	}

	sh.as("bob")
	if err := SignIncomingProposal(sh.ctx, rec.Address, txid, 0, []byte("pass")); err != nil {
		t.Fatalf("bob sign: %v", err)
	}
	sh.pump()

	final := sh.proposal(t, "alice", tempID, txid)
	if final.SigCount != 2 {
		t.Fatalf("signature count: %d", final.SigCount)
	}
	if final.Status != ProposalBroadcast {
		t.Fatalf("proposer status: %s (%s)", final.Status, final.Reason)
	}
	if len(sh.broadcasts) != 1 || sh.broadcasts[0] != txid {
		t.Fatalf("broadcasts: %v", sh.broadcasts)
	}
	// Carol was never asked: the threshold was met at the first hop.
	for _, h := range final.Queue {
		if h.Nick == "carol" && h.State != HopPending {
			t.Fatalf("carol was contacted unnecessarily: %s", h.State)
		}
	}
	// Both cosigners learn the outcome from the broadcast notice.
	if got := sh.proposal(t, "carol", tempID, txid); got.Status != ProposalBroadcast {
		t.Fatalf("carol's view after broadcast: %s", got.Status)
	}
}

func TestSpendDeclineAdvancesToAlternate(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob", "carol")
	rec := sh.record("alice", tempID)
	sh.fund(rec.Address, 500_000_000, 0)

	sh.as("alice")
	prop, err := ProposeSpend(sh.ctx, rec.Address,
		[]Recipient{{Address: rec.Address, Atoms: 100_000_000}},
		[]string{sh.nodeByNick("bob").uid, sh.nodeByNick("carol").uid},
		"", 0, 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	txid := prop.TxID
	sh.pump()

	sh.as("bob")
	if err := RejectIncomingProposal(sh.ctx, rec.Address, txid, "not this month"); err != nil {
		t.Fatalf("bob reject: %v", err)
	}
	sh.pump()

	// The relay moved on to carol without any action at the hub.
	mid := sh.proposal(t, "alice", tempID, txid)
	if mid.Status != ProposalCollecting {
		t.Fatalf("status after decline: %s", mid.Status)
	}
	var bobHop, carolHop *QueueHop
	for _, h := range mid.Queue {
		if h.Nick == "bob" {
			bobHop = h
		}
		if h.Nick == "carol" {
			carolHop = h
		}
	}
	if bobHop.State != HopDeclined || bobHop.Reason != "not this month" {
		t.Fatalf("bob hop: %+v", bobHop)
	}
	if carolHop.State != HopSent {
		t.Fatalf("carol hop not engaged: %s", carolHop.State)
	}

	sh.as("carol")
	if err := SignIncomingProposal(sh.ctx, rec.Address, txid, 0, []byte("pass")); err != nil {
		t.Fatalf("carol sign: %v", err)
	}
	sh.pump()
	if got := sh.proposal(t, "alice", tempID, txid); got.Status != ProposalBroadcast {
		t.Fatalf("final status: %s (%s)", got.Status, got.Reason)
	}
}

func TestSpendHopTimeout(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob", "carol")
	rec := sh.record("alice", tempID)
	sh.fund(rec.Address, 500_000_000, 0)

	sh.as("alice")
	prop, err := ProposeSpend(sh.ctx, rec.Address,
		[]Recipient{{Address: rec.Address, Atoms: 100_000_000}},
		[]string{sh.nodeByNick("bob").uid, sh.nodeByNick("carol").uid},
		"", time.Hour, 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	txid := prop.TxID
	// Bob never answers; age the hop past its deadline.
	sh.queue = nil
	store := sh.store("alice")
	if err := store.UpdateProposal(tempID, txid, false, func(_ *WalletRecord, p *Proposal) error {
		for _, h := range p.Queue {
			if h.State == HopSent {
				h.Deadline = time.Now().Add(-time.Minute).Unix()
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("age hop: %v", err)
	}
	sh.as("alice")
	sweepProposals(sh.ctx, store)

	after := sh.proposal(t, "alice", tempID, txid)
	var bobHop, carolHop *QueueHop
	for _, h := range after.Queue {
		if h.Nick == "bob" {
			bobHop = h
		}
		if h.Nick == "carol" {
			carolHop = h
		}
	}
	if bobHop.State != HopTimeout {
		t.Fatalf("bob hop after timeout: %s", bobHop.State)
	}
	if carolHop.State != HopSent {
		t.Fatalf("relay did not advance to carol: %s", carolHop.State)
	}
}

func TestSpendInputLocksAndSupersede(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob")
	rec := sh.record("alice", tempID)
	sh.fund(rec.Address, 500_000_000, 0)

	sh.as("alice")
	first, err := ProposeSpend(sh.ctx, rec.Address,
		[]Recipient{{Address: rec.Address, Atoms: 100_000_000}},
		[]string{sh.nodeByNick("bob").uid}, "", 0, 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	sh.queue = nil

	// The single funding output is claimed, so a second proposal cannot
	// take it.
	if _, err := ProposeSpend(sh.ctx, rec.Address,
		[]Recipient{{Address: rec.Address, Atoms: 50_000_000}},
		[]string{sh.nodeByNick("bob").uid}, "", 0, 0, []byte("pass")); err == nil {
		t.Fatalf("second proposal ignored the input lock")
	}

	// Aborting releases the claim.
	if err := AbortProposal(sh.ctx, rec.Address, first.TxID); err != nil {
		t.Fatalf("abort: %v", err)
	}
	second, err := ProposeSpend(sh.ctx, rec.Address,
		[]Recipient{{Address: rec.Address, Atoms: 50_000_000}},
		[]string{sh.nodeByNick("bob").uid}, "", 0, 0, []byte("pass"))
	if err != nil {
		t.Fatalf("proposal after abort: %v", err)
	}
	sh.pump()
	sh.as("bob")
	if err := SignIncomingProposal(sh.ctx, rec.Address, second.TxID, 0, []byte("pass")); err != nil {
		t.Fatalf("bob sign: %v", err)
	}
	sh.pump()

	// The completed spend consumed the funding output; reviving the old
	// proposal must now show it superseded.
	store := sh.store("alice")
	if err := store.UpdateProposal(tempID, first.TxID, false, func(_ *WalletRecord, p *Proposal) error {
		p.Status = ProposalCollecting
		return nil
	}); err != nil {
		t.Fatalf("revive: %v", err)
	}
	sh.as("alice")
	sweepProposals(sh.ctx, store)
	if got := sh.proposal(t, "alice", tempID, first.TxID); got.Status != ProposalSuperseded {
		t.Fatalf("stale proposal status: %s", got.Status)
	}
	if got := sh.proposal(t, "alice", tempID, second.TxID); got.Status != ProposalConfirmed {
		t.Fatalf("broadcast proposal not confirmed after its inputs left the set: %s", got.Status)
	}
}

func TestSpendCosignerRejectsTampering(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob")
	rec := sh.record("alice", tempID)
	sh.fund(rec.Address, 500_000_000, 0)

	sh.as("alice")
	prop, err := ProposeSpend(sh.ctx, rec.Address,
		[]Recipient{{Address: rec.Address, Atoms: 100_000_000}},
		[]string{sh.nodeByNick("bob").uid}, "", 0, 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	sh.queue = nil

	// A request naming outputs that are not this wallet's UTXOs is
	// auto-declined rather than queued for review.
	forged := prop.RawTx
	tx, err := DecodeTxHex(forged)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	tx.TxIn[0].PreviousOutPoint.Index = 99
	raw, err := tx.Bytes()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	bad := &Message{
		Type: TypeSignReq, WalletID: rec.Address, TxID: tx.TxHash().String(),
		RawTx: hex.EncodeToString(raw),
	}
	payload, err := EncodeMessage(bad)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	mid, _ := NewID()
	body, _ := Encode(payload, mid, time.Now().Add(time.Hour))
	alice := sh.nodeByNick("alice")
	sh.current = sh.nodeByNick("bob")
	handleInbound(alice.uid, alice.nick, body, time.Now())

	if _, _, ok := sh.store("bob").Proposal(tempID, bad.TxID); ok {
		t.Fatalf("bob queued a request spending unknown funds")
	}
	if len(sh.queue) == 0 {
		t.Fatalf("no automatic decline was sent")
	}
}
