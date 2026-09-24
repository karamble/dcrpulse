// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"sync"

	"dcrpulse/internal/gamingpb"
)

// Frame is one Bison Relay message addressed to a game.
//
// Declared here rather than taken from the services package so this one keeps
// depending on nothing: the bridge is handed frames, it does not go and get
// them.
type Frame struct {
	Seq   uint64
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
	// epoch identifies the durable inbox contract. It remains stable across
	// process restarts because frame sequence numbers are persisted by the host.
	epoch string

	mu      sync.Mutex
	streams map[string]map[*liveStream]struct{}

	// states is the last thing each game reported, kept so the console can
	// render a game that is not connected right now as it last was.
	states map[string]*gamingpb.GameState

	// locks is the refund and bond timelocks each game advertised on Hello,
	// kept so the console can disclose them before a person pays.
	locks map[string]lockTerms
}

// lockTerms is what a game advertised on Hello about the timelocks its money
// must sit behind: the least the refund branch may be, and how long a table
// bond is held.
type lockTerms struct {
	minRefund uint32
	bondLock  uint32
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
	return &registry{
		epoch:   "dcrpulse-gaming-inbox-v2",
		streams: make(map[string]map[*liveStream]struct{}),
		states:  make(map[string]*gamingpb.GameState),
		locks:   make(map[string]lockTerms),
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

func (r *registry) setHello(game string, terms lockTerms) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.locks[game] = terms
}

func (r *registry) hello(game string) lockTerms {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.locks[game]
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

// closeGame ends all subscriptions during server shutdown. Credential
// replacement and revocation instead invalidate their specific admissions.
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
func (r *registry) streamStart(req *gamingpb.SubscribeRequest) *gamingpb.StreamStart {
	r.mu.Lock()
	defer r.mu.Unlock()
	after := req.GetLastSeq()
	if req.GetEpoch() != r.epoch {
		after = 0
	}
	return &gamingpb.StreamStart{Epoch: r.epoch, FromSeq: after}
}

// frameEvent carries the durable sequence assigned before fan-out.
func (r *registry) frameEvent(game string, f Frame) *gamingpb.BridgeEvent {
	return &gamingpb.BridgeEvent{Event: &gamingpb.BridgeEvent_Frame{Frame: &gamingpb.Frame{
		Seq:   f.Seq,
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
