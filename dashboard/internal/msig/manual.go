// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Manual wallets ferry the same frames Bison Relay carries, by human
// hands: outbound frames wait in the outbox as export cards, and pasted
// frames enter through ImportFrame. The wire never carries identities,
// so the importer says who handed the blob over; the protocol's own
// verification does not trust that word for anything but labels.

// reencodeHorizon is how much lifetime an export may have left before it
// is silently re-framed with a fresh expiry (same mid, same payload).
const reencodeHorizon = ManualTTL / 2

// ManualFrame is one hand-carried export.
type ManualFrame struct {
	MID    string `json:"mid"`
	ToUID  string `json:"toUid"`
	ToNick string `json:"toNick"`
	Type   string `json:"type"`
	Body   string `json:"body"`
	Ts     int64  `json:"ts"`
}

// ManualOutbox lists the frames of a manual wallet still waiting to be
// handed over, freshest expiry guaranteed. Items whose delivery is
// PROVEN by protocol state (the peer answered, the payment advanced) are
// retired on the way out.
func ManualOutbox(ctx context.Context, id string) ([]ManualFrame, error) {
	network, err := networkSeam(ctx)
	if err != nil {
		return nil, err
	}
	store, rec := manager(network).Route(id, nil)
	if rec == nil {
		return nil, fmt.Errorf("unknown shared wallet %s", id)
	}
	if !rec.ManualTransport() {
		return nil, fmt.Errorf("shared wallet %q coordinates over Bison Relay", rec.Label)
	}
	out := make([]ManualFrame, 0, 4)
	for _, it := range store.PendingOutbox() {
		if !it.Manual || it.RecID != rec.TempID {
			continue
		}
		if manualFrameStale(rec, it) {
			if err := store.MarkOutboxSent(it.MID, it.ToUID); err != nil {
				msigLog.Warnf("retire manual frame: %v", err)
			}
			continue
		}
		body := it.Body
		if frame, err := Parse(body, time.Now().Add(-ManualTTL)); err == nil {
			if time.Until(frame.Exp) < reencodeHorizon {
				if fresh, rerr := Reencode(body, time.Now().Add(ManualTTL)); rerr == nil {
					if uerr := store.UpdateOutboxBody(it.MID, it.ToUID, fresh); uerr == nil {
						body = fresh
					}
				}
			}
		}
		nick := it.ToUID
		if p := rec.peerByUID(it.ToUID); p != nil {
			nick = p.Nick
		}
		out = append(out, ManualFrame{
			MID: it.MID, ToUID: it.ToUID, ToNick: nick, Type: it.Type, Body: body, Ts: it.Ts,
		})
	}
	return out, nil
}

// manualFrameStale reports whether protocol state PROVES the counterpart
// already acted on this frame, so the export card can retire itself.
// Anything without such proof stays until the user marks it handed over.
func manualFrameStale(rec *WalletRecord, it *OutboxItem) bool {
	peer := rec.peerByUID(it.ToUID)
	switch it.Type {
	case TypeInvite:
		if rec.Terminal() || rec.Status == StatusActive || rec.Status == StatusActivating {
			return true
		}
		return peer != nil && peer.State != PeerInvited
	case TypeRoster:
		if rec.Terminal() || rec.Status == StatusActive {
			return true
		}
		return peer != nil && peer.State == PeerReady
	case TypeAccept:
		// The roster's arrival proves the initiator received the accept.
		return rec.Terminal() || rec.Status != StatusInvited && rec.Status != StatusAccepted
	case TypeSignReq:
		p, ok := rec.Proposals[it.TxID]
		if !ok {
			return true
		}
		if p.Terminal() || p.Status == ProposalBroadcast {
			return true
		}
		for _, h := range p.Queue {
			if h.SentMID == it.MID {
				return h.State != HopSent
			}
		}
		return false
	case TypeSig:
		// The broadcast notice (or the chain) advancing the proposal
		// proves the proposer received the signature.
		p, ok := rec.Proposals[it.TxID]
		if !ok {
			return true
		}
		return p.Status != ProposalSigned
	case TypeBroadcast:
		p, ok := rec.Proposals[it.TxID]
		if !ok {
			return true
		}
		return p.Status == ProposalConfirmed
	case TypeReady:
		// The attestation set's arrival proves the initiator received
		// this cosigner's confirmation.
		return rec.Terminal() || len(rec.Attests) > 0 ||
			rec.Status == StatusPendingImport || rec.Status == StatusActive
	}
	// attest_set, decline, sig_decline, invite_cancel: nothing ever comes
	// back to prove delivery; the user retires them.
	return false
}

// MarkManualDelivered records that the user handed one frame over.
func MarkManualDelivered(ctx context.Context, id, mid, toUID string) error {
	network, err := networkSeam(ctx)
	if err != nil {
		return err
	}
	store, rec := manager(network).Route(id, nil)
	if rec == nil {
		return fmt.Errorf("unknown shared wallet %s", id)
	}
	if !rec.ManualTransport() {
		return fmt.Errorf("shared wallet %q coordinates over Bison Relay", rec.Label)
	}
	return store.MarkOutboxSent(mid, toUID)
}

