// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"encoding/hex"
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
	deriveKeySeam = func(ctx context.Context, account uint32) (*OwnKey, error) {
		k, err := services.DeriveMsigKey(ctx, account)
		if err != nil {
			return nil, err
		}
		return &OwnKey{PubKey: k.PubKey, Address: k.Address, Account: k.Account, Branch: k.Branch, Index: k.Index}, nil
	}
	importScriptSeam = services.ImportMsigScript
	tipHeightSeam    = services.WalletTipHeight
	sendPMSeam       = rpc.BrclientdSendPM
	msigHistorySeam  = rpc.BrclientdMsigHistory
	activeWalletSeam = services.CurrentWalletName
	networkSeam      = services.CurrentNetwork

	// verifyOwnKeySeam proves a restored key belongs to this wallet's
	// seed. The address never appears on-chain as a plain payment, so
	// the cursor must be advanced before the wallet recognizes it.
	verifyOwnKeySeam = func(ctx context.Context, own *OwnKey) error {
		accounts, err := services.FetchAllAccounts(ctx)
		if err != nil {
			return err
		}
		accountName := ""
		for _, a := range accounts {
			if a.AccountNumber == own.Account {
				accountName = a.AccountName
			}
		}
		if accountName == "" {
			return fmt.Errorf("account %d from the backup does not exist in this wallet", own.Account)
		}
		if err := services.SyncAccountAddressIndex(ctx, accountName, own.Branch, own.Index+1); err != nil {
			return fmt.Errorf("could not advance the address cursor: %v", err)
		}
		res, err := services.ValidateAddress(ctx, own.Address)
		if err != nil {
			return err
		}
		if !res.IsMine {
			return fmt.Errorf("this wallet does not own the backup's key; restore it into the wallet whose seed created it")
		}
		return nil
	}
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
// queued and the sweep resends it byte-identical.
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
	body, err := Encode(payload, mid, time.Now().Add(TTLFor(msg.Type)))
	if err != nil {
		return err
	}
	if err := store.AppendOutbox(&OutboxItem{
		MID: mid, ToUID: toUID, Body: body, State: OutboxSending, Ts: time.Now().Unix(),
	}); err != nil {
		return err
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

// CreateSharedWallet derives this wallet's key, records the round and
// sends every invite. The invitees plus this wallet form the n keys.
func CreateSharedWallet(ctx context.Context, label string, m int, invitees []InviteePeer, account uint32) (*WalletRecord, error) {
	network, err := networkSeam(ctx)
	if err != nil {
		return nil, err
	}
	n := len(invitees) + 1
	if label == "" || len(label) > MaxLabelLen {
		return nil, fmt.Errorf("label must be 1 to %d characters", MaxLabelLen)
	}
	if !validParams(m, n) {
		return nil, fmt.Errorf("invalid scheme: %d-of-%d", m, n)
	}
	seen := make(map[string]bool, len(invitees))
	for _, p := range invitees {
		if p.UID == "" {
			return nil, fmt.Errorf("invitee without uid")
		}
		if seen[p.UID] {
			return nil, fmt.Errorf("duplicate invitee")
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
	own, err := deriveKeySeam(ctx, account)
	if err != nil {
		return nil, err
	}
	peers := make([]*Peer, 0, len(invitees))
	for _, p := range invitees {
		peers = append(peers, &Peer{UID: p.UID, Nick: p.Nick, State: PeerInvited})
	}
	rec := &WalletRecord{
		TempID: tempID, Label: label, M: m, N: n, Network: network,
		Role: RoleInitiator, Status: StatusInviting, Own: own, Peers: peers,
	}
	if err := store.PutWallet(rec); err != nil {
		return nil, err
	}
	invite := &Message{
		Type: TypeInvite, TempID: tempID, Label: label, M: m, N: n,
		Network: network, PubKey: own.PubKey,
	}
	for _, p := range invitees {
		if err := sendFrame(store, p.UID, invite, tempID); err != nil {
			return nil, err
		}
	}
	out, _ := store.Wallet(tempID)
	return out, nil
}

// AcceptInvite derives the local key for an incoming invite and answers
// the initiator. The invite must belong to the active wallet because the
// key is derived from it.
func AcceptInvite(ctx context.Context, id string, account uint32) error {
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
		return fmt.Errorf("this invite belongs to wallet %q; switch to it first", store.WalletName())
	}
	if rec.Status != StatusInvited {
		return fmt.Errorf("invite is not pending")
	}
	own, err := deriveKeySeam(ctx, account)
	if err != nil {
		return err
	}
	err = store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		if r.Status != StatusInvited {
			return fmt.Errorf("invite is not pending")
		}
		r.Own = own
		r.Status = StatusAccepted
		return nil
	})
	if err != nil {
		return err
	}
	return sendFrame(store, rec.InitiatorUID,
		&Message{Type: TypeAccept, TempID: rec.TempID, PubKey: own.PubKey}, "")
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
func dispatchInbound(msg *Message, frame *Frame, fromUID, fromNick string, now time.Time) {
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
		inboundInvite(ctx, m, msg, frame, fromUID, fromNick, network, now)
	case TypeAccept, TypeDecline, TypeRoster, TypeReady, TypeInviteCancel:
		inboundHandshake(ctx, m, msg, frame, fromUID, fromNick, now)
	case TypeSignReq, TypeSig, TypeSigDecline, TypeBroadcast:
		inboundSpend(ctx, m, msg, frame, fromUID, fromNick, now)
	}
}

