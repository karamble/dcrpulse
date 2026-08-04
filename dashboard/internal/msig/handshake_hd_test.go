// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/decred/dcrd/hdkeychain/v3"

	"dcrpulse/internal/types"
)

// hdHarness extends the handshake harness with per-node deterministic
// wallets: each node owns a master key, mints dedicated accounts from it
// and serves their xpubs, mirroring dcrwallet's side of the HD seams.
type hdHarness struct {
	*hsHarness
	masters  map[string]*hdkeychain.ExtendedKey // uid -> master (simnet)
	accounts map[string][]string                // uid -> dedicated account names; number = index+1
	syncs    map[string][]string                // uid -> "name/branch/through"
}

func newHDHarness(t *testing.T, names ...string) *hdHarness {
	t.Helper()
	h := newHsHarness(t, names...)
	hd := &hdHarness{
		hsHarness: h,
		masters:   make(map[string]*hdkeychain.ExtendedKey),
		accounts:  make(map[string][]string),
		syncs:     make(map[string][]string),
	}
	params := chaincfg.SimNetParams()
	for uid, n := range h.nodes {
		seed := make([]byte, 32)
		copy(seed, n.nick)
		for i := len(n.nick); i < len(seed); i++ {
			seed[i] = byte(0x80 + i)
		}
		master, err := hdkeychain.NewMaster(seed, params)
		if err != nil {
			t.Fatal(err)
		}
		hd.masters[uid] = master
	}

	origCreate, origXpub, origSync, origAccounts := createAccountSeam, accountXpubSeam, syncBranchSeam, accountsSeam
	origSign := signMessageSeam
	t.Cleanup(func() {
		createAccountSeam, accountXpubSeam, syncBranchSeam, accountsSeam = origCreate, origXpub, origSync, origAccounts
		signMessageSeam = origSign
	})
	// Only the signing half is faked. Verification runs the real code, so a
	// forged or misdirected attestation fails these tests the way it would
	// fail in production.
	signMessageSeam = func(ctx context.Context, account uint32, address, message string, passphrase []byte) (string, error) {
		if len(passphrase) == 0 {
			return "", fmt.Errorf("wallet passphrase required")
		}
		cur := h.current
		key := hd.masters[cur.uid]
		var err error
		for _, step := range []uint32{44, 42, account} {
			if key, err = key.ChildBIP32Std(hdkeychain.HardenedKeyStart + step); err != nil {
				return "", err
			}
		}
		for _, step := range []uint32{attestBranch, attestIndex} {
			if key, err = key.Child(step); err != nil {
				return "", err
			}
		}
		priv, err := key.SerializedPrivKey()
		if err != nil {
			return "", err
		}
		hash, err := signedMessageHash(message)
		if err != nil {
			return "", err
		}
		sig := ecdsa.SignCompact(secp256k1.PrivKeyFromBytes(priv), hash, true)
		return base64.StdEncoding.EncodeToString(sig), nil
	}
	createAccountSeam = func(ctx context.Context, name string, passphrase []byte) (uint32, error) {
		if len(passphrase) == 0 {
			return 0, fmt.Errorf("wallet passphrase required")
		}
		cur := h.current
		hd.accounts[cur.uid] = append(hd.accounts[cur.uid], name)
		return uint32(len(hd.accounts[cur.uid])), nil
	}
	accountXpubSeam = func(ctx context.Context, number uint32) (string, error) {
		cur := h.current
		key := hd.masters[cur.uid]
		var err error
		for _, step := range []uint32{44, 42, number} {
			if key, err = key.ChildBIP32Std(hdkeychain.HardenedKeyStart + step); err != nil {
				return "", err
			}
		}
		return key.Neuter().String(), nil
	}
	syncBranchSeam = func(ctx context.Context, name string, branch, through uint32) error {
		cur := h.current
		hd.syncs[cur.uid] = append(hd.syncs[cur.uid], fmt.Sprintf("%s/%d/%d", name, branch, through))
		return nil
	}
	accountsSeam = func(ctx context.Context) ([]types.AccountInfo, error) {
		cur := h.current
		out := []types.AccountInfo{{AccountName: "default", AccountNumber: 0}}
		for i, name := range hd.accounts[cur.uid] {
			out = append(out, types.AccountInfo{AccountName: name, AccountNumber: uint32(i + 1)})
		}
		return out, nil
	}
	return hd
}

