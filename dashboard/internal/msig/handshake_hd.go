// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// HD rounds exchange dedicated-account xpubs instead of single keys.
// Creating or accepting therefore needs the wallet passphrase once, to
// create the dedicated account; declining never does.

// sanitizeAccountName builds the dedicated account's name from the
// wallet label and round id: unique per round, self-describing, and
// clear of the reserved names.
func sanitizeAccountName(label, tempID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(label) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	clean := strings.Trim(b.String(), "-")
	if len(clean) > 24 {
		clean = clean[:24]
	}
	if clean == "" {
		clean = "wallet"
	}
	suffix := tempID
	if len(suffix) > 6 {
		suffix = suffix[:6]
	}
	return "shared-" + clean + "-" + suffix
}

// newDedicatedAccount creates the round's account and fetches its xpub.
// The passphrase is the wallet passphrase; the account comes back
// per-account-encrypted with it.
func newDedicatedAccount(ctx context.Context, label, tempID string, passphrase []byte) (*OwnHDKey, error) {
	name := sanitizeAccountName(label, tempID)
	number, err := createAccountSeam(ctx, name, passphrase)
	if err != nil {
		return nil, fmt.Errorf("create the dedicated account: %v", err)
	}
	xpub, err := accountXpubSeam(ctx, number)
	if err != nil {
		return nil, fmt.Errorf("read the dedicated account's extended public key: %v", err)
	}
	return &OwnHDKey{Xpub: xpub, Account: number}, nil
}

// CreateSharedWalletHD starts an HD round: it creates the dedicated
// account, records the round and invites every participant with this
// wallet's xpub. The invitees plus this wallet form the n keys. With the
// manual transport the invitees are just labels: pseudo peer ids are
// minted locally (the wire never carries identities, so each side keeps
// its own table) and the invite frames wait in the outbox for the user
// to hand over.
func CreateSharedWalletHD(ctx context.Context, label string, m int, invitees []InviteePeer, transport string, passphrase []byte) (*WalletRecord, error) {
	defer zero(passphrase)
	network, err := networkSeam(ctx)
	if err != nil {
		return nil, err
	}
	if transport != "" && transport != TransportManual {
		return nil, fmt.Errorf("unknown coordination transport %q", transport)
	}
	manual := transport == TransportManual
	n := len(invitees) + 1
	if label == "" || len(label) > MaxLabelLen {
		return nil, fmt.Errorf("label must be 1 to %d characters", MaxLabelLen)
	}
	if !validParams(m, n) {
		return nil, fmt.Errorf("invalid scheme: %d-of-%d", m, n)
	}
	seen := make(map[string]bool, len(invitees))
	for i := range invitees {
		p := &invitees[i]
		if manual {
			if p.Nick == "" || len(p.Nick) > MaxLabelLen {
				return nil, fmt.Errorf("every cosigner needs a name of 1 to %d characters", MaxLabelLen)
			}
			key := strings.ToLower(p.Nick)
			if seen[key] {
				return nil, fmt.Errorf("duplicate cosigner name %q", p.Nick)
			}
			seen[key] = true
			if p.UID, err = NewID(); err != nil {
				return nil, err
			}
			continue
		}
		if p.UID == "" || seen[p.UID] {
			return nil, fmt.Errorf("duplicate or empty invitee")
		}
		seen[p.UID] = true
	}
	store, err := manager(network).StoreFor(activeWalletSeam())
	if err != nil {
		return nil, err
	}
	tempID, err := NewID()
	if err != nil {
		return nil, err
	}
	own, err := newDedicatedAccount(ctx, label, tempID, passphrase)
	if err != nil {
		return nil, err
	}
	peers := make([]*Peer, 0, len(invitees))
	for _, p := range invitees {
		peers = append(peers, &Peer{UID: p.UID, Nick: p.Nick, State: PeerInvited})
	}
	rec := &WalletRecord{
		TempID: tempID, Label: label, M: m, N: n, Network: network,
		HD: true, OwnHD: own, Transport: transport,
		Role: RoleInitiator, Status: StatusInviting, Peers: peers,
	}
	if err := store.PutWallet(rec); err != nil {
		return nil, err
	}
	msg := &Message{
		Type: TypeInvite, Ver: ProtoHD, TempID: tempID, Label: label,
		M: m, N: n, Network: network, Xpub: own.Xpub,
	}
	for _, p := range invitees {
		if err := sendFrame(store, p.UID, msg, tempID); err != nil {
			log.Printf("msig: invite to %s: %v", p.Nick, err)
		}
	}
	log.Printf("msig: HD round %q (%d-of-%d) started, %d invitations sent", label, m, n, len(invitees))
	rec, _ = store.Wallet(tempID)
	return rec, nil
}

