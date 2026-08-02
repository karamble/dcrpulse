// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"dcrpulse/internal/services"
)

// The engine consumes typed "msig" events that brclientd publishes for
// coordination frames. This is the transport skeleton: strict parse,
// in-memory dedup by mid, log. Protocol dispatch and the durable
// per-wallet journal arrive with the registry store. Log lines carry only
// type, mid and peer, never payloads.

// seenTTL keeps mids past the BR server's 7 day delivery horizon so a
// replayed frame can never be mistaken for a new one.
const seenTTL = 30 * 24 * time.Hour

var (
	seenMu   sync.Mutex
	seenMIDs = make(map[string]time.Time)
)

// markSeen records a mid and reports whether it was new.
func markSeen(mid string, now time.Time) bool {
	seenMu.Lock()
	defer seenMu.Unlock()
	if len(seenMIDs) > 4096 {
		for k, t := range seenMIDs {
			if now.Sub(t) > seenTTL {
				delete(seenMIDs, k)
			}
		}
	}
	if _, ok := seenMIDs[mid]; ok {
		return false
	}
	seenMIDs[mid] = now
	return true
}

// StartEngine subscribes to the dashboard's Bison Relay event bus and
// consumes coordination frames. Live events are a hint only; the reliable
// backbone is replay through brclientd's /msig/history, which later phases
// run at startup, on wallet switch and on a periodic sweep.
func StartEngine(ctx context.Context) {
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
	peer := p.FromNick
	if len(p.From) >= 12 {
		peer = p.FromNick + " (" + p.From[:12] + ")"
	}
	frame, err := Parse(p.Message, now)
	switch {
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
	if !markSeen(frame.MID, now) {
		log.Printf("msig: duplicate frame %s from %s", frame.MID, peer)
		return
	}
	msg, err := DecodeMessage(frame.Payload)
	if errors.Is(err, ErrUnknownType) {
		log.Printf("msig: ignoring frame %s of unknown type %q from %s", frame.MID, msg.Type, peer)
		return
	}
	if err != nil {
		log.Printf("msig: dropping invalid frame %s from %s: %v", frame.MID, peer, err)
		return
	}
	log.Printf("msig: received %s frame %s from %s", msg.Type, frame.MID, peer)
}