func (hd *hdHarness) createHD(t *testing.T, m int, initiator string, invitees ...string) string {
	t.Helper()
	hd.as(initiator)
	peers := make([]InviteePeer, 0, len(invitees))
	for _, nick := range invitees {
		n := hd.nodeByNick(nick)
		peers = append(peers, InviteePeer{UID: n.uid, Nick: n.nick})
	}
	rec, err := CreateSharedWalletHD(hd.ctx, "hd drill", m, peers, "", []byte("wallet-pass"))
	if err != nil {
		t.Fatalf("create HD: %v", err)
	}
	return rec.TempID
}

// settle drives the two human checkpoints the ceremony now has: the initiator
// signs off on the assembled key set, and every cosigner signs off on the
// roster it received. Nothing reaches active without both.
func (hd *hdHarness) settle(t *testing.T, tempID, initiator string, cosigners ...string) {
	t.Helper()
	hd.pump()
	hd.as(initiator)
	if err := ActivateRound(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("%s activate: %v", initiator, err)
	}
	hd.pump()
	for _, nick := range cosigners {
		hd.as(nick)
		if err := ConfirmRoster(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
			t.Fatalf("%s confirm: %v", nick, err)
		}
		hd.pump()
	}
	hd.pump()
}

// acceptAll accepts the invite on every named cosigner.
func (hd *hdHarness) acceptAll(t *testing.T, tempID string, cosigners ...string) {
	t.Helper()
	hd.pump()
	for _, nick := range cosigners {
		hd.as(nick)
		if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
			t.Fatalf("%s accept: %v", nick, err)
		}
	}
	hd.pump()
}

func TestHandshakeHDTwoOfTwo(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()

	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("bob accept: %v", err)
	}
	hd.settle(t, tempID, "alice", "bob")

	alice := hd.record("alice", tempID)
	bob := hd.record("bob", tempID)
	if alice.Status != StatusActive || bob.Status != StatusActive {
		t.Fatalf("statuses: alice %s (%s), bob %s (%s)", alice.Status, alice.FailReason, bob.Status, bob.FailReason)
	}
	if alice.Address == "" || alice.Address != bob.Address {
		t.Fatalf("wallet ids diverge: %q vs %q", alice.Address, bob.Address)
	}
	if len(alice.Xpubs) != 2 || len(bob.Xpubs) != 2 {
		t.Fatalf("xpub rosters: %d and %d", len(alice.Xpubs), len(bob.Xpubs))
	}
	for i := 1; i < len(alice.Xpubs); i++ {
		if alice.Xpubs[i-1] >= alice.Xpubs[i] {
			t.Fatalf("roster not canonical")
		}
	}

	// Both sides imported the full initial windows and synced both
	// branches of their dedicated accounts.
	wantImports := int(GapExt + GapInt)
	for _, nick := range []string{"alice", "bob"} {
		n := hd.nodeByNick(nick)
		if len(n.imports) != wantImports {
			t.Fatalf("%s imported %d scripts, want %d", nick, len(n.imports), wantImports)
		}
		if len(hd.syncs[n.uid]) < 2 {
			t.Fatalf("%s synced %d branches, want both", nick, len(hd.syncs[n.uid]))
		}
	}
	rec := hd.record("alice", tempID)
	if got := cursorValue(rec.Ext).ImportedThrough; got != GapExt {
		t.Fatalf("external imported-through %d != %d", got, GapExt)
	}
	if got := cursorValue(rec.Int).ImportedThrough; got != GapInt {
		t.Fatalf("internal imported-through %d != %d", got, GapInt)
	}
	if cursorValue(rec.Ext).Next != 0 || cursorValue(rec.Int).Next != 0 {
		t.Fatalf("fresh wallet cursors must start at 0")
	}

	// The dedicated account name derives from the label and round.
	if len(hd.accounts[hd.nodeByNick("alice").uid]) != 1 ||
		!strings.HasPrefix(hd.accounts[hd.nodeByNick("alice").uid][0], "shared-hd-drill-") {
		t.Fatalf("dedicated account name: %v", hd.accounts[hd.nodeByNick("alice").uid])
	}
}