// AcceptInviteHD contributes this wallet's xpub to an HD round. The
// dedicated account is created here, so declining stays free.
func AcceptInviteHD(ctx context.Context, id string, passphrase []byte) error {
	defer zero(passphrase)
	network, err := networkSeam(ctx)
	if err != nil {
		return err
	}
	store, rec := manager(network).Route(id, func(r *WalletRecord) bool {
		return r.Role == RoleCosigner
	})
	if rec == nil {
		return fmt.Errorf("unknown invite %s", id)
	}
	if !rec.HD {
		return fmt.Errorf("this invite predates HD wallets; accept it from the build that received it")
	}
	if store.WalletName() != activeWalletSeam() {
		return fmt.Errorf("this invite belongs to wallet %q; switch to it first", store.WalletName())
	}
	if rec.Status != StatusInvited {
		return fmt.Errorf("this invite can no longer be accepted")
	}
	own, err := newDedicatedAccount(ctx, rec.Label, rec.TempID, passphrase)
	if err != nil {
		return err
	}
	err = store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		if r.Status != StatusInvited {
			return fmt.Errorf("invite state changed underneath the accept")
		}
		r.OwnHD = own
		r.HD = true
		r.Status = StatusAccepted
		return nil
	})
	if err != nil {
		return err
	}
	return sendFrame(store, rec.InitiatorUID, &Message{
		Type: TypeAccept, Ver: ProtoHD, TempID: rec.TempID, Xpub: own.Xpub,
	}, "")
}

// inboundAcceptHD records a cosigner's xpub and activates once the
// roster is complete. Duplicate xpubs fail the round: two participants
// on the same extended key would collapse the scheme's threshold.
func inboundAcceptHD(ctx context.Context, store *Store, rec *WalletRecord, msg *Message, fromUID string, now time.Time) {
	if rec.Role != RoleInitiator || rec.Terminal() || rec.Status != StatusInviting {
		return
	}
	peer := rec.peerByUID(fromUID)
	if peer == nil {
		log.Printf("msig: accept for %s from a non-invited uid", rec.TempID)
		return
	}
	if msg.Ver != ProtoHD || msg.Xpub == "" {
		log.Printf("msig: %s answered an HD round with a v1 accept; peer must upgrade", peer.Nick)
		return
	}
	// A cosigner that already delivered a key never changes it. On the
	// manual transport this is the misattribution backstop: crediting a
	// second acceptance to the wrong person must not overwrite the key
	// they really delivered.
	if peer.Xpub != "" && peer.Xpub != msg.Xpub {
		log.Printf("msig: %s already delivered a different key for %s; frame ignored", peer.Nick, rec.TempID)
		return
	}
	dup := rec.OwnHD != nil && msg.Xpub == rec.OwnHD.Xpub
	for _, p := range rec.Peers {
		if p.UID != fromUID && p.Xpub == msg.Xpub {
			dup = true
		}
	}
	if dup {
		if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
			r.Status = StatusFailed
			r.FailReason = "duplicate extended key offered by " + peer.Nick
			return nil
		}); err != nil {
			log.Printf("msig: %v", err)
		}
		return
	}
	err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		p := r.peerByUID(fromUID)
		if p == nil || r.Status != StatusInviting {
			return fmt.Errorf("round %s no longer accepting", r.TempID)
		}
		p.Xpub = msg.Xpub
		p.State = PeerAccepted
		p.LastSeenTs = now.Unix()
		return nil
	})
	if err != nil {
		log.Printf("msig: %v", err)
		return
	}
	maybeActivateInitiatorHD(ctx, store, rec.TempID)
}

