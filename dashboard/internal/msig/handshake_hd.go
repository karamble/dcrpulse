// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"fmt"
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
	// Who this round is being offered to, so an invitee sees the membership
	// before it decides rather than only after the roster settles. On the
	// manual transport the ids are local pseudo-ids that mean nothing in the
	// recipient's namespace, so only the names travel.
	proposed := make([]RosterPeer, 0, len(invitees))
	for _, p := range invitees {
		t := RosterPeer{Nick: p.Nick}
		if !manual {
			t.UID = p.UID
		}
		proposed = append(proposed, t)
	}
	rec := &WalletRecord{
		TempID: tempID, Label: label, M: m, N: n, Network: network,
		HD: true, OwnHD: own, Transport: transport,
		Role: RoleInitiator, Status: StatusInviting, Peers: peers,
		ProposedPeers: proposed,
	}
	if err := store.PutWallet(rec); err != nil {
		return nil, err
	}
	msg := &Message{
		Type: TypeInvite, Ver: ProtoHD, TempID: tempID, Label: label,
		M: m, N: n, Network: network, Xpub: own.Xpub, Peers: proposed,
	}
	for _, p := range invitees {
		if err := sendFrame(store, p.UID, msg, tempID); err != nil {
			msigLog.Warnf("invite to %s: %v", p.Nick, err)
		}
	}
	msigLog.Infof("HD round %q (%d-of-%d) started, %d invitations sent", label, m, n, len(invitees))
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
	// Prove the key is held before it can reach anyone's roster, so a round
	// can never settle on a key nobody has.
	pop, err := signOwnAttest(ctx, rec, own, AttestPoPMessage(rec.Network, rec.TempID, own.Xpub), passphrase)
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
		Type: TypeAccept, Ver: ProtoHD, TempID: rec.TempID, Xpub: own.Xpub, Attest: pop,
	}, "")
}

// signOwnAttest signs one attestation with the round's dedicated account key.
// The signing address is child (0,0) of that account's xpub, the one key every
// other participant can derive from the roster alone.
func signOwnAttest(ctx context.Context, rec *WalletRecord, own *OwnHDKey, message string, passphrase []byte) (string, error) {
	if own == nil {
		return "", fmt.Errorf("this wallet has no key in the round yet")
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		return "", err
	}
	addr, err := AttestAddress(own.Xpub, params)
	if err != nil {
		return "", err
	}
	// The wallet signs only for an address it has already derived.
	name := sanitizeAccountName(rec.Label, rec.TempID)
	if err := syncBranchSeam(ctx, name, attestBranch, attestIndex); err != nil {
		return "", fmt.Errorf("prepare the signing address: %v", err)
	}
	// signMessageSeam wipes what it is given, and callers still need theirs.
	sig, err := signMessageSeam(ctx, own.Account, addr, message, append([]byte(nil), passphrase...))
	if err != nil {
		return "", fmt.Errorf("sign the attestation: %v", err)
	}
	return sig, nil
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
		msigLog.Warnf("accept for %s from a non-invited uid", rec.TempID)
		return
	}
	if msg.Ver != ProtoHD || msg.Xpub == "" {
		msigLog.Warnf("%s answered an HD round with a v1 accept; peer must upgrade", peer.Nick)
		return
	}
	// The accept must prove the offered key is held. A build that cannot
	// produce one predates cosigner attestation, and there is deliberately no
	// unattested path to fall back to: the initiator picks the ceremony, so a
	// tolerated legacy path would just become the way to avoid attestation.
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		msigLog.Error(err)
		return
	}
	if msg.Attest == "" {
		failRound(store, rec.TempID, fmt.Sprintf("%s answered without proof that they hold their key; ask them to upgrade dcrpulse", peer.Nick))
		return
	}
	pop := AttestPoPMessage(rec.Network, rec.TempID, msg.Xpub)
	if err := VerifyAttest(msg.Xpub, pop, msg.Attest, params); err != nil {
		failRound(store, rec.TempID, fmt.Sprintf("%s did not prove they hold the key they offered: %v", peer.Nick, err))
		return
	}
	// A cosigner that already delivered a key never changes it. On the
	// manual transport this is the misattribution backstop: crediting a
	// second acceptance to the wrong person must not overwrite the key
	// they really delivered.
	if peer.Xpub != "" && peer.Xpub != msg.Xpub {
		msigLog.Warnf("%s already delivered a different key for %s; frame ignored", peer.Nick, rec.TempID)
		return
	}
	dup := rec.OwnHD != nil && msg.Xpub == rec.OwnHD.Xpub
	for _, p := range rec.Peers {
		if p.UID != fromUID && p.Xpub == msg.Xpub {
			dup = true
		}
	}
	if dup {
		failRound(store, rec.TempID, "duplicate extended key offered by "+peer.Nick)
		return
	}
	err = store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
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
		msigLog.Error(err)
		return
	}
	maybeReviewInitiatorHD(ctx, store, rec.TempID)
}

