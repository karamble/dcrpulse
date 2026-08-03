// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// The harness runs several "nodes" (dcrpulse wallets with their own BR
// identities) inside one process by swapping the package seams per
// delivery. Frames queue through a captured transport and are pumped
// synchronously, so every assertion sees a settled state.

type hsNode struct {
	wallet  string
	uid     string
	nick    string
	keyIdx  uint32
	imports []string // scriptHex per importScriptSeam call
	nextKey func() *OwnKey
}

type hsHarness struct {
	t       *testing.T
	nodes   map[string]*hsNode // by uid
	current *hsNode
	queue   []hsFrame
	ctx     context.Context
}

type hsFrame struct {
	from *hsNode
	to   string
	body string
}

func newHsHarness(t *testing.T, names ...string) *hsHarness {
	t.Helper()
	h := &hsHarness{t: t, nodes: make(map[string]*hsNode), ctx: context.Background()}
	base := t.TempDir()

	origWalletDir, origWalletsDir := walletDirFn, walletsDirFn
	origDerive, origImport, origTip := deriveKeySeam, importScriptSeam, tipHeightSeam
	origSend, origHist := sendPMSeam, msigHistorySeam
	origActive, origNetwork := activeWalletSeam, networkSeam
	t.Cleanup(func() {
		walletDirFn, walletsDirFn = origWalletDir, origWalletsDir
		deriveKeySeam, importScriptSeam, tipHeightSeam = origDerive, origImport, origTip
		sendPMSeam, msigHistorySeam = origSend, origHist
		activeWalletSeam, networkSeam = origActive, origNetwork
		mgrMu.Lock()
		mgr = nil
		mgrMu.Unlock()
	})

	walletDirFn = func(network, walletName string) string {
		return filepath.Join(base, network, walletName)
	}
	walletsDirFn = func(network string) string {
		return filepath.Join(base, network)
	}
	networkSeam = func(context.Context) (string, error) { return "simnet", nil }
	activeWalletSeam = func() string { return h.current.wallet }
	deriveKeySeam = func(ctx context.Context, account uint32) (*OwnKey, error) {
		return h.current.nextKey(), nil
	}
	importScriptSeam = func(ctx context.Context, scriptHex string, rescan bool, scanFrom int64) error {
		h.current.imports = append(h.current.imports, scriptHex)
		return nil
	}
	tipHeightSeam = func(context.Context) (int64, error) { return 4242, nil }
	sendPMSeam = func(ctx context.Context, user, msg string) error {
		h.queue = append(h.queue, hsFrame{from: h.current, to: user, body: msg})
		return nil
	}
	msigHistorySeam = func(ctx context.Context, uid string, limit int, since int64) (json.RawMessage, error) {
		return json.RawMessage(`{"local_nick":"","entries":[]}`), nil
	}

	mgrMu.Lock()
	mgr = nil
	mgrMu.Unlock()

	for i, name := range names {
		n := &hsNode{
			wallet: "w-" + name,
			uid:    strings.Repeat(fmt.Sprintf("%02x", i+1), 16),
			nick:   name,
		}
		n.nextKey = func() *OwnKey {
			priv, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Fatalf("generate key: %v", err)
			}
			n.keyIdx++
			return &OwnKey{
				PubKey:  hex.EncodeToString(priv.PubKey().SerializeCompressed()),
				Address: "Ss-" + n.nick,
				Account: 0, Branch: 0, Index: n.keyIdx - 1,
			}
		}
		h.nodes[n.uid] = n
	}
	h.current = h.nodeByNick(names[0])
	return h
}

func (h *hsHarness) nodeByNick(nick string) *hsNode {
	for _, n := range h.nodes {
		if n.nick == nick {
			return n
		}
	}
	h.t.Fatalf("no node %q", nick)
	return nil
}

func (h *hsHarness) as(nick string) *hsNode {
	h.current = h.nodeByNick(nick)
	return h.current
}

func (h *hsHarness) store(nick string) *Store {
	n := h.nodeByNick(nick)
	s, err := manager("simnet").StoreFor(n.wallet)
	if err != nil {
		h.t.Fatalf("store for %s: %v", nick, err)
	}
	return s
}

