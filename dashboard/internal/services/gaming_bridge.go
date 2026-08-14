package services

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
)

// Errors a game can be told about. They are deliberately unrevealing: a game
// learns that the tunnel refused, not what else the host is carrying.
var (
	ErrGamingGameNotRegistered = errors.New("game is not registered")
	ErrGamingNotAFrame         = errors.New("not a gaming frame")
	ErrGamingWrongGame         = errors.New("frame belongs to another game")
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

	// missedMu guards missed, which records the tables whose frames reached
	// nobody: either the game was not connected, or it was not draining.
	//
	// Remembered because a loss the game is never told about is the worst
	// kind. Nothing here buffers frames, so the only repair is for the game
	// to resynchronise - and it can only do that for the right tables if the
	// bridge kept the names.
	missedMu sync.Mutex
	missed   map[string]map[string]struct{}

	// missedAll records games that missed frames whose tables are unknown,
	// because the loss happened upstream of this process and the event that
	// would have named a table never arrived.
	missedAll map[string]struct{}

	// resync closes every live game stream so the games resubscribe and are
	// told what they missed. Injected rather than called directly because the
	// bridge depends on this package, not the other way round.
	resync func(reason string)
}

// SetGamingResync wires the bridge's stream-closing hook. Called once at
// startup, before any game can connect.
func (b *GamingBus) SetGamingResync(fn func(reason string)) {
	b.resync = fn
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
	delivered, dropped := 0, false
	for s := range b.subs {
		if s.game != ev.Game {
			continue
		}
		select {
		case s.ch <- ev:
			delivered++
		default:
			dropped = true
			gameLog.Warnf("%s is not draining its frames; dropping one", s.game)
		}
	}
	b.mu.RUnlock()

	// Nobody listening is a loss too, and the commonest one: a game runs on a
	// machine of the person's choosing and is off more often than not.
	if delivered == 0 || dropped {
		b.noteMissed(ev.Game, ev.GCID)
	}
}

// noteMissed remembers a table whose frame reached nobody.
func (b *GamingBus) noteMissed(game, gcid string) {
	if gcid == "" {
		return
	}
	b.missedMu.Lock()
	defer b.missedMu.Unlock()
	if b.missed == nil {
		b.missed = make(map[string]map[string]struct{})
	}
	if b.missed[game] == nil {
		b.missed[game] = make(map[string]struct{})
	}
	b.missed[game][gcid] = struct{}{}
}

// noteMissedAll remembers that a game missed frames without knowing which
// tables they were for.
//
// Loss upstream of this process has no gcid to name: the event never arrived,
// so nothing says what it was about. That is the difference between a gap this
// bus observed and one it was told about, and it is why the answer has to be
// "resynchronise everything" rather than a list.
func (b *GamingBus) noteMissedAll(game string) {
	b.missedMu.Lock()
	defer b.missedMu.Unlock()
	if b.missedAll == nil {
		b.missedAll = make(map[string]struct{})
	}
	b.missedAll[game] = struct{}{}
}

// TookMissedAll reports whether a game missed frames whose tables are unknown,
// and forgets it. Taken once, for the same reason as TakeMissed.
func (b *GamingBus) TookMissedAll(game string) bool {
	b.missedMu.Lock()
	defer b.missedMu.Unlock()
	_, ok := b.missedAll[game]
	delete(b.missedAll, game)
	return ok
}

// resyncAllGames tells every registered game that it may have missed anything.
//
// Recorded before the streams are closed, never after: a game that reconnects
// against an unmarked bridge is told it resumed cleanly, which is the failure
// this exists to remove rather than a smaller version of it.
func (b *GamingBus) resyncAllGames(reason string) {
	games := ReadGamingSettings().RegisteredGames
	if len(games) == 0 {
		return
	}
	for _, game := range games {
		b.noteMissedAll(game)
	}
	if fn := b.resync; fn != nil {
		fn(reason)
	}
}

// TakeMissed reports the tables a game missed frames on and forgets them.
//
// Taken rather than read, because it is answered once, into the opening event
// of a stream. A game told the same gap twice would resynchronise twice.
func (b *GamingBus) TakeMissed(game string) []string {
	b.missedMu.Lock()
	defer b.missedMu.Unlock()
	set := b.missed[game]
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for gcid := range set {
		out = append(out, gcid)
	}
	delete(b.missed, game)
	return out
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
		gameLog.Warnf("undecodable frame event: %v", err)
		return
	}
	frame, ok := parseGamingFrame(evt.Message)
	if !ok {
		// brclientd forwards envelopes and nothing else, so reaching
		// here means the two sides disagree about the envelope format.
		gameLog.Warnf("forwarded event from %s is not a frame (%d bytes)",
			evt.From, len(evt.Message))
		return
	}
	if !gamingGameRegistered(frame.Game) {
		// A game this installation does not have. Dropping it is what
		// makes the namespace work: a new game can appear without every
		// existing host being taught about it.
		return
	}
	gameLog.Debugf("delivering %q frame to %d subscriber(s)", frame.Game, b.subscribers(frame.Game))
	b.broadcast(GamingFrameEvent{
		Game:  frame.Game,
		GCID:  evt.GCID,
		From:  evt.From,
		Frame: frame.Text,
	})
}

// gamingGameRegistered reports whether the operator added a game. The
// registered list is the routing table: a frame for anything else is not this
// bridge's business.
func gamingGameRegistered(game string) bool {
	return gamingRegisteredIn(ReadGamingSettings(), game)
}

// gamingRegisteredIn is the same answer against settings already read.
func gamingRegisteredIn(s types.GamingSettings, game string) bool {
	for _, id := range s.RegisteredGames {
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
	if !gamingGameRegistered(game) {
		return ErrGamingGameNotRegistered
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