func TestHandshakeHDRosterRejections(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()
	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	// Drop the real accept so the round stays open on alice's side and
	// bob keeps waiting for a roster.
	hd.queue = nil

	// A roster that omits bob's xpub must fail his round permanently.
	params := chaincfg.MainNetParams()
	foreign := SortXpubs([]string{
		testXpub(t, "mallory", 1, params),
		testXpub(t, "mallet", 1, params),
	})
	forged := &Message{
		Type: TypeRoster, Ver: ProtoHD, TempID: tempID, Label: "hd drill",
		M: 2, N: 2, Network: "simnet", Xpubs: foreign, Address: "SsForged",
	}
	payload, err := EncodeMessage(forged)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	mid, _ := NewID()
	body, _ := Encode(payload, mid, time.Now().Add(time.Hour))
	alice := hd.nodeByNick("alice")
	hd.current = hd.nodeByNick("bob")
	handleInbound(alice.uid, alice.nick, body, time.Now())

	rec := hd.record("bob", tempID)
	if rec.Status != StatusFailed {
		t.Fatalf("forged roster accepted: %s", rec.Status)
	}
	if len(hd.nodeByNick("bob").imports) != 0 {
		t.Fatalf("forged roster caused imports")
	}
}

func TestHandshakeHDDuplicateXpub(t *testing.T) {
	// bob2 shares alice's seed, so his dedicated account xpub collides
	// with hers and the initiator must fail the round.
	hd := newHDHarness(t, "alice", "bob")
	hd.masters[hd.nodeByNick("bob").uid] = hd.masters[hd.nodeByNick("alice").uid]
	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()
	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	hd.pump()

	rec := hd.record("alice", tempID)
	if rec.Status != StatusFailed || !strings.Contains(rec.FailReason, "duplicate extended key") {
		t.Fatalf("duplicate xpub not rejected: %s (%s)", rec.Status, rec.FailReason)
	}
}