// pump delivers queued frames until quiet, switching the active node per
// delivery like a wallet switch would.
func (h *hsHarness) pump() {
	for len(h.queue) > 0 {
		f := h.queue[0]
		h.queue = h.queue[1:]
		to, ok := h.nodes[f.to]
		if !ok {
			h.t.Fatalf("frame to unknown uid %s", f.to)
		}
		prev := h.current
		h.current = to
		handleInbound(f.from.uid, f.from.nick, f.body, time.Now())
		h.current = prev
	}
}

// pumpTo delivers only the frames addressed to nick, including ones
// enqueued during delivery; everything else stays queued so a caller can
// drop it to simulate lost frames.
func (h *hsHarness) pumpTo(nick string) {
	target := h.nodeByNick(nick)
	for i := 0; i < len(h.queue); {
		f := h.queue[i]
		if f.to != target.uid {
			i++
			continue
		}
		h.queue = append(h.queue[:i], h.queue[i+1:]...)
		prev := h.current
		h.current = target
		handleInbound(f.from.uid, f.from.nick, f.body, time.Now())
		h.current = prev
	}
}

func (h *hsHarness) record(nick, id string) *WalletRecord {
	rec, ok := h.store(nick).Wallet(id)
	if !ok {
		h.t.Fatalf("%s has no record %s", nick, id)
	}
	return rec
}