func inboundInvite(ctx context.Context, m *Manager, msg *Message, frame *Frame, fromUID, fromNick, network string, now time.Time) {
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
	if _, exists := store.Wallet(msg.TempID); exists {
		log.Printf("msig: duplicate invite round %s from %s", msg.TempID, fromNick)
		return
	}
	rec := &WalletRecord{
		TempID: msg.TempID, Label: msg.Label, M: msg.M, N: msg.N, Network: msg.Network,
		HD:   msg.Ver == ProtoHD,
		Role: RoleCosigner, Status: StatusInvited, InitiatorUID: fromUID,
		Peers: []*Peer{{UID: fromUID, Nick: fromNick, PubKey: msg.PubKey, Xpub: msg.Xpub, State: PeerAccepted, LastSeenTs: now.Unix()}},
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
		if rec.HD {
			inboundAcceptHD(ctx, store, rec, msg, fromUID, now)
		} else {
			inboundAccept(ctx, store, rec, msg, fromUID, now)
		}
	case TypeDecline:
		inboundDecline(store, rec, msg, fromUID, fromNick)
	case TypeRoster:
		if rec.HD {
			inboundRosterHD(ctx, store, rec, msg, fromUID)
		} else {
			inboundRoster(ctx, store, rec, msg, fromUID, now)
		}
	case TypeReady:
		inboundReady(store, rec, msg, fromUID, now)
	case TypeInviteCancel:
		inboundCancel(store, rec, fromUID)
	}
}

func inboundAccept(ctx context.Context, store *Store, rec *WalletRecord, msg *Message, fromUID string, now time.Time) {
	if rec.Role != RoleInitiator || rec.Terminal() || rec.Status != StatusInviting {
		return
	}
	peer := rec.peerByUID(fromUID)
	if peer == nil {
		log.Printf("msig: accept for %s from a non-invited uid", rec.TempID)
		return
	}
	dupKey := msg.PubKey == rec.Own.PubKey
	for _, p := range rec.Peers {
		if p.UID != fromUID && p.PubKey == msg.PubKey {
			dupKey = true
		}
	}
	if dupKey {
		if err := store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
			r.Status = StatusFailed
			r.FailReason = "duplicate key offered by " + peer.Nick
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
		p.PubKey = msg.PubKey
		p.State = PeerAccepted
		p.LastSeenTs = now.Unix()
		return nil
	})
	if err != nil {
		log.Printf("msig: %v", err)
		return
	}
	maybeActivateInitiator(ctx, store, rec.TempID)
}

