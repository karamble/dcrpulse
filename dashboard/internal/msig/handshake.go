// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"fmt"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
)

// Wallet and transport calls go through package seams so the whole
// handshake is assertion-testable without a wallet or a relay.
var (
	importScriptSeam = services.ImportMsigScript
	tipHeightSeam    = services.WalletTipHeight
	sendPMSeam       = rpc.BrclientdSendPM
	msigHistorySeam  = rpc.BrclientdMsigHistory
	activeWalletSeam = services.CurrentWalletName
	networkSeam      = services.CurrentNetwork
)

const walletCallTimeout = 60 * time.Second

func paramsForNetwork(network string) (*chaincfg.Params, error) {
	switch network {
	case "mainnet":
		return chaincfg.MainNetParams(), nil
	case "testnet":
		return chaincfg.TestNet3Params(), nil
	case "simnet":
		return chaincfg.SimNetParams(), nil
	}
	return nil, fmt.Errorf("unknown network %q", network)
}

// sendFrame persists one frame to the store's outbox and attempts the
// send. fixedMID lets the invite reuse its tempId as the frame id; pass
// "" for a fresh id. Transport failures are not errors: the item stays
// queued and the sweep resends it byte-identical. Frames of a manual
// wallet are never sent at all: they wait in the outbox for the user to
// hand them over, and the record-changed signal makes the export card
// appear at once.
func sendFrame(store *Store, toUID string, msg *Message, fixedMID string) error {
	payload, err := EncodeMessage(msg)
	if err != nil {
		return err
	}
	mid := fixedMID
	if mid == "" {
		if mid, err = NewID(); err != nil {
			return err
		}
	}
	// Every message names its record (tempId for the handshake, walletId
	// after activation), and terminal records still resolve, so the
	// transport is always derivable here without widening the signature.
	recID := msg.TempID
	if recID == "" {
		recID = msg.WalletID
	}
	manual := false
	if rec, ok := store.Wallet(recID); ok {
		manual = rec.ManualTransport()
		recID = rec.TempID
	}
	body, err := Encode(payload, mid, time.Now().Add(TTLFor(msg.Type, manual)))
	if err != nil {
		return err
	}
	if err := store.AppendOutbox(&OutboxItem{
		MID: mid, ToUID: toUID, Body: body, State: OutboxSending, Ts: time.Now().Unix(),
		Manual: manual, Type: msg.Type, RecID: recID, TxID: msg.TxID,
	}); err != nil {
		return err
	}
	if manual {
		if rec, ok := store.Wallet(recID); ok {
			notifyRecordChanged(rec)
		}
		return nil
	}
	deliverOutbox(store, mid, toUID, body)
	return nil
}

func deliverOutbox(store *Store, mid, toUID, body string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := sendPMSeam(ctx, toUID, body); err != nil {
		msigLog.Warnf("send to %s deferred: %v", shortUID(toUID), err)
		return
	}
	if err := store.MarkOutboxSent(mid, toUID); err != nil {
		msigLog.Errorf("mark outbox sent: %v", err)
	}
}

func shortUID(uid string) string {
	if len(uid) > 12 {
		return uid[:12]
	}
	return uid
}

// InviteePeer names one Bison Relay contact to invite.
type InviteePeer struct {
	UID  string
	Nick string
}

// DeclineInvite answers an incoming invite negatively.
func DeclineInvite(ctx context.Context, id, reason string) error {
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
	if rec.Status != StatusInvited {
		return fmt.Errorf("invite is not pending")
	}
	reason = clampReason(reason)
	err = store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		r.Status = StatusDeclined
		r.FailReason = "declined locally"
		return nil
	})
	if err != nil {
		return err
	}
	return sendFrame(store, rec.InitiatorUID,
		&Message{Type: TypeDecline, TempID: rec.TempID, Reason: reason}, "")
}

// CancelRound withdraws a round that has not activated yet and tells
// every peer.
func CancelRound(ctx context.Context, id string) error {
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
	if rec.Status != StatusInviting {
		return fmt.Errorf("only a pending invite round can be cancelled")
	}
	err = store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		r.Status = StatusFailed
		r.FailReason = "cancelled locally"
		return nil
	})
	if err != nil {
		return err
	}
	cancel := &Message{Type: TypeInviteCancel, TempID: rec.TempID}
	for _, p := range rec.Peers {
		if p.State == PeerDeclined {
			continue
		}
		// Keep fanning out past a failed peer like every other fan-out;
		// stopping early would leave later peers waiting on a dead round.
		if err := sendFrame(store, p.UID, cancel, ""); err != nil {
			msigLog.Warnf("cancel to %s: %v", p.Nick, err)
		}
	}
	return nil
}