func (h *hsHarness) create(t *testing.T, m int, initiator string, invitees ...string) string {
	t.Helper()
	h.as(initiator)
	peers := make([]InviteePeer, 0, len(invitees))
	for _, nick := range invitees {
		n := h.nodeByNick(nick)
		peers = append(peers, InviteePeer{UID: n.uid, Nick: n.nick})
	}
	rec, err := CreateSharedWallet(h.ctx, "drill", m, peers, 0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return rec.TempID
}

func TestHandshakeTwoOfThree(t *testing.T) {
	h := newHsHarness(t, "alice", "bob", "carol")
	tempID := h.create(t, 2, "alice", "bob", "carol")

	if got := h.record("alice", tempID).Status; got != StatusInviting {
		t.Fatalf("initiator status: %s", got)
	}
	h.pump()
	for _, nick := range []string{"bob", "carol"} {
		rec := h.record(nick, tempID)
		if rec.Status != StatusInvited || rec.Role != RoleCosigner {
			t.Fatalf("%s after invite: %s/%s", nick, rec.Role, rec.Status)
		}
		if rec.M != 2 || rec.N != 3 || rec.Label != "drill" {
			t.Fatalf("%s invite fields: %+v", nick, rec)
		}
	}

	h.as("bob")
	if err := AcceptInvite(h.ctx, tempID, 0); err != nil {
		t.Fatalf("bob accept: %v", err)
	}
	h.pump()
	if rec := h.record("alice", tempID); rec.Status != StatusInviting {
		t.Fatalf("alice after one accept: %s", rec.Status)
	}

	h.as("carol")
	if err := AcceptInvite(h.ctx, tempID, 0); err != nil {
		t.Fatalf("carol accept: %v", err)
	}
	h.pump()

	alice := h.record("alice", tempID)
	if alice.Status != StatusActive {
		t.Fatalf("alice final status: %s (%s)", alice.Status, alice.FailReason)
	}
	if alice.Address == "" || alice.ScriptHex == "" || len(alice.RosterPubKeys) != 3 {
		t.Fatalf("alice roster fields incomplete")
	}
	if len(h.nodeByNick("alice").imports) != 1 {
		t.Fatalf("alice imports: %d", len(h.nodeByNick("alice").imports))
	}
	for _, p := range alice.Peers {
		if p.State != PeerReady {
			t.Fatalf("peer %s state %s", p.Nick, p.State)
		}
	}
	for _, nick := range []string{"bob", "carol"} {
		rec := h.record(nick, tempID)
		if rec.Status != StatusActive {
			t.Fatalf("%s final status: %s (%s)", nick, rec.Status, rec.FailReason)
		}
		if rec.Address != alice.Address || rec.ScriptHex != alice.ScriptHex {
			t.Fatalf("%s address/script diverges", nick)
		}
		if rec.CreatedHeight != 4242 {
			t.Fatalf("%s created height: %d", nick, rec.CreatedHeight)
		}
		own := rec.Own.PubKey
		found := false
		for _, pk := range rec.RosterPubKeys {
			if pk == own {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s own key missing from roster", nick)
		}
		if len(h.nodeByNick(nick).imports) != 1 {
			t.Fatalf("%s imports: %d", nick, len(h.nodeByNick(nick).imports))
		}
	}
}

func TestHandshakeDecline(t *testing.T) {
	h := newHsHarness(t, "alice", "bob")
	tempID := h.create(t, 2, "alice", "bob")
	h.pump()

	h.as("bob")
	if err := DeclineInvite(h.ctx, tempID, "not now"); err != nil {
		t.Fatalf("decline: %v", err)
	}
	h.pump()

	alice := h.record("alice", tempID)
	if alice.Status != StatusFailed || !strings.Contains(alice.FailReason, "bob") {
		t.Fatalf("alice after decline: %s (%s)", alice.Status, alice.FailReason)
	}
	if bob := h.record("bob", tempID); bob.Status != StatusDeclined {
		t.Fatalf("bob after decline: %s", bob.Status)
	}
}

func TestHandshakeCancel(t *testing.T) {
	h := newHsHarness(t, "alice", "bob")
	tempID := h.create(t, 2, "alice", "bob")
	h.pump()

	h.as("alice")
	if err := CancelRound(h.ctx, tempID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	h.pump()

	if bob := h.record("bob", tempID); bob.Status != StatusFailed {
		t.Fatalf("bob after cancel: %s", bob.Status)
	}
}

func TestHandshakeRosterRejections(t *testing.T) {
	h := newHsHarness(t, "alice", "bob", "mallory")
	tempID := h.create(t, 2, "alice", "bob")
	h.pump()
	h.as("bob")
	if err := AcceptInvite(h.ctx, tempID, 0); err != nil {
		t.Fatalf("accept: %v", err)
	}
	// Hold the real roster back; bob stays in accepted.
	h.queue = nil

	bob := h.record("bob", tempID)
	forged := sortedPubKeyHexes(t, 2)
	badRoster := &Message{
		Type: TypeRoster, TempID: tempID, Label: "drill", M: 2, N: 2,
		Network: "simnet", PubKeys: forged, Script: "5221aa52ae", Address: "SsForged",
	}
	payload, err := EncodeMessage(badRoster)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// A roster from anyone but the initiator is ignored outright.
	mid, _ := NewID()
	body, _ := Encode(payload, mid, time.Now().Add(time.Hour))
	mallory := h.nodeByNick("mallory")
	h.current = h.nodeByNick("bob")
	handleInbound(mallory.uid, mallory.nick, body, time.Now())
	if rec := h.record("bob", tempID); rec.Status != StatusAccepted {
		t.Fatalf("bob moved on a non-initiator roster: %s", rec.Status)
	}

	// A roster from the initiator whose keys omit bob hard-fails the round.
	mid2, _ := NewID()
	body2, _ := Encode(payload, mid2, time.Now().Add(time.Hour))
	handleInbound(bob.InitiatorUID, "alice", body2, time.Now())
	rec := h.record("bob", tempID)
	if rec.Status != StatusFailed {
		t.Fatalf("bob accepted a forged roster: %s", rec.Status)
	}
	if len(h.nodeByNick("bob").imports) != 0 {
		t.Fatalf("forged roster reached importscript")
	}
}

func TestHandshakeDuplicateFramesAndKeys(t *testing.T) {
	h := newHsHarness(t, "alice", "bob", "carol")
	tempID := h.create(t, 2, "alice", "bob", "carol")
	h.pump()

	// Capture bob's accept frame to replay later.
	h.as("bob")
	if err := AcceptInvite(h.ctx, tempID, 0); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if len(h.queue) != 1 {
		t.Fatalf("expected one queued accept, got %d", len(h.queue))
	}
	replay := h.queue[0]
	h.pump()

	h.as("carol")
	if err := AcceptInvite(h.ctx, tempID, 0); err != nil {
		t.Fatalf("accept: %v", err)
	}
	h.pump()

	if rec := h.record("alice", tempID); rec.Status != StatusActive {
		t.Fatalf("round not active: %s", rec.Status)
	}
	imports := len(h.nodeByNick("alice").imports)

	// Replaying the identical accept frame must be absorbed by the journal.
	h.current = h.nodeByNick("alice")
	handleInbound(replay.from.uid, replay.from.nick, replay.body, time.Now())
	if got := len(h.nodeByNick("alice").imports); got != imports {
		t.Fatalf("replayed frame re-ran activation: %d imports", got)
	}

	// A cosigner offering an already-present key fails the round.
	h2 := newHsHarness(t, "dan", "erin", "frank")
	tempID2 := h2.create(t, 2, "dan", "erin", "frank")
	h2.pump()
	h2.as("erin")
	if err := AcceptInvite(h2.ctx, tempID2, 0); err != nil {
		t.Fatalf("erin accept: %v", err)
	}
	h2.pump()
	erinKey := h2.record("erin", tempID2).Own.PubKey
	frank := h2.nodeByNick("frank")
	frank.nextKey = func() *OwnKey {
		return &OwnKey{PubKey: erinKey, Address: "Ss-frank"}
	}
	h2.as("frank")
	if err := AcceptInvite(h2.ctx, tempID2, 0); err != nil {
		t.Fatalf("frank accept: %v", err)
	}
	h2.pump()
	rec := h2.record("dan", tempID2)
	if rec.Status != StatusFailed || !strings.Contains(rec.FailReason, "duplicate key") {
		t.Fatalf("duplicate key not rejected: %s (%s)", rec.Status, rec.FailReason)
	}
}

func TestHandshakePendingImportResume(t *testing.T) {
	h := newHsHarness(t, "alice", "bob")
	tempID := h.create(t, 2, "alice", "bob")
	h.pump()
	h.as("bob")
	if err := AcceptInvite(h.ctx, tempID, 0); err != nil {
		t.Fatalf("accept: %v", err)
	}
	// Deliver only the accept: alice activates and enqueues the roster,
	// which a full pump would deliver straight through.
	if len(h.queue) != 1 {
		t.Fatalf("expected queued accept, got %d frames", len(h.queue))
	}
	acc := h.queue[0]
	h.queue = nil
	h.current = h.nodeByNick("alice")
	handleInbound(acc.from.uid, acc.from.nick, acc.body, time.Now())

	// Deliver the roster while bob's wallet is NOT the active one: the
	// roster verifies and persists but the import defers.
	if len(h.queue) != 1 {
		t.Fatalf("expected queued roster, got %d frames", len(h.queue))
	}
	f := h.queue[0]
	h.queue = nil
	elsewhere := &hsNode{wallet: "w-elsewhere", nick: "elsewhere", uid: strings.Repeat("ee", 16)}
	h.nodes[elsewhere.uid] = elsewhere
	h.current = elsewhere
	handleInbound(f.from.uid, f.from.nick, f.body, time.Now())

	rec := h.record("bob", tempID)
	if rec.Status != StatusPendingImport {
		t.Fatalf("bob status with inactive wallet: %s", rec.Status)
	}
	if len(h.nodeByNick("bob").imports) != 0 {
		t.Fatalf("import ran against the wrong wallet")
	}

	// Switching back and resuming completes the import and reports ready.
	h.as("bob")
	ResumePending(h.ctx)
	h.pump()
	if rec := h.record("bob", tempID); rec.Status != StatusActive {
		t.Fatalf("bob after resume: %s", rec.Status)
	}
	if rec := h.record("alice", tempID); rec.Status != StatusActive {
		t.Fatalf("alice after ready: %s", rec.Status)
	}
}

func TestExpireStaleRounds(t *testing.T) {
	h := newHsHarness(t, "alice", "bob")
	tempID := h.create(t, 2, "alice", "bob")
	store := h.store("alice")
	if err := store.UpdateWallet(tempID, func(r *WalletRecord) error {
		r.CreatedAt = time.Now().Add(-9 * 24 * time.Hour).Unix()
		return nil
	}); err != nil {
		t.Fatalf("age record: %v", err)
	}
	expireStaleRounds(store)
	if rec := h.record("alice", tempID); rec.Status != StatusFailed {
		t.Fatalf("stale round not expired: %s", rec.Status)
	}
}
