// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dcrpulse/internal/services"
)

// The manual tests assert one invariant everywhere: the Bison Relay
// transport is NEVER touched — hd.queue (the sendPM capture) stays empty
// and frames move only through ManualOutbox and ImportFrame.

// createManual starts a manual round from the initiator with label-only
// invitees.
func createManual(t *testing.T, hd *hdHarness, initiator string, m int, labels ...string) *WalletRecord {
	t.Helper()
	hd.as(initiator)
	invitees := make([]InviteePeer, 0, len(labels))
	for _, l := range labels {
		invitees = append(invitees, InviteePeer{Nick: l})
	}
	rec, err := CreateSharedWalletHD(hd.ctx, "manual drill", m, invitees, TransportManual, []byte("wallet-pass"))
	if err != nil {
		t.Fatalf("create manual: %v", err)
	}
	if len(hd.queue) != 0 {
		t.Fatalf("manual create touched the BR transport")
	}
	return rec
}

// exportOne returns the single pending manual frame of the given type.
func exportOne(t *testing.T, hd *hdHarness, nick, recID, frameType string) ManualFrame {
	t.Helper()
	hd.as(nick)
	frames, err := ManualOutbox(hd.ctx, recID)
	if err != nil {
		t.Fatalf("%s outbox: %v", nick, err)
	}
	var got []ManualFrame
	for _, f := range frames {
		if f.Type == frameType {
			got = append(got, f)
		}
	}
	if len(got) != 1 {
		t.Fatalf("%s has %d pending %s frames, want 1 (%+v)", nick, len(got), frameType, frames)
	}
	return got[0]
}

// importAs imports a frame on the given node, attributed to fromUID.
func importAs(t *testing.T, hd *hdHarness, nick, body, id, fromUID, fromLabel string) *ImportResult {
	t.Helper()
	hd.as(nick)
	res, err := ImportFrame(hd.ctx, body, id, fromUID, fromLabel)
	if err != nil {
		t.Fatalf("%s import: %v", nick, err)
	}
	return res
}

// manualHandshake walks a 2-party manual round to active on both sides
// and returns the tempID.
func manualHandshake(t *testing.T, hd *hdHarness, initiator, cosigner string) string {
	t.Helper()
	rec := createManual(t, hd, initiator, 2, "ferry-peer")
	tempID := rec.TempID

	invite := exportOne(t, hd, initiator, tempID, TypeInvite)
	if res := importAs(t, hd, cosigner, invite.Body, "", "", initiator+"-label"); res.Outcome != "processed" {
		t.Fatalf("invite import outcome: %s", res.Outcome)
	}
	hd.as(cosigner)
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	accept := exportOne(t, hd, cosigner, tempID, TypeAccept)
	initRec := hd.record(initiator, tempID)
	if res := importAs(t, hd, initiator, accept.Body, tempID, initRec.Peers[0].UID, ""); res.Outcome != "processed" {
		t.Fatalf("accept import outcome: %s", res.Outcome)
	}
	roster := exportOne(t, hd, initiator, tempID, TypeRoster)
	coRec := hd.record(cosigner, tempID)
	if res := importAs(t, hd, cosigner, roster.Body, tempID, coRec.InitiatorUID, ""); res.Outcome != "processed" {
		t.Fatalf("roster import outcome: %s", res.Outcome)
	}
	ready := exportOne(t, hd, cosigner, tempID, TypeReady)
	if res := importAs(t, hd, initiator, ready.Body, tempID, initRec.Peers[0].UID, ""); res.Outcome != "processed" {
		t.Fatalf("ready import outcome: %s", res.Outcome)
	}
	if len(hd.queue) != 0 {
		t.Fatalf("manual handshake touched the BR transport")
	}
	return tempID
}

func TestManualHandshakeTwoOfTwo(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	tempID := manualHandshake(t, hd, "alice", "bob")

	alice := hd.record("alice", tempID)
	bob := hd.record("bob", tempID)
	if alice.Status != StatusActive || bob.Status != StatusActive {
		t.Fatalf("statuses: %s / %s", alice.Status, bob.Status)
	}
	if alice.Address == "" || alice.Address != bob.Address {
		t.Fatalf("wallet ids diverge: %q vs %q", alice.Address, bob.Address)
	}
	if !alice.ManualTransport() || !bob.ManualTransport() {
		t.Fatalf("transport not manual on both sides")
	}
	// The invite export retired itself once the peer's accept proved
	// delivery.
	hd.as("alice")
	frames, err := ManualOutbox(hd.ctx, tempID)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range frames {
		if f.Type == TypeInvite || f.Type == TypeRoster {
			t.Fatalf("proven-delivered %s frame still pending", f.Type)
		}
	}
}

