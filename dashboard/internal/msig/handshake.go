// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"fmt"
	"log"
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
		log.Printf("msig: send to %s deferred: %v", shortUID(toUID), err)
		return
	}
	if err := store.MarkOutboxSent(mid, toUID); err != nil {
		log.Printf("msig: mark outbox sent: %v", err)
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
	if len(reason) > MaxReasonLen {
		reason = reason[:MaxReasonLen]
	}
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
		if err := sendFrame(store, p.UID, cancel, ""); err != nil {
			return err
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
		log.Printf("msig: network unavailable, frame %s left for catch-up: %v", frame.MID, err)
		return
	}
	m := manager(network)
	switch msg.Type {
	case TypeInvite:
		inboundInvite(ctx, m, msg, frame, fromUID, fromNick, network, now, imported)
	case TypeAccept, TypeDecline, TypeRoster, TypeReady, TypeInviteCancel:
		inboundHandshake(ctx, m, msg, frame, fromUID, fromNick, now)
	case TypeSignReq, TypeSig, TypeSigDecline, TypeBroadcast:
		inboundSpend(ctx, m, msg, frame, fromUID, fromNick, now)
	}
}

func inboundInvite(ctx context.Context, m *Manager, msg *Message, frame *Frame, fromUID, fromNick, network string, now time.Time, imported bool) {
	store, err := m.StoreFor(activeWalletSeam())
	if err != nil {
		log.Printf("msig: open store: %v", err)
		return
	}
	fresh, err := store.MarkProcessed(frame.MID, now)
	if err != nil {
		log.Printf("msig: journal: %v", err)
		return
	}
	if !fresh {
		return
	}
	if msg.Network != network {
		log.Printf("msig: dropping cross-network invite from %s (%s)", fromNick, msg.Network)
		return
	}
	if msg.Ver != ProtoHD {
		log.Printf("msig: dropping pre-HD invite from %s; the peer must upgrade", fromNick)
		return
	}
	if _, exists := store.Wallet(msg.TempID); exists {
		log.Printf("msig: duplicate invite round %s from %s", msg.TempID, fromNick)
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
	}
	if err := store.PutWallet(rec); err != nil {
		log.Printf("msig: store invite: %v", err)
		return
	}
	log.Printf("msig: invite %q (%d-of-%d) received from %s", msg.Label, msg.M, msg.N, fromNick)
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
	case TypeRoster, TypeInviteCancel:
		match = func(r *WalletRecord) bool {
			return r.Role == RoleCosigner && r.InitiatorUID == fromUID
		}
	}
	store, rec := m.Route(msg.TempID, match)
	if rec == nil {
		log.Printf("msig: %s frame for unknown round %s from %s, left for catch-up", msg.Type, msg.TempID, fromNick)
		return
	}
	fresh, err := store.MarkProcessed(frame.MID, now)
	if err != nil {
		log.Printf("msig: journal: %v", err)
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
	}
}

func failRound(store *Store, tempID, reason string) {
	if err := store.UpdateWallet(tempID, func(r *WalletRecord) error {
		r.Status = StatusFailed
		r.FailReason = reason
		return nil
	}); err != nil {
		log.Printf("msig: %v", err)
	}
	log.Printf("msig: round %s failed: %s", tempID, reason)
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
		log.Printf("msig: %v", err)
	}
	log.Printf("msig: round %s declined by %s", rec.TempID, fromNick)
}

func inboundReady(store *Store, rec *WalletRecord, msg *Message, fromUID string, now time.Time) {
	if rec.Role != RoleInitiator || rec.Terminal() {
		return
	}
	if rec.Status != StatusActivating && rec.Status != StatusActive {
		return
	}
	if msg.WalletID != rec.Address {
		log.Printf("msig: ready for %s names the wrong address", rec.TempID)
		return
	}
	if rec.peerByUID(fromUID) == nil {
		return
	}
	err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		p := r.peerByUID(fromUID)
		if p == nil {
			return fmt.Errorf("unknown peer")
		}
		p.State = PeerReady
		p.LastSeenTs = now.Unix()
		allReady := true
		for _, q := range r.Peers {
			if q.State != PeerReady {
				allReady = false
			}
		}
		if allReady && r.Status == StatusActivating {
			r.Status = StatusActive
		}
		return nil
	})
	if err != nil {
		log.Printf("msig: %v", err)
		return
	}
	if updated, ok := store.Wallet(rec.TempID); ok && updated.Status == StatusActive && rec.Status != StatusActive {
		log.Printf("msig: shared wallet %q active at %s", updated.Label, updated.Address)
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
		log.Printf("msig: %v", err)
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
			maybeActivateInitiatorHD(ctx, store, rec.TempID)
		case StatusPendingImport:
			completeCosignerImportHD(ctx, store, rec.TempID)
		case StatusActive:
			if err := ensureWindowImported(ctx, store, rec.TempID); err != nil {
				log.Printf("msig: window catch-up for %q: %v", rec.Label, err)
			}
		}
	}
}
