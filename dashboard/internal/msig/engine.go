// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
)

// The engine consumes typed "msig" events that brclientd publishes for
// coordination frames and replays missed frames from brclientd's PM log.
// Live events are a hint only: the event bus drops on slow consumers, so
// the /msig/history catch-up is the reliability backbone. Log lines carry
// type, mid and peer, never payloads.

const (
	sweepInterval = 15 * time.Minute
	// roundExpiry fails handshakes that outlived the BR delivery horizon
	// plus a day of grace for an initiator restart. Manual rounds get the
	// hand-carried frame lifetime plus the same grace: nobody re-routes
	// behind the humans, but zombie rounds still die eventually.
	roundExpiry       = 8 * 24 * time.Hour
	manualRoundExpiry = ManualTTL + 24*time.Hour
)

// StartEngine subscribes to the dashboard's Bison Relay event bus and
// starts the maintenance sweep. Frames for wallets this node has not yet
// activated are intentionally left unjournaled so a later catch-up can
// process them.
func StartEngine(ctx context.Context) {
	// Announce every persisted state change to the browser bus so open
	// pages re-render on the engine's commit, not just on a frame's
	// arrival. The payload carries the wallet id only.
	notifyChange = func(walletID string) {
		payload, err := json.Marshal(struct {
			WalletID string `json:"walletId"`
		}{walletID})
		if err != nil {
			return
		}
		services.PublishBisonrelayEvent("msig-state", payload)
	}
	ch, cancel := services.Bisonrelay().Subscribe(64)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				if evt.Type != "msig" {
					continue
				}
				handleFrameEvent(evt.Payload, time.Now())
			}
		}
	}()
	go sweepLoop(ctx)
	// One-shot full-contact replay so invites that arrived while the
	// dashboard was down surface without a manual refresh. Delayed so
	// brclientd has a chance to come up first.
	go func() {
		select {
		case <-ctx.Done():
		case <-time.After(20 * time.Second):
			cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			CatchUpAllContacts(cctx)
		}
	}()
	log.Printf("msig engine: listening for coordination frames")
}