// maybeReviewInitiatorHD derives the roster once every xpub is in and stops.
// It does not import or announce anything: the initiator signs off on the key
// set first, which needs the passphrase and therefore a human. Wallet-gated;
// the sweep resumes it after a switch.
func maybeReviewInitiatorHD(ctx context.Context, store *Store, tempID string) {
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
		msigLog.Infof("round %q complete; switch to wallet %q to activate it", rec.Label, store.WalletName())
		return
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		msigLog.Error(err)
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
		msigLog.Errorf("derive wallet id: %v", err)
		return
	}
	if err := store.UpdateWallet(tempID, func(r *WalletRecord) error {
		if r.Status != StatusInviting {
			return fmt.Errorf("round %s left the inviting state", r.TempID)
		}
		r.Xpubs = xpubs
		r.Address = walletID
		r.RosterDigest = AttestRosterMessage(r.Network, r.TempID, r.M, r.N, walletID, xpubs)
		r.Status = StatusReviewing
		return nil
	}); err != nil {
		msigLog.Error(err)
		return
	}
	msigLog.Infof("HD round %q has every key; review the cosigners and activate it", rec.Label)
}

// ActivateRound is the initiator's checkpoint. Every key is in and the address
// is derived, but nothing is imported or announced until the initiator signs
// the key set, which is also the moment it has to look at who is in it.
func ActivateRound(ctx context.Context, id string, passphrase []byte) error {
	defer zero(passphrase)
	network, err := networkSeam(ctx)
	if err != nil {
		return err
	}
	store, rec := manager(network).Route(id, func(r *WalletRecord) bool {
		return r.Role == RoleInitiator
	})
	if rec == nil {
		return fmt.Errorf("unknown shared wallet %s", id)
	}
	if store.WalletName() != activeWalletSeam() {
		return fmt.Errorf("this wallet belongs to %q; switch to it first", store.WalletName())
	}
	if rec.Status != StatusReviewing {
		return fmt.Errorf("this wallet is not waiting to be activated")
	}
	sig, err := signOwnAttest(ctx, rec, rec.OwnHD, rec.RosterDigest, passphrase)
	if err != nil {
		return err
	}
	if err := ensureWindowImported(ctx, store, rec.TempID); err != nil {
		return fmt.Errorf("import the ladder: %v", err)
	}
	tip, err := tipHeightSeam(ctx)
	if err != nil {
		return fmt.Errorf("read the chain tip: %v", err)
	}
	err = store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		if r.Status != StatusReviewing {
			return fmt.Errorf("this wallet is not waiting to be activated")
		}
		r.OwnAttest = sig
		r.CreatedHeight = tip
		r.Status = StatusActivating
		for _, p := range r.Peers {
			p.State = PeerRosterSent
		}
		return nil
	})
	if err != nil {
		return err
	}
	rec, _ = store.Wallet(rec.TempID)
	msg := rosterMessage(rec)
	for _, p := range rec.Peers {
		if err := sendFrame(store, p.UID, msg, ""); err != nil {
			msigLog.Warnf("roster to %s: %v", p.Nick, err)
		}
	}
	msigLog.Infof("HD shared wallet %q activating at %s", rec.Label, rec.Address)
	return nil
}