func TestHandshakeHDResumePendingImport(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()
	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	hd.settle(t, tempID, "alice", "bob")

	// Rewind bob to a verified-but-unimported roster, as if the import
	// had been deferred by a wallet switch, and let the sweep resume it.
	bob := hd.nodeByNick("bob")
	store := hd.store("bob")
	if err := store.UpdateWallet(tempID, func(r *WalletRecord) error {
		r.Status = StatusPendingImport
		r.Ext = nil
		r.Int = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	bob.imports = nil
	hd.as("bob")
	ResumePending(hd.ctx)

	rec := hd.record("bob", tempID)
	if rec.Status != StatusActive {
		t.Fatalf("resume left bob at %s", rec.Status)
	}
	if got := len(bob.imports); got != int(GapExt+GapInt) {
		t.Fatalf("resume imported %d scripts, want %d", got, GapExt+GapInt)
	}
}

func TestHandshakeHDWindowCatchUp(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()
	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	hd.settle(t, tempID, "alice", "bob")

	// Advancing the receive cursor widens the window; the active-record
	// resume case must top the imports up.
	alice := hd.nodeByNick("alice")
	store := hd.store("alice")
	if err := store.UpdateWallet(tempID, func(r *WalletRecord) error {
		cursorFor(r, BranchExternal).Next = 5
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := len(alice.imports)
	hd.as("alice")
	ResumePending(hd.ctx)
	rec := hd.record("alice", tempID)
	if got := cursorValue(rec.Ext).ImportedThrough; got != 5+GapExt {
		t.Fatalf("window not topped up: imported through %d, want %d", got, 5+GapExt)
	}
	if len(alice.imports) != before+5 {
		t.Fatalf("top-up imported %d scripts, want 5", len(alice.imports)-before)
	}
}

func TestHandshakeHDTwoOfThree(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob", "carol")
	tempID := hd.createHD(t, 2, "alice", "bob", "carol")
	hd.pump()
	hd.acceptAll(t, tempID, "bob", "carol")
	hd.settle(t, tempID, "alice", "bob", "carol")
	alice := hd.record("alice", tempID)
	if alice.Status != StatusActive || len(alice.Xpubs) != 3 {
		t.Fatalf("alice: %s (%s), %d xpubs", alice.Status, alice.FailReason, len(alice.Xpubs))
	}
	for _, p := range alice.Peers {
		if p.State != PeerReady {
			t.Fatalf("peer %s state %s", p.Nick, p.State)
		}
	}
	for _, nick := range []string{"bob", "carol"} {
		rec := hd.record(nick, tempID)
		if rec.Status != StatusActive || rec.Address != alice.Address {
			t.Fatalf("%s diverges: %s %s", nick, rec.Status, rec.Address)
		}
		if rec.CreatedHeight != 4242 {
			t.Fatalf("%s created height: %d", nick, rec.CreatedHeight)
		}
	}
}

func TestHandshakeHDDecline(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()

	hd.as("bob")
	if err := DeclineInvite(hd.ctx, tempID, "not now"); err != nil {
		t.Fatalf("decline: %v", err)
	}
	hd.pump()

	alice := hd.record("alice", tempID)
	if alice.Status != StatusFailed || !strings.Contains(alice.FailReason, "bob") {
		t.Fatalf("alice after decline: %s (%s)", alice.Status, alice.FailReason)
	}
	if bob := hd.record("bob", tempID); bob.Status != StatusDeclined {
		t.Fatalf("bob after decline: %s", bob.Status)
	}
	// Declining never creates the dedicated account.
	if got := len(hd.accounts[hd.nodeByNick("bob").uid]); got != 0 {
		t.Fatalf("decline created %d accounts", got)
	}
}

func TestHandshakeHDCancel(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()

	hd.as("alice")
	if err := CancelRound(hd.ctx, tempID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	hd.pump()

	if bob := hd.record("bob", tempID); bob.Status != StatusFailed {
		t.Fatalf("bob after cancel: %s", bob.Status)
	}
}

func TestHandshakeHDReplayAbsorbed(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()
	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if len(hd.queue) != 1 {
		t.Fatalf("expected one queued accept, got %d", len(hd.queue))
	}
	replay := hd.queue[0]
	hd.settle(t, tempID, "alice", "bob")

	if rec := hd.record("alice", tempID); rec.Status != StatusActive {
		t.Fatalf("round not active: %s", rec.Status)
	}
	imports := len(hd.nodeByNick("alice").imports)

	// Replaying the identical accept frame must be absorbed by the journal.
	hd.current = hd.nodeByNick("alice")
	handleInbound(replay.from.uid, replay.from.nick, replay.body, time.Now())
	if got := len(hd.nodeByNick("alice").imports); got != imports {
		t.Fatalf("replayed frame re-ran activation: %d imports", got)
	}
}

func TestHDExpireStaleRounds(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	tempID := hd.createHD(t, 2, "alice", "bob")
	store := hd.store("alice")
	if err := store.UpdateWallet(tempID, func(r *WalletRecord) error {
		r.CreatedAt = time.Now().Add(-9 * 24 * time.Hour).Unix()
		return nil
	}); err != nil {
		t.Fatalf("age record: %v", err)
	}
	expireStaleRounds(store)
	if rec := hd.record("alice", tempID); rec.Status != StatusFailed {
		t.Fatalf("stale round not expired: %s", rec.Status)
	}
}

// Every cosigner learns the full membership from the roster's identity
// tuples, so cosigner-to-cosigner frames pass the membership check and
// the cosigner card shows all participants.
func TestHDRosterCarriesPeers(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob", "carol")
	tempID := hd.createHD(t, 2, "alice", "bob", "carol")
	hd.pump()
	hd.acceptAll(t, tempID, "bob", "carol")
	hd.settle(t, tempID, "alice", "bob", "carol")

	carolX := hd.record("carol", tempID).OwnHD.Xpub
	bob := hd.record("bob", tempID)
	if bob.Status != StatusActive || len(bob.Peers) != 2 {
		t.Fatalf("bob: %s with %d peers", bob.Status, len(bob.Peers))
	}
	if bob.Peers[0].UID != bob.InitiatorUID {
		t.Fatalf("initiator no longer first: %+v", bob.Peers[0])
	}
	var carolPeer *Peer
	for _, p := range bob.Peers {
		if p.Xpub == carolX {
			carolPeer = p
		}
	}
	if carolPeer == nil || carolPeer.Nick != "carol" || carolPeer.UID != hd.nodeByNick("carol").uid {
		t.Fatalf("bob's view of carol: %+v", carolPeer)
	}
}

// mergeRosterPeers must tolerate tuple-less legacy rosters and skip
// self, known, out-of-roster and identity-colliding tuples.
func TestMergeRosterPeersLegacyAndBounds(t *testing.T) {
	fresh := func() *WalletRecord {
		return &WalletRecord{
			OwnHD: &OwnHDKey{Xpub: "xown"},
			Peers: []*Peer{{UID: "uinit", Nick: "init", Xpub: "xinit"}},
			Xpubs: []string{"xinit", "xown", "xnew"},
		}
	}

	rec := fresh()
	if err := mergeRosterPeers(rec, &Message{Xpubs: rec.Xpubs}); err != nil || len(rec.Peers) != 1 {
		t.Fatalf("legacy roster changed peers: %v %d", err, len(rec.Peers))
	}

	rec = fresh()
	msg := &Message{Xpubs: rec.Xpubs, Peers: []RosterPeer{
		{UID: "uself", Nick: "self", Xpub: "xown"},
		{UID: "uinit2", Nick: "dup", Xpub: "xinit"},
		{UID: "uout", Nick: "out", Xpub: "xoutsider"},
		{UID: "uinit", Nick: "collide", Xpub: "xnew"},
	}}
	if err := mergeRosterPeers(rec, msg); err != nil || len(rec.Peers) != 1 {
		t.Fatalf("bad tuples merged: %v %+v", err, rec.Peers)
	}

	rec = fresh()
	msg = &Message{Xpubs: rec.Xpubs, Peers: []RosterPeer{{UID: "unew", Nick: "newbie", Xpub: "xnew"}}}
	if err := mergeRosterPeers(rec, msg); err != nil || len(rec.Peers) != 2 {
		t.Fatalf("valid tuple not merged: %v %+v", err, rec.Peers)
	}
	if p := rec.Peers[1]; p.UID != "unew" || p.Nick != "newbie" || p.Xpub != "xnew" || p.State != PeerAccepted {
		t.Fatalf("merged peer: %+v", p)
	}
}

// An initiator stuck activating must never expire by clock (the wallet
// is on-chain by then) and must recover lost readies by re-announcing
// the roster, which active cosigners answer with a fresh ready.
func TestActivationSurvivesLostReady(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	tempID := hd.createHD(t, 2, "alice", "bob")
	hd.pump()
	hd.as("bob")
	if err := AcceptInviteHD(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("accept: %v", err)
	}
	hd.pumpTo("alice")
	hd.as("alice")
	if err := ActivateRound(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("activate: %v", err)
	}
	hd.pumpTo("bob")
	hd.as("bob")
	if err := ConfirmRoster(hd.ctx, tempID, []byte("wallet-pass")); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	// Bob has confirmed and his ready sits queued; lose it.
	hd.queue = nil
	// Bob cannot be active yet: the attestation set only follows alice's
	// own activation, which the lost ready is holding up.
	if got := hd.record("bob", tempID); got.Status != StatusAttested {
		t.Fatalf("bob after confirming: %s", got.Status)
	}
	alice := hd.record("alice", tempID)
	if alice.Status != StatusActivating {
		t.Fatalf("alice without the ready: %s", alice.Status)
	}

	// The expiry sweep leaves the settled round alone even long past
	// the round horizon, while a plain stale invite round still dies.
	hd.as("alice")
	store := hd.store("alice")
	if err := store.UpdateWallet(tempID, func(r *WalletRecord) error {
		r.CreatedAt = time.Now().Add(-30 * 24 * time.Hour).Unix()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expireStaleRounds(store)
	if got := hd.record("alice", tempID); got.Status != StatusActivating {
		t.Fatalf("expiry sweep killed a settled round: %s (%s)", got.Status, got.FailReason)
	}

	// Re-announcing the roster (the Refresh path) makes bob re-ack and
	// alice finish activating.
	reannounceRosters(hd.ctx)
	hd.pumpTo("bob")
	hd.pumpTo("alice")
	if got := hd.record("alice", tempID); got.Status != StatusActive {
		t.Fatalf("alice after re-announce: %s (%s)", got.Status, got.FailReason)
	}
	// The attestation set rides the same recovery, so bob finishes too.
	hd.pump()
	if got := hd.record("bob", tempID); got.Status != StatusActive {
		t.Fatalf("bob after re-announce: %s (%s)", got.Status, got.FailReason)
	}
}

// Undeliverable frames whose envelopes expired are retired by the
// resend sweep instead of being retried forever.
func TestExpiredOutboxFrameRetired(t *testing.T) {
	hd := newHDHarness(t, "alice", "bob")
	// The relay is down during the create, so the invite stays pending.
	origSend := sendPMSeam
	sendPMSeam = func(ctx context.Context, uid, body string) error {
		return fmt.Errorf("relay offline")
	}
	hd.createHD(t, 2, "alice", "bob")
	sendPMSeam = origSend
	hd.as("alice")
	store := hd.store("alice")
	pending := store.PendingOutbox()
	if len(pending) == 0 {
		t.Fatal("no pending outbox after create")
	}
	it := pending[0]
	stale, err := Reencode(it.Body, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateOutboxBody(it.MID, it.ToUID, stale); err != nil {
		t.Fatal(err)
	}
	hd.queue = nil
	resendOutbox(store)
	for _, left := range store.PendingOutbox() {
		if left.MID == it.MID && left.ToUID == it.ToUID {
			t.Fatalf("expired frame still pending: %+v", left)
		}
	}
	if len(hd.queue) != 0 {
		t.Fatalf("expired frame was re-sent: %d sends", len(hd.queue))
	}
}
