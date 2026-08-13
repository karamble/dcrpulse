package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"

	"dcrpulse/internal/rpc"
)

// Errors a game can be told about. They are deliberately unrevealing: a game
// learns that the tunnel refused, not what else the host is carrying.
var (
	ErrGamingGameNotInstalled = errors.New("game is not installed")
	ErrGamingNotAFrame        = errors.New("not a gaming frame")
	ErrGamingWrongGame        = errors.New("frame belongs to another game")
)

// The gaming bridge is a tunnel between registered games and Bison Relay.
//
// Games are untrusted, and they are not run here: a person runs one wherever
// they like and it connects in. They play for real money against strangers, so
// they are never handed the credentials for any of it. The bridge holds
// brclientd's client certificate and does the talking, which is the entire
// reason it exists.
//
// It is a tunnel and not a censor. Frames pass through whole: the bridge picks
// out which installed game a frame is for and hands it over, forming no opinion
// about the contents. Players sign their own actions and check each other's, so
// a host that inspected payloads would insert a party nobody agreed to trust.
// Policy belongs where money moves - account scope, caps and grants apply when
// a game asks to *spend*, not when it asks to speak.
//
// Frames reach the bridge over clientrpc rather than the /notifications stream
// the chat UI uses, because brclientd deliberately drops them from that stream:
// they are protocol, not conversation, and must not badge the UI or surface as
// messages. The clientrpc stream carries them unfiltered and resumes from a
// sequence id, so a table survives the bridge restarting - which matters, since
// a player who misses frames looks exactly like a player who walked away.

// GamingFrameEvent is one inbound frame, addressed to a game.
type GamingFrameEvent struct {
	// Game is the routing key from the envelope.
	Game string `json:"game"`
	// GCID identifies the group chat, which for a game is the table.
	GCID string `json:"gcid"`
	// From is the sender's Bison Relay uid, hex encoded. It is the
	// authenticated identity: it comes from the ratcheted message, not from
	// anything inside the payload, so no payload field can forge it.
	From string `json:"from"`
	// Frame is the whole envelope, untouched.
	Frame string `json:"frame"`
}

type gamingSubscriber struct {
	game string
	ch   chan GamingFrameEvent
}

// GamingBus fans inbound frames out to the games that are listening.
//
// Subscribers are per game, so one game never sees another's traffic. That is
// containment rather than privacy - frames travel over a group chat every
// member can read - but a game has no business seeing traffic addressed to a
// different one.
type GamingBus struct {
	mu   sync.RWMutex
	subs map[*gamingSubscriber]struct{}
}

var (
	gamingBus     *GamingBus
	gamingBusOnce sync.Once
)

// Gaming returns the singleton frame bus.
func Gaming() *GamingBus {
	gamingBusOnce.Do(func() {
		gamingBus = &GamingBus{subs: make(map[*gamingSubscriber]struct{})}
	})
	return gamingBus
}

// Subscribe registers a listener for one game's frames and returns a buffered
// channel it must drain, plus a cancel func.
func (b *GamingBus) Subscribe(game string, buf int) (<-chan GamingFrameEvent, func()) {
	if buf <= 0 {
		buf = 64
	}
	s := &gamingSubscriber{game: game, ch: make(chan GamingFrameEvent, buf)}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s.ch, func() {
		b.mu.Lock()
		delete(b.subs, s)
		b.mu.Unlock()
		close(s.ch)
	}
}

// subscribers counts the listeners for a game, so a frame that reaches nobody
// can be told apart from one that was never received.
func (b *GamingBus) subscribers(game string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	n := 0
	for s := range b.subs {
		if s.game == game {
			n++
		}
	}
	return n
}

// broadcast delivers a frame to every listener for its game.
//
// A listener that is not draining is skipped rather than waited for. Blocking
// here would stall the single stream that every table shares, so one wedged
// game would take down the others; a game that cannot keep up loses frames and
// has to resynchronise, which its protocol needs to handle regardless because
// Bison Relay does not guarantee delivery either.
func (b *GamingBus) broadcast(ev GamingFrameEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for s := range b.subs {
		if s.game != ev.Game {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			log.Printf("gaming bridge: %s is not draining its frames; dropping one", s.game)
		}
	}
}

// gamingFrameEvent is the notification type brclientd forwards gaming envelope
// frames on. It is deliberately not "pm" or "gc-message": a frame that took the
// chat path would badge a conversation the user never had.
const gamingFrameEvent = "gaming-frame"

// deliverFrame routes one inbound frame to the game it belongs to.
//
// Frames reach us on brclientd's /notifications feed, the same in-process path
// every other kind of Bison Relay message takes here, fed by an OnGCMNtfn (or
// OnPMNtfn, for invites) registration on the client's notification manager -
// which is how the MCP bridge receives as well. The clientrpc ChatService
// streams are not used: they replay their whole backlog on every (re)subscribe,
// which is why the chat path avoids them too.
func (b *GamingBus) deliverFrame(payload json.RawMessage) {
	var evt struct {
		GCID    string `json:"gcid"`
		From    string `json:"from"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(payload, &evt); err != nil {
		log.Printf("gaming bridge: undecodable frame event: %v", err)
		return
	}
	frame, ok := parseGamingFrame(evt.Message)
	if !ok {
		// brclientd forwards envelopes and nothing else, so reaching
		// here means the two sides disagree about the envelope format.
		log.Printf("gaming bridge: forwarded event from %s is not a frame (%d bytes)",
			evt.From, len(evt.Message))
		return
	}
	if !gamingGameInstalled(frame.Game) {
		// A game this installation does not have. Dropping it is what
		// makes the namespace work: a new game can appear without every
		// existing host being taught about it.
		return
	}
	log.Printf("gaming bridge: delivering %q frame to %d subscriber(s)", frame.Game, b.subscribers(frame.Game))
	b.broadcast(GamingFrameEvent{
		Game:  frame.Game,
		GCID:  evt.GCID,
		From:  evt.From,
		Frame: frame.Text,
	})
}

// gamingGameInstalled reports whether the user added a game. The installed list
// is the routing table: a frame for anything else is not this host's business.
func gamingGameInstalled(game string) bool {
	for _, id := range ReadGamingSettings().InstalledGames {
		if id == game {
			return true
		}
	}
	return false
}

// SendGamingFrame delivers a frame to a table's group chat on a game's behalf.
//
// The frame is sent as the game built it. The bridge checks that the game is
// installed and that the text really is one of its frames - so a game cannot
// use the tunnel to send chat, or to send another game's traffic - and carries
// it no further than that.
func SendGamingFrame(ctx context.Context, game, gcid, frame string) error {
	if !gamingGameInstalled(game) {
		return ErrGamingGameNotInstalled
	}
	parsed, ok := parseGamingFrame(frame)
	if !ok {
		return ErrGamingNotAFrame
	}
	if parsed.Game != game {
		return ErrGamingWrongGame
	}
	return rpc.BrclientdGCMessage(ctx, gcid, frame, 0)
}