// rosterMessage builds the roster frame from a settled record. The peer
// tuples name the invitees so every cosigner learns the full membership,
// not just the initiator it heard the invite from. The initiator's own
// attestation rides along so a cosigner can check the sender committed to the
// same key set before it commits to anything itself.
func rosterMessage(rec *WalletRecord) *Message {
	msg := &Message{
		Type: TypeRoster, Ver: ProtoHD, TempID: rec.TempID, Label: rec.Label,
		M: rec.M, N: rec.N, Network: rec.Network, Xpubs: rec.Xpubs, Address: rec.Address,
		Attest: rec.OwnAttest,
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
	if rec.Status != StatusAccepted && rec.Status != StatusConfirming && rec.Status != StatusPendingImport {
		// A settled record accepts a byte-identical roster purely to
		// fill in peer identities a pre-tuple build never delivered.
		// Nothing else changes: membership settled at activation. The
		// repeat is also answered with a fresh ready, so an initiator
		// stuck activating on a lost ready recovers by re-announcing.
		// Attested is included: a cosigner that confirmed but whose ready
		// was lost is exactly the case this recovery exists for, and it
		// cannot reach active until the initiator hears that ready.
		if (rec.Status == StatusActive || rec.Status == StatusAttested) && rosterMatchesRecord(rec, msg) {
			if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
				return mergeRosterPeers(r, msg)
			}); err != nil {
				msigLog.Error(err)
			}
			// The stored signature rides the repeat: a ready without one
			// reads as a cosigner that never confirmed, and would fail
			// the very round this re-announce exists to recover.
			if err := sendFrame(store, rec.InitiatorUID, &Message{
				Type: TypeReady, TempID: rec.TempID, WalletID: rec.Address, Attest: rec.OwnAttest,
			}, ""); err != nil {
				msigLog.Warnf("ready: %v", err)
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
			msigLog.Error(err)
		}
		msigLog.Warnf("roster for %q rejected: %s", rec.Label, reason)
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
	// The initiator must have committed to this exact key set before this
	// node commits to anything. A roster whose sender will not sign it is one
	// the sender is not standing behind.
	digest := AttestRosterMessage(rec.Network, rec.TempID, msg.M, msg.N, msg.Address, msg.Xpubs)
	if msg.Attest == "" {
		fail("the initiator did not sign this roster; ask them to upgrade dcrpulse")
		return
	}
	if err := VerifyAttest(rec.Peers[0].Xpub, digest, msg.Attest, params); err != nil {
		fail(fmt.Sprintf("the initiator's signature over this roster is not valid: %v", err))
		return
	}
	if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		r.Xpubs = append([]string(nil), msg.Xpubs...)
		r.Address = msg.Address
		r.RosterDigest = digest
		r.Status = StatusConfirming
		if p := r.peerByUID(fromUID); p != nil {
			p.AttestSig = msg.Attest
		}
		return mergeRosterPeers(r, msg)
	}); err != nil {
		msigLog.Error(err)
		return
	}
	msigLog.Infof("roster for %q verified; confirm the cosigners to finish joining", rec.Label)
}

// ConfirmRoster is the cosigner's checkpoint. The roster verified, but a
// verified roster only proves the keys derive the address they claim - not who
// holds them. Signing it is the user saying they checked that, so it needs the
// passphrase and therefore a human. See attest.go for what this does and does
// not prove.
func ConfirmRoster(ctx context.Context, id string, passphrase []byte) error {
	defer zero(passphrase)
	network, err := networkSeam(ctx)
	if err != nil {
		return err
	}
	store, rec := manager(network).Route(id, func(r *WalletRecord) bool {
		return r.Role == RoleCosigner
	})
	if rec == nil {
		return fmt.Errorf("unknown shared wallet %s", id)
	}
	if store.WalletName() != activeWalletSeam() {
		return fmt.Errorf("this wallet belongs to %q; switch to it first", store.WalletName())
	}
	if rec.Status != StatusConfirming {
		return fmt.Errorf("this wallet is not waiting to be confirmed")
	}
	sig, err := signOwnAttest(ctx, rec, rec.OwnHD, rec.RosterDigest, passphrase)
	if err != nil {
		return err
	}
	if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		if r.Status != StatusConfirming {
			return fmt.Errorf("this wallet is not waiting to be confirmed")
		}
		r.OwnAttest = sig
		r.Status = StatusAttested
		return nil
	}); err != nil {
		return err
	}
	if err := sendFrame(store, rec.InitiatorUID, &Message{
		Type: TypeReady, TempID: rec.TempID, WalletID: rec.Address, Attest: sig,
	}, ""); err != nil {
		return err
	}
	msigLog.Infof("confirmed the cosigners for %q; waiting for the others", rec.Label)
	// An attestation set that arrived early is acted on now, never before.
	maybeCompleteAttested(ctx, store, rec.TempID)
	return nil
}

