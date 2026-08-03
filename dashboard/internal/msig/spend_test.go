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
	"github.com/decred/dcrd/hdkeychain/v3"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/sign"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"

	"dcrpulse/internal/services"
)

// spendHarness extends the HD harness with a fake chain: it holds the
// ladder's UTXO set per address and signs per input with each node's own
// child keys, emulating dcrwallet's per-account key custody.
type spendHarness struct {
	*hdHarness
	utxos      map[string][]UTXO // ladder address -> unspent
	broadcasts []string
	txConfs    map[string]int64 // txid -> confirmations for mined txs
}

func newSpendHarness(t *testing.T, m int, names ...string) (*spendHarness, string) {
	t.Helper()
	hd := newHDHarness(t, names...)
	sh := &spendHarness{
		hdHarness: hd,
		utxos:     make(map[string][]UTXO),
		txConfs:   make(map[string]int64),
	}

	origList, origSign, origBroadcast, origLookup := listUTXOsMultiSeam, signSeam, broadcastSeam, txLookupSeam
	t.Cleanup(func() {
		listUTXOsMultiSeam, signSeam, broadcastSeam, txLookupSeam = origList, origSign, origBroadcast, origLookup
	})
	listUTXOsMultiSeam = func(ctx context.Context, addresses []string) ([]UTXO, error) {
		var out []UTXO
		for _, a := range addresses {
			out = append(out, sh.utxos[a]...)
		}
		return out, nil
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
	// The fake wallet knows a tx once it was broadcast (0 conf) or mined
	// via txConfs.
	txLookupSeam = func(ctx context.Context, txid string) (int64, bool, error) {
		if conf, ok := sh.txConfs[txid]; ok {
			return conf, true, nil
		}
		for _, b := range sh.broadcasts {
			if b == txid {
				return 0, true, nil
			}
		}
		return 0, false, nil
	}

	// Run the HD handshake to an active wallet.
	cosigners := names[1:]
	tempID := hd.createHD(t, m, names[0], cosigners...)
	hd.pump()
	for _, nick := range cosigners {
		hd.as(nick)
		if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
			t.Fatalf("%s accept: %v", nick, err)
		}
		hd.pump()
	}
	rec := hd.record(names[0], tempID)
	if rec.Status != StatusActive {
		t.Fatalf("setup: wallet not active: %s (%s)", rec.Status, rec.FailReason)
	}
	return sh, tempID
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

// childPriv derives a node's private key at (branch, index) of its
// dedicated account, mirroring the production derivation.
func (sh *spendHarness) childPriv(t *testing.T, node *hsNode, account, branch, index uint32) *secp256k1.PrivateKey {
	t.Helper()
	key := sh.masters[node.uid]
	var err error
	for _, step := range []uint32{44, 42, account} {
		if key, err = key.ChildBIP32Std(hdkeychain.HardenedKeyStart + step); err != nil {
			t.Fatal(err)
		}
	}
	for _, step := range []uint32{branch, index} {
		if key, err = key.Child(step); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := key.SerializedPrivKey()
	if err != nil {
		t.Fatal(err)
	}
	return secp256k1.PrivKeyFromBytes(raw)
}

// signAs adds the signatures of the given node, resolving the redeem
// script and the node's own child key PER INPUT from the ladder.
func (sh *spendHarness) signAs(t *testing.T, node *hsNode, rawTxHex string) (string, error) {
	t.Helper()
	rec := sh.recordFor(node)
	if rec == nil {
		t.Fatalf("no shared wallet record for %s", node.nick)
	}
	view, err := ladderFor(rec)
	if err != nil {
		return "", err
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		return "", err
	}
	byOutpoint := make(map[string]UTXO)
	for _, set := range sh.utxos {
		for _, u := range set {
			byOutpoint[u.TxID+":"+itoa(u.Vout)] = u
		}
	}
	tx, err := DecodeTxHex(rawTxHex)
	if err != nil {
		return "", err
	}
	for i := range tx.TxIn {
		op := tx.TxIn[i].PreviousOutPoint
		u, ok := byOutpoint[op.Hash.String()+":"+itoa(op.Index)]
		if !ok {
			t.Fatalf("input %d spends an unknown outpoint", i)
		}
		entry, ok := view.byAddr[u.Address]
		if !ok {
			t.Fatalf("input %d address %s outside the ladder", i, u.Address)
		}
		priv := sh.childPriv(t, node, rec.OwnHD.Account, entry.Branch, entry.Index)
		pubAddr, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(priv.PubKey(), params)
		if err != nil {
			return "", err
		}
		addr, err := stdaddr.NewAddressScriptHashV0(entry.Script, params)
		if err != nil {
			return "", err
		}
		_, pkScript := addr.PaymentScript()
		kdb := sign.KeyClosure(func(a stdaddr.Address) ([]byte, dcrec.SignatureType, bool, error) {
			if a.String() != pubAddr.String() {
				return nil, 0, false, errNoKey
			}
			return priv.Serialize(), dcrec.STEcdsaSecp256k1, true, nil
		})
		script := entry.Script
		sdb := sign.ScriptClosure(func(stdaddr.Address) ([]byte, error) { return script, nil })
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

func itoa(v uint32) string {
	return strings.TrimLeft(hex.EncodeToString([]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}), "0") + "#"
}

// fund adds an unspent output at one ladder address.
func (sh *spendHarness) fund(address string, atoms int64, index uint32) {
	sh.utxos[address] = append(sh.utxos[address], UTXO{
		TxID: strings.Repeat("11", 32), Vout: index, Tree: 0, Atoms: atoms, Address: address,
	})
}

// fundIndex funds the external ladder address at the given index.
func (sh *spendHarness) fundIndex(t *testing.T, nick, tempID string, index uint32, atoms int64, vout uint32) string {
	t.Helper()
	rec := sh.record(nick, tempID)
	entry, err := entryAt(rec, BranchExternal, index)
	if err != nil {
		t.Fatal(err)
	}
	sh.fund(entry.Address, atoms, vout)
	return entry.Address
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
	dest := rec.Address
	prop, err := ProposeSpend(sh.ctx, tempID,
		[]Recipient{{Address: dest, Atoms: 100_000_000}},
		false,
		[]string{sh.nodeByNick("bob").uid, sh.nodeByNick("carol").uid},
		"rent", 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if prop.SigCount != 1 || prop.Status != ProposalCollecting {
		t.Fatalf("after propose: %d sigs, status %s", prop.SigCount, prop.Status)
	}
	txid := prop.TxID
	sh.pump()

	// Bob sees a reviewable request with resolved amounts, per-input
	// addresses and the change classified against his own derivation.
	incoming := sh.proposal(t, "bob", tempID, txid)
	if incoming.Status != ProposalIncoming || incoming.Role != RoleCosigner {
		t.Fatalf("bob's view: %s/%s", incoming.Role, incoming.Status)
	}
	if incoming.FeeAtoms <= 0 || len(incoming.Inputs) != 1 || incoming.Inputs[0].Address != rec.Address {
		t.Fatalf("bob's view lacks resolved values: %+v", incoming)
	}
	var change *ProposalOutput
	for i := range incoming.Outputs {
		if incoming.Outputs[i].IsChange {
			change = &incoming.Outputs[i]
		}
	}
	bobRec := sh.record("bob", tempID)
	wantChange, err := entryAt(bobRec, BranchInternal, 0)
	if err != nil {
		t.Fatal(err)
	}
	if change == nil || change.Address != wantChange.Address {
		t.Fatalf("change not classified to internal index 0: %+v", change)
	}

	sh.as("bob")
	if err := SignIncomingProposal(sh.ctx, tempID, txid, []byte("pass")); err != nil {
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
	if got := sh.proposal(t, "carol", tempID, txid); got.Status != ProposalBroadcast {
		t.Fatalf("carol never asked, but must know the broadcast: %s", got.Status)
	}
	// Signer sets are keyed by roster xpub.
	for signer := range final.SignedBy {
		found := false
		for _, x := range rec.Xpubs {
			if x == signer {
				found = true
			}
		}
		if !found {
			t.Fatalf("signer %q not a roster xpub", signer)
		}
	}
}

func TestSpendAcrossIndices(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob")
	rec := sh.record("alice", tempID)
	addr0 := sh.fundIndex(t, "alice", tempID, 0, 300_000_000, 0)
	addr1 := sh.fundIndex(t, "alice", tempID, 1, 200_000_000, 1)
	if addr0 == addr1 {
		t.Fatalf("indices derived the same address")
	}

	sh.as("alice")
	prop, err := ProposeSpend(sh.ctx, tempID,
		[]Recipient{{Address: rec.Address, Atoms: 450_000_000}},
		false,
		[]string{sh.nodeByNick("bob").uid},
		"", 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if len(prop.Inputs) != 2 {
		t.Fatalf("expected both indices spent, inputs: %d", len(prop.Inputs))
	}
	seen := map[string]bool{}
	for _, in := range prop.Inputs {
		seen[in.Address] = true
	}
	if !seen[addr0] || !seen[addr1] {
		t.Fatalf("inputs missing an index: %+v", prop.Inputs)
	}
	txid := prop.TxID
	sh.pump()

	sh.as("bob")
	if err := SignIncomingProposal(sh.ctx, tempID, txid, []byte("pass")); err != nil {
		t.Fatalf("bob sign across indices: %v", err)
	}
	sh.pump()
	if got := sh.proposal(t, "alice", tempID, txid); got.Status != ProposalBroadcast {
		t.Fatalf("multi-index spend did not broadcast: %s (%s)", got.Status, got.Reason)
	}
}

func TestSpendDeclineAdvancesQueue(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob", "carol")
	rec := sh.record("alice", tempID)
	sh.fund(rec.Address, 500_000_000, 0)

	sh.as("alice")
	prop, err := ProposeSpend(sh.ctx, tempID,
		[]Recipient{{Address: rec.Address, Atoms: 100_000_000}},
		false,
		[]string{sh.nodeByNick("bob").uid, sh.nodeByNick("carol").uid},
		"", 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	txid := prop.TxID
	sh.pump()

	sh.as("bob")
	if err := RejectIncomingProposal(sh.ctx, tempID, txid, "not this week"); err != nil {
		t.Fatalf("bob reject: %v", err)
	}
	sh.pump()

	// The relay moved on to carol.
	mine := sh.proposal(t, "alice", tempID, txid)
	var bobHop, carolHop *QueueHop
	for _, h := range mine.Queue {
		if h.UID == sh.nodeByNick("bob").uid {
			bobHop = h
		}
		if h.UID == sh.nodeByNick("carol").uid {
			carolHop = h
		}
	}
	if bobHop == nil || bobHop.State != HopDeclined || bobHop.Reason != "not this week" {
		t.Fatalf("bob's hop: %+v", bobHop)
	}
	if carolHop == nil || carolHop.State != HopSent {
		t.Fatalf("carol's hop: %+v", carolHop)
	}
	if got := sh.proposal(t, "carol", tempID, txid); got.Status != ProposalIncoming {
		t.Fatalf("carol did not receive the re-route: %s", got.Status)
	}
}

func TestSpendInputLocksAndSupersede(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob")
	rec := sh.record("alice", tempID)
	sh.fund(rec.Address, 500_000_000, 0)

	sh.as("alice")
	first, err := ProposeSpend(sh.ctx, tempID,
		[]Recipient{{Address: rec.Address, Atoms: 100_000_000}},
		false,
		[]string{sh.nodeByNick("bob").uid}, "", 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	sh.queue = nil

	// The single funding output is claimed, so a second proposal cannot
	// take it.
	if _, err := ProposeSpend(sh.ctx, tempID,
		[]Recipient{{Address: rec.Address, Atoms: 50_000_000}},
		false,
		[]string{sh.nodeByNick("bob").uid}, "", 0, []byte("pass")); err == nil {
		t.Fatalf("second proposal ignored the input lock")
	}

	// Aborting releases the claim.
	if err := AbortProposal(sh.ctx, tempID, first.TxID); err != nil {
		t.Fatalf("abort: %v", err)
	}
	second, err := ProposeSpend(sh.ctx, tempID,
		[]Recipient{{Address: rec.Address, Atoms: 50_000_000}},
		false,
		[]string{sh.nodeByNick("bob").uid}, "", 0, []byte("pass"))
	if err != nil {
		t.Fatalf("proposal after abort: %v", err)
	}
	sh.pump()
	sh.as("bob")
	if err := SignIncomingProposal(sh.ctx, tempID, second.TxID, []byte("pass")); err != nil {
		t.Fatalf("bob sign: %v", err)
	}
	sh.pump()

	// The completed spend consumed the funding output and gets mined;
	// reviving the old proposal must now show it superseded.
	sh.txConfs[second.TxID] = 1
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
		t.Fatalf("broadcast proposal not confirmed after mining: %s", got.Status)
	}
}

func TestSpendForgedInputsAutoDeclined(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob")
	rec := sh.record("alice", tempID)
	sh.fund(rec.Address, 500_000_000, 0)

	sh.as("alice")
	prop, err := ProposeSpend(sh.ctx, tempID,
		[]Recipient{{Address: rec.Address, Atoms: 100_000_000}},
		false,
		[]string{sh.nodeByNick("bob").uid}, "", 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	sh.queue = nil

	tx, err := DecodeTxHex(prop.RawTx)
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

func TestSpendSendAll(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob", "carol")
	rec := sh.record("alice", tempID)
	sh.fundIndex(t, "alice", tempID, 0, 300_000_000, 0)
	sh.fundIndex(t, "alice", tempID, 1, 200_000_000, 1)

	sh.as("alice")
	if _, err := ProposeSpend(sh.ctx, tempID,
		[]Recipient{{Address: rec.Address}, {Address: rec.Address}},
		true,
		[]string{sh.nodeByNick("bob").uid},
		"", 0, []byte("pass")); err == nil {
		t.Fatalf("send all accepted two recipients")
	}

	prop, err := ProposeSpend(sh.ctx, tempID,
		[]Recipient{{Address: rec.Address}},
		true,
		[]string{sh.nodeByNick("bob").uid, sh.nodeByNick("carol").uid},
		"sweep", 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose send all: %v", err)
	}
	if len(prop.Inputs) != 2 {
		t.Fatalf("send all left inputs behind: %d", len(prop.Inputs))
	}
	if len(prop.Outputs) != 1 || prop.Outputs[0].IsChange {
		t.Fatalf("send all outputs: %+v", prop.Outputs)
	}
	if prop.FeeAtoms <= 0 || prop.Outputs[0].Atoms != 500_000_000-prop.FeeAtoms {
		t.Fatalf("sweep amount %d + fee %d does not consume the balance",
			prop.Outputs[0].Atoms, prop.FeeAtoms)
	}
	txid := prop.TxID
	sh.pump()

	sh.as("bob")
	if err := SignIncomingProposal(sh.ctx, tempID, txid, []byte("pass")); err != nil {
		t.Fatalf("bob sign: %v", err)
	}
	sh.pump()

	final := sh.proposal(t, "alice", tempID, txid)
	if final.Status != ProposalBroadcast {
		t.Fatalf("proposer status: %s (%s)", final.Status, final.Reason)
	}
	for _, set := range sh.utxos {
		if len(set) != 0 {
			t.Fatalf("sweep left unspent outputs behind")
		}
	}
}

func TestSpendBroadcastVerifiedAndRebroadcastConverges(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob")
	rec := sh.record("alice", tempID)
	sh.fund(rec.Address, 500_000_000, 0)

	sh.as("alice")
	prop, err := ProposeSpend(sh.ctx, tempID,
		[]Recipient{{Address: rec.Address, Atoms: 100_000_000}},
		false,
		[]string{sh.nodeByNick("bob").uid}, "", 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	sh.pump()
	sh.as("bob")
	if err := SignIncomingProposal(sh.ctx, tempID, prop.TxID, []byte("pass")); err != nil {
		t.Fatalf("bob sign: %v", err)
	}
	sh.pump()

	got := sh.proposal(t, "bob", tempID, prop.TxID)
	if got.Status != ProposalBroadcast || got.Reason != "" {
		t.Fatalf("bob after verified notice: %s (%q)", got.Status, got.Reason)
	}

	sh.as("alice")
	if err := RebroadcastProposal(sh.ctx, tempID, prop.TxID); err != nil {
		t.Fatalf("rebroadcast after auto-broadcast: %v", err)
	}

	sh.txConfs[prop.TxID] = 1
	sweepProposals(sh.ctx, sh.store("alice"))
	sh.as("bob")
	sweepProposals(sh.ctx, sh.store("bob"))
	if got := sh.proposal(t, "alice", tempID, prop.TxID); got.Status != ProposalConfirmed {
		t.Fatalf("alice after mining: %s", got.Status)
	}
	if got := sh.proposal(t, "bob", tempID, prop.TxID); got.Status != ProposalConfirmed {
		t.Fatalf("bob after mining: %s", got.Status)
	}
}

func TestSpendSignedConfirmsWhenNoticeLost(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob")
	rec := sh.record("alice", tempID)
	sh.fund(rec.Address, 500_000_000, 0)

	sh.as("alice")
	prop, err := ProposeSpend(sh.ctx, tempID,
		[]Recipient{{Address: rec.Address, Atoms: 100_000_000}},
		false,
		[]string{sh.nodeByNick("bob").uid}, "", 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	sh.pump()
	sh.as("bob")
	if err := SignIncomingProposal(sh.ctx, tempID, prop.TxID, []byte("pass")); err != nil {
		t.Fatalf("bob sign: %v", err)
	}
	sh.pumpTo("alice")
	sh.queue = nil
	if got := sh.proposal(t, "bob", tempID, prop.TxID); got.Status != ProposalSigned {
		t.Fatalf("bob should still be waiting: %s", got.Status)
	}

	sh.txConfs[prop.TxID] = 1
	sh.as("bob")
	sweepProposals(sh.ctx, sh.store("bob"))
	if got := sh.proposal(t, "bob", tempID, prop.TxID); got.Status != ProposalConfirmed {
		t.Fatalf("bob after silent confirmation: %s (%s)", got.Status, got.Reason)
	}
}

func TestSpendNoticeForUnseenTxStaysCaveated(t *testing.T) {
	sh, tempID := newSpendHarness(t, 2, "alice", "bob")
	rec := sh.record("alice", tempID)

	txid := strings.Repeat("ab", 32)
	notice := &Message{Type: TypeBroadcast, WalletID: rec.Address, TxID: txid}
	payload, err := EncodeMessage(notice)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	mid, _ := NewID()
	body, _ := Encode(payload, mid, time.Now().Add(time.Hour))
	alice := sh.nodeByNick("alice")
	sh.current = sh.nodeByNick("bob")
	handleInbound(alice.uid, alice.nick, body, time.Now())

	got := sh.proposal(t, "bob", tempID, txid)
	if got.Status != ProposalBroadcast || got.Reason == "" {
		t.Fatalf("unverified notice: %s (%q)", got.Status, got.Reason)
	}

	sh.txConfs[txid] = 1
	sh.as("bob")
	sweepProposals(sh.ctx, sh.store("bob"))
	got = sh.proposal(t, "bob", tempID, txid)
	if got.Status != ProposalConfirmed || got.Reason != "" {
		t.Fatalf("after local confirmation: %s (%q)", got.Status, got.Reason)
	}
}
