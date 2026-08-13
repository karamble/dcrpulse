// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"

	"dcrpulse/internal/gamingpb"
)

// ErrGameNotConnected is a request nothing was holding a stream to take.
var ErrGameNotConnected = errors.New("game is not connected")

// pending is the requests waiting on a game's answer, keyed by request id.
type pending struct {
	mu      sync.Mutex
	waiting map[string]*waiter
}

type waiter struct {
	game  string
	reply chan *gamingpb.RespondRequest
}

func newPending() *pending {
	return &pending{waiting: make(map[string]*waiter)}
}

func (p *pending) add(id, game string) chan *gamingpb.RespondRequest {
	// Buffered so a reply that lands after the caller gave up does not block
	// the responding game.
	ch := make(chan *gamingpb.RespondRequest, 1)
	p.mu.Lock()
	p.waiting[id] = &waiter{game: game, reply: ch}
	p.mu.Unlock()
	return ch
}

func (p *pending) drop(id string) {
	p.mu.Lock()
	delete(p.waiting, id)
	p.mu.Unlock()
}

// fulfil hands a reply to whoever is waiting for it, and reports whether the
// answer was owed. The game is checked because a request id is the only thing
// naming the waiter, and one game must not answer another's.
func (p *pending) fulfil(id, game string, reply *gamingpb.RespondRequest) bool {
	p.mu.Lock()
	w, ok := p.waiting[id]
	if ok && w.game == game {
		delete(p.waiting, id)
	}
	p.mu.Unlock()
	if !ok || w.game != game {
		return false
	}
	w.reply <- reply
	return true
}

// requestID names one request for the length of its answer.
func requestID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Request asks a game to do something and waits for it to say what happened.
//
// The console's half of the stream: Deliver pushes and forgets, this one is for
// the things a person is standing in front of waiting on. A game that is not
// connected fails here rather than timing out, because the answer is already
// known.
func (s *Server) Request(ctx context.Context, game string, req *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error) {
	id, err := requestID()
	if err != nil {
		return nil, err
	}
	req.RequestId = id
	if dl, ok := ctx.Deadline(); ok {
		req.DeadlineUnix = dl.Unix()
	}

	ch := s.pend.add(id, game)
	defer s.pend.drop(id)

	ev := &gamingpb.BridgeEvent{Event: &gamingpb.BridgeEvent_Request{Request: req}}
	if !s.reg.push(game, ev) {
		return nil, ErrGameNotConnected
	}

	select {
	case reply := <-ch:
		return reply, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