// dispatchInbound routes one validated message to its handler. Frames
// referencing records this node does not know are dropped WITHOUT being
// journaled, so a later restore or activation lets catch-up process them.
func dispatchInbound(msg *Message, frame *Frame, fromUID, fromNick string, now time.Time, imported bool) {
	ctx, cancel := context.WithTimeout(context.Background(), walletCallTimeout)
	defer cancel()
	network, err := networkSeam(ctx)
	if err != nil {
		msigLog.Warnf("network unavailable, frame %s left for catch-up: %v", frame.MID, err)
		return
	}
	m := manager(network)
	switch msg.Type {
	case TypeInvite:
		inboundInvite(ctx, m, msg, frame, fromUID, fromNick, network, now, imported)
	case TypeAccept, TypeDecline, TypeRoster, TypeReady, TypeInviteCancel, TypeAttestSet:
		inboundHandshake(ctx, m, msg, frame, fromUID, fromNick, now)
	case TypeSignReq, TypeSig, TypeSigDecline, TypeBroadcast:
		inboundSpend(ctx, m, msg, frame, fromUID, fromNick, now)
	}
}

func inboundInvite(ctx context.Context, m *Manager, msg *Message, frame *Frame, fromUID, fromNick, network string, now time.Time, imported bool) {
	store, err := m.StoreFor(activeWalletSeam())
	if err != nil {
		msigLog.Errorf("open store: %v", err)
		return
	}
	fresh, err := store.MarkProcessed(frame.MID, now)
	if err != nil {
		msigLog.Warnf("journal: %v", err)
		return
	}
	if !fresh {
		return
	}
	if msg.Network != network {
		msigLog.Warnf("dropping cross-network invite from %s (%s)", fromNick, msg.Network)
		return
	}
	if msg.Ver != ProtoHD {
		msigLog.Warnf("dropping pre-HD invite from %s; the peer must upgrade", fromNick)
		return
	}
	if _, exists := store.Wallet(msg.TempID); exists {
		msigLog.Warnf("duplicate invite round %s from %s", msg.TempID, fromNick)
		return
	}
	transport := ""
	if imported {
		transport = TransportManual
	}
	rec := &WalletRecord{
		TempID: msg.TempID, Label: msg.Label, M: msg.M, N: msg.N, Network: msg.Network,
		HD: true, Transport: transport,
		Role: RoleCosigner, Status: StatusInvited, InitiatorUID: fromUID,
		Peers: []*Peer{{UID: fromUID, Nick: fromNick, Xpub: msg.Xpub, State: PeerAccepted, LastSeenTs: now.Unix()}},
		// Who the initiator says the other cosigners are. Shown to the
		// invitee before it accepts, and never treated as more than a claim.
		ProposedPeers: append([]RosterPeer(nil), msg.Peers...),
	}
	if err := store.PutWallet(rec); err != nil {
		msigLog.Errorf("store invite: %v", err)
		return
	}
	msigLog.Infof("invite %q (%d-of-%d) received from %s", msg.Label, msg.M, msg.N, fromNick)
}

func inboundHandshake(ctx context.Context, m *Manager, msg *Message, frame *Frame, fromUID, fromNick string, now time.Time) {
	// Role-aware routing: both sides of a round share the tempId when a
	// user cosigns between two of their own wallets, so the message type
	// picks which side it addresses and roster-side frames must come
	// from the round's initiator.
	var match func(*WalletRecord) bool
	switch msg.Type {
	case TypeAccept, TypeDecline, TypeReady:
		match = func(r *WalletRecord) bool { return r.Role == RoleInitiator }
	case TypeRoster, TypeInviteCancel, TypeAttestSet:
		match = func(r *WalletRecord) bool {
			return r.Role == RoleCosigner && r.InitiatorUID == fromUID
		}
	}
	store, rec := m.Route(msg.TempID, match)
	if rec == nil {
		msigLog.Warnf("%s frame for unknown round %s from %s, left for catch-up", msg.Type, msg.TempID, fromNick)
		return
	}
	fresh, err := store.MarkProcessed(frame.MID, now)
	if err != nil {
		msigLog.Warnf("journal: %v", err)
		return
	}
	if !fresh {
		return
	}
	switch msg.Type {
	case TypeAccept:
		inboundAcceptHD(ctx, store, rec, msg, fromUID, now)
	case TypeDecline:
		inboundDecline(store, rec, msg, fromUID, fromNick)
	case TypeRoster:
		inboundRosterHD(ctx, store, rec, msg, fromUID)
	case TypeReady:
		inboundReady(store, rec, msg, fromUID, now)
	case TypeInviteCancel:
		inboundCancel(store, rec, fromUID)
	case TypeAttestSet:
		inboundAttestSet(ctx, store, rec, msg, fromUID)
	}
}

func failRound(store *Store, tempID, reason string) {
	if err := store.UpdateWallet(tempID, func(r *WalletRecord) error {
		r.Status = StatusFailed
		r.FailReason = reason
		return nil
	}); err != nil {
		msigLog.Error(err)
	}
	msigLog.Warnf("round %s failed: %s", tempID, reason)
}

func inboundDecline(store *Store, rec *WalletRecord, msg *Message, fromUID, fromNick string) {
	if rec.Role != RoleInitiator || rec.Terminal() {
		return
	}
	if rec.peerByUID(fromUID) == nil {
		return
	}
	if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		if p := r.peerByUID(fromUID); p != nil {
			p.State = PeerDeclined
			p.Reason = msg.Reason
		}
		r.Status = StatusFailed
		r.FailReason = "declined by " + fromNick
		return nil
	}); err != nil {
		msigLog.Error(err)
	}
	msigLog.Infof("round %s declined by %s", rec.TempID, fromNick)
}

