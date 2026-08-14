// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"dcrpulse/internal/gamingpb"
)

// Frame is one Bison Relay message addressed to a game.
//
// Declared here rather than taken from the services package so this one keeps
// depending on nothing: the bridge is handed frames, it does not go and get
// them.
type Frame struct {
	GCID  string
	From  string
	Frame string
}

// pushBuffer is how many operator requests may queue for a game before one is
// dropped. Small on purpose: these are things a person clicked, and a game that
// has not answered a dozen of them is not going to answer the thirteenth.
const pushBuffer = 16

// frameBuffer is how many inbound frames may queue for one stream.
const frameBuffer = 64

// registry is the live streams, and the sequencing the contract requires.
//
// The bridge owns all of it. Nothing upstream supplies an ordinal: a frame
// arrives on a notification feed with no sequence, no acknowledgement and no
// replay log, and the fan-out beneath this buffers nothing. So a game cannot be
// told where it is in a stream unless this counts, and cannot be told what it
// missed unless this remembers.
type registry struct {
	// epoch changes every time the process starts, because everything below
	// is in memory: a game holding a sequence number from before a restart is
	// holding a number this bridge can no longer place.
	epoch string

	mu      sync.Mutex
	streams map[string]map[*liveStream]struct{}
	seq     map[string]uint64
	missed  map[string]map[string]struct{}

	// states is the last thing each game reported, kept so the console can
	// render a game that is not connected right now as it last was.
	states map[string]*gamingpb.GameState
}

// liveStream is one game's open subscription.
type liveStream struct {
	events chan *gamingpb.BridgeEvent

	// done ends the stream from the outside, which is what revoking a
	// credential has to be able to do.
	done      chan struct{}
	closeOnce sync.Once
}

func (l *liveStream) close() {
	l.closeOnce.Do(func() { close(l.done) })
}

func newRegistry() *registry {
	var seed [16]byte
	if _, err := rand.Read(seed[:]); err != nil {
		// An epoch nobody can generate is not a reason to refuse to run.
		// A fixed one only costs a resync that would have happened anyway.
		gameLog.Warnf("could not generate a stream epoch, so every reconnect will resync: %v", err)
	}
	return &registry{
		epoch:   hex.EncodeToString(seed[:]),
		streams: make(map[string]map[*liveStream]struct{}),
		seq:     make(map[string]uint64),
		missed:  make(map[string]map[string]struct{}),
		states:  make(map[string]*gamingpb.GameState),
	}
}

func (r *registry) setState(game string, state *gamingpb.GameState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states[game] = state
}

func (r *registry) state(game string) *gamingpb.GameState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.states[game]
}

// add registers a new stream for a game.
func (r *registry) add(game string) *liveStream {
	l := &liveStream{
		events: make(chan *gamingpb.BridgeEvent, pushBuffer),
		done:   make(chan struct{}),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.streams[game] == nil {
		r.streams[game] = make(map[*liveStream]struct{})
	}
	r.streams[game][l] = struct{}{}
	return l
}

func (r *registry) remove(game string, l *liveStream) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if set := r.streams[game]; set != nil {
		delete(set, l)
		if len(set) == 0 {
			delete(r.streams, game)
		}
	}
}

func (r *registry) count(game string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.streams[game])
}

// closeGame ends every stream a game is holding.
//
// This is what makes revoking take effect now rather than at the game's
// convenience. A credential withdrawn while a stream is open would otherwise go
// on receiving every table's traffic for as long as the game chose to stay
// connected, which is exactly the situation an operator revokes in.
// liveGames names the games with at least one open stream.
func (r *registry) liveGames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.streams))
	for game, set := range r.streams {
		if len(set) > 0 {
			out = append(out, game)
		}
	}
	return out
}

func (r *registry) closeGame(game string) {
	r.mu.Lock()
	set := r.streams[game]
	live := make([]*liveStream, 0, len(set))
	for l := range set {
		live = append(live, l)
	}
	r.mu.Unlock()

	for _, l := range live {
		l.close()
	}
	if len(live) > 0 {
		gameLog.Infof("closed %d open stream(s) for %q", len(live), game)
	}
}

// streamStart is the first event on every stream, and the only place a gap is
// declared.
//
// A game resyncs when and only when gap is true, so this has to be right in
// both directions: claiming a gap that did not happen costs a resync of every
// table, and missing one leaves the game quietly out of date.
func (r *registry) streamStart(game string, req *gamingpb.SubscribeRequest, missedGCIDs []string, missedAll bool) *gamingpb.StreamStart {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Anything this bridge dropped while the game was away, plus anything the
	// fan-out beneath dropped because nothing was listening.
	for _, gcid := range missedGCIDs {
		r.noteMissedLocked(game, gcid)
	}

	start := &gamingpb.StreamStart{Epoch: r.epoch, FromSeq: r.seq[game]}

	switch {
	case missedAll:
		// Something upstream of this bridge lost events, so nothing that
		// arrived can be trusted to be everything, and there is no gcid to
		// name: the event that would have said which table never got here.
		start.Gap = true
		start.GapScope = gamingpb.GapScope_GAP_ALL
	case req.GetEpoch() != r.epoch:
		// A different epoch means a different process. Nothing from before
		// can be placed, so everything is suspect.
		start.Gap = true
		start.GapScope = gamingpb.GapScope_GAP_ALL
	case req.GetLastSeq() != r.seq[game]:
		// The game is behind what was sent, and this bridge keeps no
		// backlog to work out which tables that covered.
		start.Gap = true
		start.GapScope = gamingpb.GapScope_GAP_ALL
	case len(r.missed[game]) > 0:
		// Known losses, and known which tables they were on - so the game
		// resyncs those and leaves the rest alone.
		start.Gap = true
		start.GapScope = gamingpb.GapScope_GAP_SCOPED
		for gcid := range r.missed[game] {
			start.GapGcids = append(start.GapGcids, gcid)
		}
	}

	// Reported once. Holding them would declare the same gap on every
	// reconnect, and a game that resynced every time would never settle.
	delete(r.missed, game)
	return start
}

func (r *registry) noteMissedLocked(game, gcid string) {
	if gcid == "" {
		return
	}
	if r.missed[game] == nil {
		r.missed[game] = make(map[string]struct{})
	}
	r.missed[game][gcid] = struct{}{}
}

func (r *registry) noteMissed(game, gcid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.noteMissedLocked(game, gcid)
}

// frameEvent stamps a frame with the game's next sequence number.
func (r *registry) frameEvent(game string, f Frame) *gamingpb.BridgeEvent {
	r.mu.Lock()
	r.seq[game]++
	seq := r.seq[game]
	r.mu.Unlock()

	return &gamingpb.BridgeEvent{Event: &gamingpb.BridgeEvent_Frame{Frame: &gamingpb.Frame{
		Seq:   seq,
		Gcid:  f.GCID,
		From:  f.From,
		Frame: f.Frame,
	}}}
}

// push hands an event to every stream a game is holding, and reports whether
// any of them took it.
//
// A stream that is not draining is skipped rather than waited for: blocking
// here would stall whoever is delivering, and one wedged game would take the
// others down with it.
func (r *registry) push(game string, ev *gamingpb.BridgeEvent) bool {
	r.mu.Lock()
	set := r.streams[game]
	live := make([]*liveStream, 0, len(set))
	for l := range set {
		live = append(live, l)
	}
	r.mu.Unlock()

	delivered := false
	for _, l := range live {
		select {
		case l.events <- ev:
			delivered = true
		default:
			gameLog.Warnf("%s is not draining its stream; dropping a request", game)
		}
	}
	return delivered
}