func handleFrameEvent(payload json.RawMessage, now time.Time) {
	var p struct {
		From     string `json:"from"`
		FromNick string `json:"fromNick"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		log.Printf("msig: malformed event payload: %v", err)
		return
	}
	handleInbound(p.From, p.FromNick, p.Message, now)
}

// handleInbound is the entry point for live and replayed Bison Relay
// frames. Manually imported frames come in through handleImportedFrame,
// which differs only in stamping new invite records as manual.
func handleInbound(fromUID, fromNick, body string, now time.Time) {
	handleFrame(fromUID, fromNick, body, now, false)
}

// handleImportedFrame ingests a hand-carried frame. The wire is
// transport-free by design, so the import path itself marks the records
// it creates.
func handleImportedFrame(fromUID, fromNick, body string, now time.Time) {
	handleFrame(fromUID, fromNick, body, now, true)
}

func handleFrame(fromUID, fromNick, body string, now time.Time, imported bool) {
	peer := fromNick
	if len(fromUID) >= 12 {
		peer = fromNick + " (" + fromUID[:12] + ")"
	}
	frame, err := Parse(body, now)
	switch {
	case errors.Is(err, ErrNotEnvelope):
		return
	case errors.Is(err, ErrExpired):
		log.Printf("msig: dropping expired frame from %s", peer)
		return
	case errors.Is(err, ErrUnsupportedVersion):
		log.Printf("msig: ignoring frame of unsupported version from %s", peer)
		return
	case err != nil:
		log.Printf("msig: dropping malformed frame from %s: %v", peer, err)
		return
	}
	msg, err := DecodeMessage(frame.Payload)
	if errors.Is(err, ErrUnknownType) {
		journalUnknownType(msg, frame, peer, now)
		return
	}
	if err != nil {
		log.Printf("msig: dropping invalid frame %s from %s: %v", frame.MID, peer, err)
		return
	}
	dispatchInbound(msg, frame, fromUID, fromNick, now, imported)
}

// journalUnknownType records the mid of a structurally valid message this
// build cannot interpret, when it can be routed to a known record, so a
// resend never reprocesses it after an upgrade either.
func journalUnknownType(msg *Message, frame *Frame, peer string, now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	network, err := networkSeam(ctx)
	if err != nil {
		return
	}
	id := msg.TempID
	if id == "" {
		id = msg.WalletID
	}
	if id != "" {
		if store, rec := manager(network).Route(id, nil); rec != nil {
			if _, err := store.MarkProcessed(frame.MID, now); err != nil {
				log.Printf("msig: journal: %v", err)
			}
		}
	}
	log.Printf("msig: ignoring frame %s of unknown type %q from %s", frame.MID, msg.Type, peer)
}

func sweepLoop(ctx context.Context) {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			runSweep(ctx)
			timer.Reset(sweepInterval)
		}
	}
}

// runSweep retries unsent frames, expires stale rounds, resumes
// wallet-gated steps for the active wallet and replays missed frames from
// known peers. Also run by the manual refresh endpoint.
func runSweep(ctx context.Context) {
	sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	network, err := networkSeam(sctx)
	if err != nil {
		return
	}
	m := manager(network)
	m.OpenExisting()
	for _, s := range m.Stores() {
		resendOutbox(s)
		sweepManualOutbox(s)
		expireStaleRounds(s)
		sweepProposals(sctx, s)
	}
	ResumePending(sctx)
	catchUpKnownPeers(sctx, m)
}

// sweepManualOutbox retires hand-over cards whose delivery the protocol
// state already proves, so they never linger after the counterpart acted
// through some other copy of the frame.
func sweepManualOutbox(s *Store) {
	for _, it := range s.PendingOutbox() {
		if !it.Manual {
			continue
		}
		rec, ok := s.Wallet(it.RecID)
		if !ok {
			continue
		}
		if manualFrameStale(rec, it) {
			if err := s.MarkOutboxSent(it.MID, it.ToUID); err != nil {
				log.Printf("msig: retire manual frame: %v", err)
			}
		}
	}
}

// RunSweepNow is the manual refresh entry used by the API. On top of
// the periodic sweep's work it re-announces settled rosters, so an
// initiator on a tuple-aware build can fill the peer lists of cosigners
// that settled under an older one. Explicit refresh only; nothing
// re-announces in the background.
func RunSweepNow(ctx context.Context) {
	runSweep(ctx)
	reannounceRosters(ctx)
}

func reannounceRosters(ctx context.Context) {
	network, err := networkSeam(ctx)
	if err != nil {
		return
	}
	s, err := manager(network).StoreFor(activeWalletSeam())
	if err != nil {
		return
	}
	for _, rec := range s.Wallets() {
		if !rec.HD || rec.Role != RoleInitiator || rec.Status != StatusActive ||
			rec.Address == "" || len(rec.Xpubs) == 0 {
			continue
		}
		msg := rosterMessage(rec)
		for _, p := range rec.Peers {
			if err := sendFrame(s, p.UID, msg, ""); err != nil {
				log.Printf("msig: roster to %s: %v", p.Nick, err)
			}
		}
	}
}

func resendOutbox(s *Store) {
	for _, it := range s.PendingOutbox() {
		// Manual items wait for the user; sending them would hand a
		// pseudo peer id to brclientd.
		if it.Manual {
			continue
		}
		deliverOutbox(s, it.MID, it.ToUID, it.Body)
	}
}

func expireStaleRounds(s *Store) {
	now := time.Now()
	for _, rec := range s.Wallets() {
		cutoff := now.Add(-roundExpiry).Unix()
		if rec.ManualTransport() {
			cutoff = now.Add(-manualRoundExpiry).Unix()
		}
		switch rec.Status {
		case StatusInviting, StatusInvited, StatusAccepted, StatusActivating:
			if rec.CreatedAt > 0 && rec.CreatedAt < cutoff {
				failRound(s, rec.TempID, "invite round expired")
			}
		}
	}
}

// catchUpKnownPeers replays frames from every peer of a non-terminal
// record of the ACTIVE wallet. Only the active wallet's BR identity is
// online, so other stores have nothing new to read.
func catchUpKnownPeers(ctx context.Context, m *Manager) {
	s, err := m.StoreFor(activeWalletSeam())
	if err != nil {
		return
	}
	peers := make(map[string]int64)
	for _, rec := range s.Wallets() {
		if rec.Terminal() || rec.ManualTransport() {
			// Manual peers are pseudo ids; brclientd has no history
			// for them.
			continue
		}
		for _, p := range rec.Peers {
			last, ok := peers[p.UID]
			if !ok || p.LastSeenTs < last {
				peers[p.UID] = p.LastSeenTs
			}
		}
	}
	for uid, last := range peers {
		since := last - 7200
		if since < 0 {
			since = 0
		}
		replayPeer(ctx, uid, since)
	}
}

// CatchUpAllContacts scans every KX'd contact's frame history. Run at
// engine start and on manual refresh so invites that arrived while the
// dashboard was down are still discovered; the mid journals absorb
// everything already processed.
func CatchUpAllContacts(ctx context.Context) {
	raw, err := rpc.BrclientdContacts(ctx)
	if err != nil {
		return
	}
	var resp struct {
		Entries []struct {
			ID struct {
				Identity string `json:"identity"`
				Nick     string `json:"nick"`
			} `json:"id"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		log.Printf("msig: parse contacts: %v", err)
		return
	}
	since := time.Now().Add(-roundExpiry).Unix()
	for _, e := range resp.Entries {
		if e.ID.Identity == "" {
			continue
		}
		replayPeer(ctx, e.ID.Identity, since)
	}
}

func replayPeer(ctx context.Context, uid string, since int64) {
	raw, err := msigHistorySeam(ctx, uid, 500, since)
	if err != nil {
		return
	}
	var resp struct {
		LocalNick string `json:"local_nick"`
		Entries   []struct {
			Message   string `json:"message"`
			From      string `json:"from"`
			Timestamp int64  `json:"timestamp"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return
	}
	now := time.Now()
	for _, e := range resp.Entries {
		if resp.LocalNick != "" && e.From == resp.LocalNick {
			continue
		}
		handleInbound(uid, e.From, e.Message, now)
	}
}