// ImportResult tells the importer what a pasted frame was and what
// happened to it.
type ImportResult struct {
	Outcome  string `json:"outcome"` // processed | duplicate | ignored
	Type     string `json:"type"`
	WalletID string `json:"walletId,omitempty"`
	Label    string `json:"label,omitempty"`
}

// ImportFrame ingests one hand-carried frame. Invitations need only the
// initiator's name; every other frame targets a known manual wallet and
// the importer says which cosigner handed it over. Validation happens
// before the journal can burn a mid, so mistakes come back as errors
// instead of silence.
func ImportFrame(ctx context.Context, body, id, fromUID, fromLabel string) (*ImportResult, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("paste or open a coordination message first")
	}
	now := time.Now()
	frame, err := Parse(body, now)
	switch {
	case errors.Is(err, ErrNotEnvelope):
		return nil, fmt.Errorf("this is not a shared-wallet coordination message")
	case errors.Is(err, ErrExpired):
		return nil, fmt.Errorf("this message has expired; ask for a fresh export")
	case errors.Is(err, ErrUnsupportedVersion):
		return nil, fmt.Errorf("this message needs a newer dcrpulse")
	case err != nil:
		return nil, fmt.Errorf("malformed coordination message: %v", err)
	}
	msg, err := DecodeMessage(frame.Payload)
	if errors.Is(err, ErrUnknownType) {
		return nil, fmt.Errorf("this message type needs a newer dcrpulse")
	}
	if err != nil {
		return nil, fmt.Errorf("invalid coordination message: %v", err)
	}
	network, err := networkSeam(ctx)
	if err != nil {
		return nil, err
	}

	if msg.Type == TypeInvite {
		if msg.Network != network {
			return nil, fmt.Errorf("this invitation is for %s; this wallet runs on %s", msg.Network, network)
		}
		if msg.Ver != ProtoHD {
			return nil, fmt.Errorf("this invitation predates HD shared wallets")
		}
		fromLabel = strings.TrimSpace(fromLabel)
		if fromLabel == "" || len(fromLabel) > MaxLabelLen {
			return nil, fmt.Errorf("name the person who sent this invitation (1 to %d characters)", MaxLabelLen)
		}
		store, err := manager(network).StoreFor(activeWalletSeam())
		if err != nil {
			return nil, err
		}
		if _, exists := store.Wallet(msg.TempID); exists {
			return &ImportResult{Outcome: "duplicate", Type: msg.Type, WalletID: msg.TempID, Label: msg.Label}, nil
		}
		uid, err := NewID()
		if err != nil {
			return nil, err
		}
		handleImportedFrame(uid, fromLabel, body, now)
		if _, exists := store.Wallet(msg.TempID); !exists {
			return nil, fmt.Errorf("the invitation could not be recorded; see the log")
		}
		return &ImportResult{Outcome: "processed", Type: msg.Type, WalletID: msg.TempID, Label: msg.Label}, nil
	}

	if id == "" || fromUID == "" {
		return nil, fmt.Errorf("pick the shared wallet and the cosigner this message came from")
	}
	store, rec := manager(network).Route(id, nil)
	if rec == nil {
		return nil, fmt.Errorf("unknown shared wallet %s", id)
	}
	if !rec.ManualTransport() {
		return nil, fmt.Errorf("shared wallet %q coordinates over Bison Relay", rec.Label)
	}
	// Frames route globally by their own ids and the router prefers the
	// active wallet, so importing against an inactive wallet's record
	// could silently land elsewhere. Refuse instead.
	if store.WalletName() != activeWalletSeam() {
		return nil, fmt.Errorf("this shared wallet belongs to wallet %q; switch to it first", store.WalletName())
	}
	peer := rec.peerByUID(fromUID)
	if peer == nil {
		return nil, fmt.Errorf("that cosigner is not part of %q", rec.Label)
	}
	if msg.TempID != "" && msg.TempID != rec.TempID {
		return nil, fmt.Errorf("this message belongs to a different shared wallet")
	}
	if msg.WalletID != "" && msg.WalletID != rec.Address && msg.WalletID != rec.TempID {
		return nil, fmt.Errorf("this message belongs to a different shared wallet")
	}
	if store.SeenMid(frame.MID) {
		return &ImportResult{Outcome: "duplicate", Type: msg.Type, WalletID: rec.TempID, Label: rec.Label}, nil
	}
	before, _ := store.Wallet(rec.TempID)
	handleImportedFrame(fromUID, peer.Nick, body, now)
	after, _ := store.Wallet(rec.TempID)
	outcome := "ignored"
	if before != nil && after != nil && after.UpdatedAt >= before.UpdatedAt && store.SeenMid(frame.MID) {
		outcome = "processed"
	}
	return &ImportResult{Outcome: outcome, Type: msg.Type, WalletID: rec.TempID, Label: rec.Label}, nil
}