// maybeCompleteAttested imports the ladder once this node has both confirmed
// the roster itself and verified every other cosigner's signature over it.
// Until both hold, the record stays short of active and no receive address is
// handed out.
func maybeCompleteAttested(ctx context.Context, store *Store, tempID string) {
	rec, ok := store.Wallet(tempID)
	if !ok || !rec.HD || rec.Status != StatusAttested || len(rec.Attests) == 0 {
		return
	}
	if err := store.UpdateWallet(tempID, func(r *WalletRecord) error {
		if r.Status != StatusAttested {
			return fmt.Errorf("wallet %s left the attested state", r.TempID)
		}
		r.Status = StatusPendingImport
		return nil
	}); err != nil {
		msigLog.Error(err)
		return
	}
	completeCosignerImportHD(ctx, store, tempID)
}

// inboundAttestSet accepts the initiator's collected attestations. Every
// signature is checked against this node's OWN roster digest, so a set gathered
// over a different key set cannot satisfy it, and every key must be covered, so
// a slot nobody signed for cannot slip through.
func inboundAttestSet(ctx context.Context, store *Store, rec *WalletRecord, msg *Message, fromUID string) {
	if rec.Role != RoleCosigner || fromUID != rec.InitiatorUID || rec.Terminal() {
		return
	}
	if rec.Status != StatusConfirming && rec.Status != StatusAttested {
		return
	}
	if msg.WalletID != rec.Address {
		msigLog.Warnf("attestation set for %q names the wrong address", rec.Label)
		return
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		msigLog.Error(err)
		return
	}
	if err := VerifyRosterAttests(rec, msg.Attests, params); err != nil {
		failRound(store, rec.TempID, fmt.Sprintf("the cosigner signatures for this wallet did not check out: %v", err))
		return
	}
	if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		r.Attests = append([]RosterAttest(nil), msg.Attests...)
		for _, p := range r.Peers {
			if a, ok := attestFor(msg.Attests, p.Xpub); ok {
				p.AttestSig = a.Sig
			}
		}
		return nil
	}); err != nil {
		msigLog.Error(err)
		return
	}
	// Arriving before this node has confirmed is fine and expected; the set is
	// stored and ConfirmRoster picks it up. The human gate is never skipped.
	maybeCompleteAttested(ctx, store, rec.TempID)
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
		msigLog.Infof("shared wallet %q waits for wallet %q to import its ladder", rec.Label, store.WalletName())
		return
	}
	if err := ensureWindowImported(ctx, store, tempID); err != nil {
		msigLog.Warnf("ladder import deferred: %v", err)
		return
	}
	tip, err := tipHeightSeam(ctx)
	if err != nil {
		msigLog.Warnf("tip height: %v", err)
		return
	}
	err = store.UpdateWallet(tempID, func(r *WalletRecord) error {
		r.CreatedHeight = tip
		r.Status = StatusActive
		return nil
	})
	if err != nil {
		msigLog.Error(err)
		return
	}
	rec, _ = store.Wallet(tempID)
	// No ready is sent here: ConfirmRoster already sent it, carrying the
	// signature that made this import possible in the first place. A lost
	// ready is recovered by the roster re-announce, which answers with the
	// stored signature rather than a bare frame.
	msigLog.Infof("HD shared wallet %q active at %s", rec.Label, rec.Address)
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