// maybeActivateInitiator builds and imports the script once every peer
// has accepted, then fans the roster out. Runs only when the owning
// wallet is active; otherwise the sweep resumes it after a switch.
func maybeActivateInitiator(ctx context.Context, store *Store, tempID string) {
	rec, ok := store.Wallet(tempID)
	if !ok || rec.Status != StatusInviting {
		return
	}
	for _, p := range rec.Peers {
		if p.State != PeerAccepted {
			return
		}
	}
	if store.WalletName() != activeWalletSeam() {
		log.Printf("msig: round %s ready to activate; switch to wallet %q", tempID, store.WalletName())
		return
	}
	keys := make([][]byte, 0, rec.N)
	ownKey, err := hex.DecodeString(rec.Own.PubKey)
	if err != nil {
		log.Printf("msig: own key: %v", err)
		return
	}
	keys = append(keys, ownKey)
	for _, p := range rec.Peers {
		pk, err := hex.DecodeString(p.PubKey)
		if err != nil {
			log.Printf("msig: peer key: %v", err)
			return
		}
		keys = append(keys, pk)
	}
	script, sorted, err := MultiSigScript(rec.M, keys)
	if err != nil {
		failRound(store, tempID, "script build failed: "+err.Error())
		return
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		failRound(store, tempID, err.Error())
		return
	}
	address, err := P2SHAddress(script, params)
	if err != nil {
		failRound(store, tempID, "address build failed: "+err.Error())
		return
	}
	scriptHex := hex.EncodeToString(script)
	if err := importScriptSeam(ctx, scriptHex, false, 0); err != nil {
		log.Printf("msig: import script for %s deferred: %v", tempID, err)
		return
	}
	tip, err := tipHeightSeam(ctx)
	if err != nil {
		log.Printf("msig: tip height: %v", err)
	}
	sortedHex := make([]string, len(sorted))
	for i, k := range sorted {
		sortedHex[i] = hex.EncodeToString(k)
	}
	err = store.UpdateWallet(tempID, func(r *WalletRecord) error {
		r.Address = address
		r.ScriptHex = scriptHex
		r.RosterPubKeys = sortedHex
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
	roster := &Message{
		Type: TypeRoster, TempID: tempID, Label: rec.Label, M: rec.M, N: rec.N,
		Network: rec.Network, PubKeys: sortedHex, Script: scriptHex, Address: address,
	}
	for _, p := range rec.Peers {
		if err := sendFrame(store, p.UID, roster, ""); err != nil {
			log.Printf("msig: roster to %s: %v", p.Nick, err)
		}
	}
	log.Printf("msig: shared wallet %q activating at %s", rec.Label, address)
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

func inboundRoster(ctx context.Context, store *Store, rec *WalletRecord, msg *Message, fromUID string, now time.Time) {
	if rec.Role != RoleCosigner || fromUID != rec.InitiatorUID {
		return
	}
	if rec.Status != StatusAccepted && rec.Status != StatusPendingImport {
		return
	}
	if msg.M != rec.M || msg.N != rec.N || msg.Network != rec.Network || msg.Label != rec.Label {
		failRound(store, rec.TempID, "roster does not match the invite")
		return
	}
	ownIn := false
	initiatorIn := false
	keys := make([][]byte, 0, len(msg.PubKeys))
	for _, pkHex := range msg.PubKeys {
		if pkHex == rec.Own.PubKey {
			ownIn = true
		}
		if pkHex == rec.Peers[0].PubKey {
			initiatorIn = true
		}
		pk, err := hex.DecodeString(pkHex)
		if err != nil {
			failRound(store, rec.TempID, "roster carries an invalid key")
			return
		}
		keys = append(keys, pk)
	}
	if !ownIn || !initiatorIn {
		failRound(store, rec.TempID, "roster omits an expected key")
		return
	}
	script, _, err := MultiSigScript(msg.M, keys)
	if err != nil {
		failRound(store, rec.TempID, "roster script rebuild failed: "+err.Error())
		return
	}
	if hex.EncodeToString(script) != msg.Script {
		failRound(store, rec.TempID, "roster script does not match the key set")
		return
	}
	params, err := paramsForNetwork(rec.Network)
	if err != nil {
		failRound(store, rec.TempID, err.Error())
		return
	}
	address, err := P2SHAddress(script, params)
	if err != nil || address != msg.Address {
		failRound(store, rec.TempID, "roster address does not match the script")
		return
	}
	err = store.UpdateWallet(rec.TempID, func(r *WalletRecord) error {
		r.Address = address
		r.ScriptHex = msg.Script
		r.RosterPubKeys = append([]string(nil), msg.PubKeys...)
		r.Status = StatusPendingImport
		if p := r.peerByUID(fromUID); p != nil {
			p.LastSeenTs = now.Unix()
		}
		return nil
	})
	if err != nil {
		log.Printf("msig: %v", err)
		return
	}
	completeCosignerImport(ctx, store, rec.TempID)
}

// completeCosignerImport imports the verified roster's script and reports
// readiness. Runs only when the owning wallet is active.
func completeCosignerImport(ctx context.Context, store *Store, tempID string) {
	rec, ok := store.Wallet(tempID)
	if !ok || rec.Status != StatusPendingImport {
		return
	}
	if store.WalletName() != activeWalletSeam() {
		log.Printf("msig: shared wallet %q verified; switch to wallet %q to finish", rec.Label, store.WalletName())
		return
	}
	if err := importScriptSeam(ctx, rec.ScriptHex, false, 0); err != nil {
		log.Printf("msig: import script for %s deferred: %v", tempID, err)
		return
	}
	tip, err := tipHeightSeam(ctx)
	if err != nil {
		log.Printf("msig: tip height: %v", err)
	}
	err = store.UpdateWallet(tempID, func(r *WalletRecord) error {
		if r.Status != StatusPendingImport {
			return fmt.Errorf("round %s state moved", tempID)
		}
		r.CreatedHeight = tip
		r.Status = StatusActive
		return nil
	})
	if err != nil {
		log.Printf("msig: %v", err)
		return
	}
	if err := sendFrame(store, rec.InitiatorUID,
		&Message{Type: TypeReady, TempID: tempID, WalletID: rec.Address}, ""); err != nil {
		log.Printf("msig: ready frame: %v", err)
	}
	log.Printf("msig: shared wallet %q active at %s", rec.Label, rec.Address)
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
		switch rec.Status {
		case StatusInviting:
			if rec.HD {
				maybeActivateInitiatorHD(ctx, store, rec.TempID)
			} else {
				maybeActivateInitiator(ctx, store, rec.TempID)
			}
		case StatusPendingImport:
			if rec.HD {
				completeCosignerImportHD(ctx, store, rec.TempID)
			} else {
				completeCosignerImport(ctx, store, rec.TempID)
			}
		case StatusActive:
			if rec.HD {
				if err := ensureWindowImported(ctx, store, rec.TempID); err != nil {
					log.Printf("msig: window catch-up for %q: %v", rec.Label, err)
				}
			}
		}
	}
}