func TestManualSpendFerry(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
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

	tempID := manualHandshake(t, hd, "alice", "bob")
	rec := hd.record("alice", tempID)
	sh.fund(rec.Address, 500_000_000, 0)

	hd.as("alice")
	prop, err := ProposeSpend(hd.ctx, tempID,
		[]Recipient{{Address: rec.Address, Atoms: 100_000_000}},
		false,
		[]string{rec.Peers[0].UID},
		"ferry", 0, []byte("pass"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if len(hd.queue) != 0 {
		t.Fatalf("manual propose touched the BR transport")
	}
	// No hop deadline, hand-carried lifetime on the envelope.
	mine := sh.proposal(t, "alice", tempID, prop.TxID)
	if mine.Queue[0].Deadline != 0 {
		t.Fatalf("manual hop carries a deadline: %d", mine.Queue[0].Deadline)
	}
	signReq := exportOne(t, hd, "alice", tempID, TypeSignReq)
	frame, err := Parse(signReq.Body, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if until := time.Until(frame.Exp); until < 29*24*time.Hour || until > 31*24*time.Hour {
		t.Fatalf("manual sign_req lifetime %v, want ~30d", until)
	}

	coRec := hd.record("bob", tempID)
	if res := importAs(t, hd, "bob", signReq.Body, tempID, coRec.InitiatorUID, ""); res.Outcome != "processed" {
		t.Fatalf("sign_req import outcome: %s", res.Outcome)
	}
	hd.as("bob")
	if err := SignIncomingProposal(hd.ctx, tempID, prop.TxID, []byte("pass")); err != nil {
		t.Fatalf("bob sign: %v", err)
	}
	sig := exportOne(t, hd, "bob", tempID, TypeSig)
	initRec := hd.record("alice", tempID)
	if res := importAs(t, hd, "alice", sig.Body, tempID, initRec.Peers[0].UID, ""); res.Outcome != "processed" {
		t.Fatalf("sig import outcome: %s", res.Outcome)
	}
	final := sh.proposal(t, "alice", tempID, prop.TxID)
	if final.Status != ProposalBroadcast {
		t.Fatalf("proposer status: %s (%s)", final.Status, final.Reason)
	}
	if len(sh.broadcasts) != 1 {
		t.Fatalf("broadcasts: %v", sh.broadcasts)
	}
	// The broadcast notice waits for hand-over instead of being sent.
	exportOne(t, hd, "alice", tempID, TypeBroadcast)
	if len(hd.queue) != 0 {
		t.Fatalf("manual spend ferry touched the BR transport")
	}
}

func TestManualResendAndCatchupSkip(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	tempID := manualHandshake(t, hd, "alice", "bob")

	sends := 0
	origSend := sendPMSeam
	t.Cleanup(func() { sendPMSeam = origSend })
	sendPMSeam = func(ctx context.Context, user, msg string) error {
		sends++
		return nil
	}
	histories := 0
	origHist := msigHistorySeam
	t.Cleanup(func() { msigHistorySeam = origHist })
	msigHistorySeam = func(ctx context.Context, uid string, limit int, since int64) (json.RawMessage, error) {
		histories++
		return json.RawMessage(`{"local_nick":"","entries":[]}`), nil
	}

	// A pending manual frame exists (the ready export); sweeping must
	// neither send it nor replay pseudo peers.
	hd.as("bob")
	runSweep(hd.ctx)
	if sends != 0 || histories != 0 {
		t.Fatalf("sweep touched BR for a manual wallet: %d sends, %d replays", sends, histories)
	}
	_ = tempID
}

func TestManualRoundExpiryHorizon(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	rec := createManual(t, hd, "alice", 2, "ferry-peer")
	store := hd.store("alice")

	if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		r.CreatedAt = time.Now().Add(-9 * 24 * time.Hour).Unix()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expireStaleRounds(store)
	if got := hd.record("alice", rec.TempID); got.Status != StatusInviting {
		t.Fatalf("manual round died at the BR horizon: %s", got.Status)
	}

	if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		r.CreatedAt = time.Now().Add(-32 * 24 * time.Hour).Unix()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expireStaleRounds(store)
	if got := hd.record("alice", rec.TempID); got.Status != StatusFailed {
		t.Fatalf("manual round survived past its horizon: %s", got.Status)
	}
}

func TestManualImportWrongActiveWallet(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob", "carol")
	tempID := manualHandshake(t, hd, "alice", "bob")

	// Importing while a wallet WITHOUT the record is active must refuse
	// loudly instead of routing into whichever store the router finds.
	hd.as("bob")
	frames, err := ManualOutbox(hd.ctx, tempID)
	if err != nil {
		t.Fatal(err)
	}
	var anyFrame ManualFrame
	for _, f := range frames {
		anyFrame = f
	}
	coRec := hd.record("bob", tempID)
	hd.as("carol")
	_, err = ImportFrame(hd.ctx, anyFrame.Body, tempID, coRec.InitiatorUID, "")
	if err == nil || !strings.Contains(err.Error(), "switch to it first") {
		t.Fatalf("wrong-wallet import not refused: %v", err)
	}
}

func TestManualExpiryAndReencode(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	rec := createManual(t, hd, "alice", 2, "ferry-peer")
	invite := exportOne(t, hd, "alice", rec.TempID, TypeInvite)

	// An expired copy is refused with a friendly error.
	frame, err := Parse(invite.Body, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	expired, err := Reencode(invite.Body, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	hd.as("bob")
	if _, err := ImportFrame(hd.ctx, expired, "", "", "alice"); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired frame not refused: %v", err)
	}

	// A re-encoded fresh copy keeps the mid and imports exactly once.
	fresh, err := Reencode(invite.Body, time.Now().Add(ManualTTL))
	if err != nil {
		t.Fatal(err)
	}
	freshFrame, err := Parse(fresh, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if freshFrame.MID != frame.MID {
		t.Fatalf("reencode changed the mid: %s != %s", freshFrame.MID, frame.MID)
	}
	if res := importAs(t, hd, "bob", fresh, "", "", "alice"); res.Outcome != "processed" {
		t.Fatalf("fresh copy outcome: %s", res.Outcome)
	}
	if res := importAs(t, hd, "bob", invite.Body, "", "", "alice"); res.Outcome != "duplicate" {
		t.Fatalf("second copy outcome: %s", res.Outcome)
	}
}

func TestManualWrongAttribution(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob", "carol")
	rec := createManual(t, hd, "alice", 2, "peer-bob", "peer-carol")
	tempID := rec.TempID

	hd.as("alice")
	frames, err := ManualOutbox(hd.ctx, tempID)
	if err != nil {
		t.Fatal(err)
	}
	var invite ManualFrame
	for _, f := range frames {
		if f.Type == TypeInvite {
			invite = f
		}
	}
	importAs(t, hd, "bob", invite.Body, "", "", "alice")
	importAs(t, hd, "carol", invite.Body, "", "", "alice")
	for _, nick := range []string{"bob", "carol"} {
		hd.as(nick)
		if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
			t.Fatalf("%s accept: %v", nick, err)
		}
	}
	bobAccept := exportOne(t, hd, "bob", tempID, TypeAccept)
	carolAccept := exportOne(t, hd, "carol", tempID, TypeAccept)

	// Swap the attributions: bob's accept lands on the peer-carol slot
	// and vice versa. Labels end up wrong; the derived wallet does not.
	initRec := hd.record("alice", tempID)
	var peerBob, peerCarol string
	for _, p := range initRec.Peers {
		if p.Nick == "peer-bob" {
			peerBob = p.UID
		}
		if p.Nick == "peer-carol" {
			peerCarol = p.UID
		}
	}
	importAs(t, hd, "alice", bobAccept.Body, tempID, peerCarol, "")
	importAs(t, hd, "alice", carolAccept.Body, tempID, peerBob, "")

	alice := hd.record("alice", tempID)
	if alice.Status != StatusActivating && alice.Status != StatusActive {
		t.Fatalf("misattributed round did not activate: %s (%s)", alice.Status, alice.FailReason)
	}
	if alice.Address == "" {
		t.Fatalf("no wallet id derived")
	}

	// The same accept under BOTH labels trips the duplicate-xpub guard.
	hd2 := newHDHarness(t, "dan", "erin")
	rec2 := createManual(t, hd2, "dan", 2, "p1", "p2")
	hd2.as("dan")
	frames2, err := ManualOutbox(hd2.ctx, rec2.TempID)
	if err != nil {
		t.Fatal(err)
	}
	var inv2 ManualFrame
	for _, f := range frames2 {
		if f.Type == TypeInvite {
			inv2 = f
		}
	}
	importAs(t, hd2, "erin", inv2.Body, "", "", "dan")
	hd2.as("erin")
	if err := AcceptInviteHD(hd2.ctx, rec2.TempID, []byte("wallet-pass")); err != nil {
		t.Fatal(err)
	}
	acc2 := exportOne(t, hd2, "erin", rec2.TempID, TypeAccept)
	danRec := hd2.record("dan", rec2.TempID)
	importAs(t, hd2, "dan", acc2.Body, rec2.TempID, danRec.Peers[0].UID, "")
	dup, err := Reencode(acc2.Body, time.Now().Add(ManualTTL))
	if err != nil {
		t.Fatal(err)
	}
	// Same mid would be journal-absorbed; a NEW mid simulates a copy
	// pasted under the second label after a journal prune.
	fresh2 := strings.Replace(dup, "mid="+mustMid(t, dup), "mid=aaaabbbbccccdddd", 1)
	res, err := ImportFrame(hd2.ctx, fresh2, rec2.TempID, danRec.Peers[1].UID, "")
	if err != nil {
		t.Fatalf("dup-label import errored: %v", err)
	}
	_ = res
	if got := hd2.record("dan", rec2.TempID); got.Status != StatusFailed || !strings.Contains(got.FailReason, "duplicate extended key") {
		t.Fatalf("duplicate xpub across labels not fail-safed: %s (%s)", got.Status, got.FailReason)
	}
}

func mustMid(t *testing.T, body string) string {
	t.Helper()
	f, err := Parse(body, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return f.MID
}

func TestManualDoneScoping(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob", "carol")
	rec := createManual(t, hd, "alice", 2, "p1", "p2")

	// One invite mid fans out to two peers; marking one delivered leaves
	// the other's card.
	hd.as("alice")
	frames, err := ManualOutbox(hd.ctx, rec.TempID)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("expected 2 invite cards, have %d", len(frames))
	}
	if err := MarkManualDelivered(hd.ctx, rec.TempID, frames[0].MID, frames[0].ToUID); err != nil {
		t.Fatal(err)
	}
	after, err := ManualOutbox(hd.ctx, rec.TempID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].ToUID == frames[0].ToUID {
		t.Fatalf("done marking not scoped per peer: %+v", after)
	}
}

// exportAll returns every pending manual frame of one type.
func exportAll(t *testing.T, hd *hdHarness, nick, recID, frameType string) []ManualFrame {
	t.Helper()
	hd.as(nick)
	frames, err := ManualOutbox(hd.ctx, recID)
	if err != nil {
		t.Fatalf("%s outbox: %v", nick, err)
	}
	var got []ManualFrame
	for _, f := range frames {
		if f.Type == frameType {
			got = append(got, f)
		}
	}
	if len(got) == 0 {
		t.Fatalf("%s has no pending %s frames (%+v)", nick, frameType, frames)
	}
	return got
}

// A manual roster's identity tuples carry the initiator's local pseudo
// ids; every receiver must mint its own and keep only nick and xpub.
func TestManualRosterMintsLocalPeers(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob", "carol")
	rec := createManual(t, hd, "alice", 2, "bobby", "carly")
	tempID := rec.TempID

	invite := exportAll(t, hd, "alice", tempID, TypeInvite)[0]
	for _, nick := range []string{"bob", "carol"} {
		if res := importAs(t, hd, nick, invite.Body, "", "", "alice-label"); res.Outcome != "processed" {
			t.Fatalf("%s invite import: %s", nick, res.Outcome)
		}
		hd.as(nick)
		if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
			t.Fatalf("%s accept: %v", nick, err)
		}
	}

	initRec := hd.record("alice", tempID)
	peerUID := func(nick string) string {
		for _, p := range initRec.Peers {
			if p.Nick == nick {
				return p.UID
			}
		}
		t.Fatalf("initiator lacks peer %q", nick)
		return ""
	}
	bobAccept := exportAll(t, hd, "bob", tempID, TypeAccept)[0]
	if res := importAs(t, hd, "alice", bobAccept.Body, tempID, peerUID("bobby"), ""); res.Outcome != "processed" {
		t.Fatalf("bob accept import: %s", res.Outcome)
	}
	carolAccept := exportAll(t, hd, "carol", tempID, TypeAccept)[0]
	if res := importAs(t, hd, "alice", carolAccept.Body, tempID, peerUID("carly"), ""); res.Outcome != "processed" {
		t.Fatalf("carol accept import: %s", res.Outcome)
	}

	roster := exportAll(t, hd, "alice", tempID, TypeRoster)[0]
	for _, nick := range []string{"bob", "carol"} {
		co := hd.record(nick, tempID)
		if res := importAs(t, hd, nick, roster.Body, tempID, co.InitiatorUID, ""); res.Outcome != "processed" {
			t.Fatalf("%s roster import: %s", nick, res.Outcome)
		}
	}

	carolX := hd.record("carol", tempID).OwnHD.Xpub
	bob := hd.record("bob", tempID)
	if !bob.ManualTransport() || len(bob.Peers) != 2 {
		t.Fatalf("bob: manual=%v peers=%d", bob.ManualTransport(), len(bob.Peers))
	}
	var carly *Peer
	for _, p := range bob.Peers {
		if p.Xpub == carolX {
			carly = p
		}
	}
	if carly == nil || carly.Nick != "carly" {
		t.Fatalf("bob's view of carly: %+v", carly)
	}
	if carly.UID == "" || carly.UID == peerUID("carly") {
		t.Fatalf("bob kept the initiator's pseudo id: %q", carly.UID)
	}
}