func inboundReady(store *Store, rec *WalletRecord, msg *Message, fromUID string, now time.Time) {
	if rec.Role != RoleInitiator || rec.Terminal() {
		return
	}
	if rec.Status != StatusActivating && rec.Status != StatusActive {
		return
	}
	if msg.WalletID != rec.Address {
		msigLog.Warnf("ready for %s names the wrong address", rec.TempID)
		return
	}
	peer := rec.peerByUID(fromUID)
	if peer == nil {
		return
	}
	// A ready is a cosigner saying it confirmed the key set, so it has to
	// carry the signature that proves it. Crediting one without would let a
	// wallet go active on a roster somebody never actually signed off on.
	if rec.HD {
		params, err := paramsForNetwork(rec.Network)
		if err != nil {
			msigLog.Error(err)
			return
		}
		if msg.Attest == "" {
			failRound(store, rec.TempID, fmt.Sprintf("%s confirmed without signing the cosigner list; ask them to upgrade dcrpulse", peer.Nick))
			return
		}
		if err := VerifyAttest(peer.Xpub, rec.RosterDigest, msg.Attest, params); err != nil {
			failRound(store, rec.TempID, fmt.Sprintf("%s's signature over the cosigner list is not valid: %v", peer.Nick, err))
			return
		}
	}
	err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		p := r.peerByUID(fromUID)
		if p == nil {
			return fmt.Errorf("unknown peer")
		}
		p.State = PeerReady
		p.AttestSig = msg.Attest
		p.LastSeenTs = now.Unix()
		allReady := true
		for _, q := range r.Peers {
			if q.State != PeerReady {
				allReady = false
			}
		}
		if allReady && r.Status == StatusActivating {
			r.Status = StatusActive
			r.Attests = collectAttests(r)
		}
		return nil
	})
	if err != nil {
		msigLog.Error(err)
		return
	}
	updated, ok := store.Wallet(rec.TempID)
	if !ok || updated.Status != StatusActive || rec.Status == StatusActive {
		return
	}
	msigLog.Infof("shared wallet %q active at %s", updated.Label, updated.Address)
	// Everyone signed; hand each cosigner the full set so they can finish.
	fanOutAttestSet(store, updated)
}

// collectAttests gathers the verified signatures into roster order. Only
// signatures that already passed verification are ever stored, so this reads
// them rather than re-checking.
func collectAttests(r *WalletRecord) []RosterAttest {
	out := make([]RosterAttest, 0, len(r.Xpubs))
	if r.OwnHD != nil && r.OwnAttest != "" {
		out = append(out, RosterAttest{Xpub: r.OwnHD.Xpub, Sig: r.OwnAttest})
	}
	for _, p := range r.Peers {
		if p.Xpub != "" && p.AttestSig != "" {
			out = append(out, RosterAttest{Xpub: p.Xpub, Sig: p.AttestSig})
		}
	}
	return out
}

// fanOutAttestSet sends every cosigner the collected attestations, which is
// what releases them to import the ladder.
func fanOutAttestSet(store *Store, rec *WalletRecord) {
	if len(rec.Attests) == 0 {
		return
	}
	msg := &Message{
		Type: TypeAttestSet, TempID: rec.TempID, WalletID: rec.Address,
		Attests: rec.Attests,
	}
	for _, p := range rec.Peers {
		if err := sendFrame(store, p.UID, msg, ""); err != nil {
			msigLog.Warnf("attestation set to %s: %v", p.Nick, err)
		}
	}
}

func inboundCancel(store *Store, rec *WalletRecord, fromUID string) {
	if rec.Role != RoleCosigner || fromUID != rec.InitiatorUID {
		return
	}
	if rec.Terminal() || rec.Status == StatusActive {
		return
	}
	if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		r.Status = StatusFailed
		r.FailReason = "cancelled by the initiator"
		return nil
	}); err != nil {
		msigLog.Error(err)
	}
}

// ResumePending re-attempts wallet-gated steps for the active wallet:
// initiator activation waiting on a wallet switch, verified rosters
// waiting on their script import, and active HD ladders whose windows
// fell behind their cursors.
func ResumePending(ctx context.Context) {
	network, err := networkSeam(ctx)
	if err != nil {
		return
	}
	store, err := manager(network).StoreFor(activeWalletSeam())
	if err != nil {
		return
	}
	for _, rec := range store.Wallets() {
		if !rec.HD {
			continue
		}
		switch rec.Status {
		case StatusInviting:
			maybeReviewInitiatorHD(ctx, store, rec.TempID)
		case StatusPendingImport:
			completeCosignerImportHD(ctx, store, rec.TempID)
		case StatusActive:
			if _, err := ensureWindowImported(ctx, store, rec.TempID); err != nil {
				msigLog.Warnf("window catch-up for %q: %v", rec.Label, err)
			}
		}
	}
}