// maybeActivateInitiatorHD derives the ladder once every xpub is in,
// imports the initial windows and fans out the roster. Wallet-gated;
// the sweep resumes it after a switch.
func maybeActivateInitiatorHD(ctx context.Context, store *Store, tempID string) {
	rec, ok := store.Wallet(tempID)
	if !ok || !rec.HD || rec.Status != StatusInviting {
		return
	}
	for _, p := range rec.Peers {
		if p.State != PeerAccepted || p.Xpub == "" {
			return
		}
	}
	if store.WalletName() != activeWalletSeam() {
		log.Printf("msig: round %q complete; switch to wallet %q to activate it", rec.Label, store.WalletName())
		return
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		log.Printf("msig: %v", err)
		return
	}
	xpubs := make([]string, 0, rec.N)
	xpubs = append(xpubs, rec.OwnHD.Xpub)
	for _, p := range rec.Peers {
		xpubs = append(xpubs, p.Xpub)
	}
	xpubs = SortXpubs(xpubs)
	walletID, err := WalletIDForRoster(rec.M, xpubs, params)
	if err != nil {
		log.Printf("msig: derive wallet id: %v", err)
		return
	}
	if err := store.UpdateWallet(tempID, func(r *WalletRecord) error {
		r.Xpubs = xpubs
		return nil
	}); err != nil {
		log.Printf("msig: %v", err)
		return
	}
	if err := ensureWindowImported(ctx, store, tempID); err != nil {
		log.Printf("msig: ladder import deferred: %v", err)
		return
	}
	tip, err := tipHeightSeam(ctx)
	if err != nil {
		log.Printf("msig: tip height: %v", err)
		return
	}
	err = store.UpdateWallet(tempID, func(r *WalletRecord) error {
		r.Address = walletID
		r.CreatedHeight = tip
		r.Status = StatusActivating
		for _, p := range r.Peers {
			p.State = PeerRosterSent
		}
		return nil
	})
	if err != nil {
		log.Printf("msig: %v", err)
		return
	}
	rec, _ = store.Wallet(tempID)
	msg := rosterMessage(rec)
	for _, p := range rec.Peers {
		if err := sendFrame(store, p.UID, msg, ""); err != nil {
			log.Printf("msig: roster to %s: %v", p.Nick, err)
		}
	}
	log.Printf("msig: HD shared wallet %q activating at %s", rec.Label, walletID)
}

// rosterMessage builds the roster frame from a settled record. The peer
// tuples name the invitees so every cosigner learns the full membership,
// not just the initiator it heard the invite from.
func rosterMessage(rec *WalletRecord) *Message {
	msg := &Message{
		Type: TypeRoster, Ver: ProtoHD, TempID: rec.TempID, Label: rec.Label,
		M: rec.M, N: rec.N, Network: rec.Network, Xpubs: rec.Xpubs, Address: rec.Address,
	}
	for _, p := range rec.Peers {
		msg.Peers = append(msg.Peers, RosterPeer{UID: p.UID, Nick: p.Nick, Xpub: p.Xpub})
	}
	return msg
}

// inboundRosterHD verifies the roster against everything this node
// already knows, then imports the ladder. Nothing in the frame is
// trusted: the roster must contain our xpub and the initiator's, and the
// wallet id must re-derive byte-identically from the xpub set.
func inboundRosterHD(ctx context.Context, store *Store, rec *WalletRecord, msg *Message, fromUID string) {
	if fromUID != rec.InitiatorUID {
		return
	}
	if rec.Status != StatusAccepted && rec.Status != StatusPendingImport {
		// A settled record accepts a byte-identical roster purely to
		// fill in peer identities a pre-tuple build never delivered.
		// Nothing else changes: membership settled at activation. The
		// repeat is also answered with a fresh ready, so an initiator
		// stuck activating on a lost ready recovers by re-announcing.
		if rec.Status == StatusActive && rosterMatchesRecord(rec, msg) {
			if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
				return mergeRosterPeers(r, msg)
			}); err != nil {
				log.Printf("msig: %v", err)
			}
			if err := sendFrame(store, rec.InitiatorUID, &Message{
				Type: TypeReady, TempID: rec.TempID, WalletID: rec.Address,
			}, ""); err != nil {
				log.Printf("msig: ready: %v", err)
			}
		}
		return
	}
	fail := func(reason string) {
		if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
			r.Status = StatusFailed
			r.FailReason = reason
			return nil
		}); err != nil {
			log.Printf("msig: %v", err)
		}
		log.Printf("msig: roster for %q rejected: %s", rec.Label, reason)
	}
	if msg.Ver != ProtoHD || len(msg.Xpubs) == 0 {
		fail("the initiator sent a non-HD roster for an HD round")
		return
	}
	if msg.M != rec.M || msg.N != rec.N || msg.Network != rec.Network || msg.Label != rec.Label {
		fail("roster contradicts the invite")
		return
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		fail(err.Error())
		return
	}
	for _, x := range msg.Xpubs {
		if _, err := ParseXpub(x, params); err != nil {
			fail("roster key does not belong to this network")
			return
		}
	}
	ownIn, initiatorIn := false, false
	for _, x := range msg.Xpubs {
		if rec.OwnHD != nil && x == rec.OwnHD.Xpub {
			ownIn = true
		}
		if len(rec.Peers) > 0 && x == rec.Peers[0].Xpub {
			initiatorIn = true
		}
	}
	if !ownIn || !initiatorIn {
		fail("roster omits an expected key")
		return
	}
	derived, err := WalletIDForRoster(msg.M, msg.Xpubs, params)
	if err != nil {
		fail(err.Error())
		return
	}
	if derived != msg.Address {
		fail("roster address does not derive from its keys")
		return
	}
	if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		r.Xpubs = append([]string(nil), msg.Xpubs...)
		r.Address = msg.Address
		r.Status = StatusPendingImport
		return mergeRosterPeers(r, msg)
	}); err != nil {
		log.Printf("msig: %v", err)
		return
	}
	completeCosignerImportHD(ctx, store, rec.TempID)
}

// rosterMatchesRecord reports whether a roster frame restates exactly
// the membership this record settled on.
func rosterMatchesRecord(rec *WalletRecord, msg *Message) bool {
	if msg.M != rec.M || msg.N != rec.N || msg.Label != rec.Label ||
		msg.Network != rec.Network || msg.Address != rec.Address ||
		len(msg.Xpubs) != len(rec.Xpubs) {
		return false
	}
	for i, x := range msg.Xpubs {
		if rec.Xpubs[i] != x {
			return false
		}
	}
	return true
}

// mergeRosterPeers folds the roster's identity tuples into the peer
// list. Routing hints only: tuples for unknown roster xpubs are added,
// everything already known stays untouched. Manual records mint local
// ids because wire uids belong to the sender's namespace; relay records
// need an unused identity or the tuple is skipped.
func mergeRosterPeers(r *WalletRecord, msg *Message) error {
	if len(msg.Peers) == 0 {
		return nil
	}
	inRoster := make(map[string]bool, len(msg.Xpubs))
	for _, x := range msg.Xpubs {
		inRoster[x] = true
	}
	known := make(map[string]bool, 2*len(r.Peers))
	for _, p := range r.Peers {
		if p.Xpub != "" {
			known[p.Xpub] = true
		}
		if p.UID != "" {
			known[p.UID] = true
		}
	}
	for _, t := range msg.Peers {
		if !inRoster[t.Xpub] || known[t.Xpub] || (r.OwnHD != nil && t.Xpub == r.OwnHD.Xpub) {
			continue
		}
		uid := t.UID
		if r.ManualTransport() {
			var err error
			if uid, err = NewID(); err != nil {
				return err
			}
		} else if uid == "" || known[uid] {
			continue
		}
		known[t.Xpub], known[uid] = true, true
		r.Peers = append(r.Peers, &Peer{UID: uid, Nick: t.Nick, Xpub: t.Xpub, State: PeerAccepted})
	}
	return nil
}

// completeCosignerImportHD imports the ladder windows and reports ready.
// Wallet-gated; the sweep resumes it after a switch.
func completeCosignerImportHD(ctx context.Context, store *Store, tempID string) {
	rec, ok := store.Wallet(tempID)
	if !ok || !rec.HD || rec.Status != StatusPendingImport {
		return
	}
	if store.WalletName() != activeWalletSeam() {
		log.Printf("msig: shared wallet %q waits for wallet %q to import its ladder", rec.Label, store.WalletName())
		return
	}
	if err := ensureWindowImported(ctx, store, tempID); err != nil {
		log.Printf("msig: ladder import deferred: %v", err)
		return
	}
	tip, err := tipHeightSeam(ctx)
	if err != nil {
		log.Printf("msig: tip height: %v", err)
		return
	}
	err = store.UpdateWallet(tempID, func(r *WalletRecord) error {
		r.CreatedHeight = tip
		r.Status = StatusActive
		return nil
	})
	if err != nil {
		log.Printf("msig: %v", err)
		return
	}
	rec, _ = store.Wallet(tempID)
	if err := sendFrame(store, rec.InitiatorUID, &Message{
		Type: TypeReady, TempID: rec.TempID, WalletID: rec.Address,
	}, ""); err != nil {
		log.Printf("msig: ready: %v", err)
	}
	log.Printf("msig: HD shared wallet %q active at %s", rec.Label, rec.Address)
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
