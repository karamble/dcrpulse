package gamingcore

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"

	"dcrpulse/internal/gamingbridge"
)

// Errors a game can be told about. Marked GameSafe because they were written
// for one: a game learns that the tunnel refused, not what else the host is
// carrying. Anything not marked reaches a game as a fixed message.
var (
	ErrGamingGameNotRegistered = gamingbridge.GameSafe(errors.New("game is not registered"))
	ErrGamingNotAFrame         = gamingbridge.GameSafe(errors.New("not a gaming frame"))
	ErrGamingWrongGame         = gamingbridge.GameSafe(errors.New("frame belongs to another game"))
	ErrGamingBadGCID           = gamingbridge.GameSafe(errors.New("not a group chat id"))
	ErrGamingWireVersion       = gamingbridge.GameSafe(errors.New("unsupported gaming wire version"))
	ErrGamingMessageID         = gamingbridge.GameSafe(errors.New("gaming message identity does not match its payload"))
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
// Frames reach the bridge through ChatService.GCMStream, Bison Relay's replay
// log of group messages: each carries the sender's authenticated UID and stays
// in the log until the bridge has journaled it, so nothing a player sends is
// lost to a restart or a slow reader. That matters, because a player who
// misses frames looks exactly like a player who walked away, and is penalised
// as one. The dashboard keeps them out of chat views and refuses a content
// filter that would match them - they are protocol, not conversation.

// GamingFrameEvent is one inbound frame, addressed to a game.
type GamingFrameEvent struct {
	// Seq is assigned and persisted before the event reaches a game.
	Seq uint64 `json:"seq"`
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
	// Financial frames stay inside dcrpulse and are never delivered to a game.
	Financial bool `json:"financial,omitempty"`
}

type gamingSubscriber struct {
	game string
	ch   chan GamingFrameEvent
	last uint64
	once sync.Once
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

	// wireMu serializes durable inbox append with replay subscription. Holding
	// it until a replaying subscriber is registered closes the replay/live gap.
	wireMu      sync.Mutex
	wireDir     string
	wireNext    map[string]uint64
	wireRecords map[string][]GamingFrameEvent
	wireSeen    map[string]struct{}
	// financialDone are financial frames this process already applied.
	financialDone map[string]struct{}

	// prunedGroups are the settled groups whose history this run already
	// dropped; guarded by gamingHistoryRecovery.
	prunedGroups map[string]struct{}
}

// SetGamingResync wires the bridge's stream-closing hook. Called once at
// startup, before any game can connect.
func (b *GamingBus) SetGamingResync(fn func(reason string)) {
	// Durable inbox replay and local BR-history recovery replaced peer resync.
	// The startup hook remains until the surrounding application wiring is
	// removed; it deliberately installs no callback.
	_ = fn
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
		s.once.Do(func() { close(s.ch) })
		b.mu.Unlock()
	}
}

// SubscribeFrom atomically registers a listener and queues every durable frame
// after the sequence it already processed. Live delivery continues on the same
// channel, so there is no replay/live race.
func (b *GamingBus) SubscribeFrom(game string, after uint64, buf int) (<-chan GamingFrameEvent, func()) {
	if buf <= 0 {
		buf = 64
	}
	b.wireMu.Lock()
	backlog, err := b.loadGamingFramesLocked(game, after)
	if err != nil {
		gameLog.Errorf("load %s gaming inbox: %v", game, err)
		ch := make(chan GamingFrameEvent)
		close(ch)
		b.wireMu.Unlock()
		return ch, func() {}
	}
	capacity := buf + len(backlog)
	s := &gamingSubscriber{game: game, ch: make(chan GamingFrameEvent, capacity), last: after}
	b.mu.Lock()
	for _, ev := range backlog {
		s.ch <- ev
		s.last = ev.Seq
	}
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	b.wireMu.Unlock()
	return s.ch, func() {
		b.mu.Lock()
		delete(b.subs, s)
		s.once.Do(func() { close(s.ch) })
		b.mu.Unlock()
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
// A listener that is not draining is closed rather than allowed to lose a
// frame. The SDK reconnects and resumes from its last accepted durable
// sequence; no peer message is sent.
func (b *GamingBus) broadcast(ev GamingFrameEvent) {
	b.mu.Lock()
	for s := range b.subs {
		if s.game != ev.Game {
			continue
		}
		if ev.Seq <= s.last {
			continue
		}
		select {
		case s.ch <- ev:
			s.last = ev.Seq
		default:
			delete(b.subs, s)
			s.once.Do(func() { close(s.ch) })
			gameLog.Warnf("%s is not draining its frames; closing its stream for durable replay", s.game)
		}
	}
	b.mu.Unlock()
}

// deliverGamingMessage is the common intake and history-recovery path. Both
// sources carry the exact stored BR message.
func (b *GamingBus) deliverGamingMessage(gcid, from, message string) bool {
	frame, ok := parseGamingFrame(message)
	if !ok {
		return false
	}
	if !gamingGameRegistered(frame.Game) {
		// A game this installation does not have. Dropping it is what
		// makes the namespace work: a new game can appear without every
		// existing host being taught about it.
		return true
	}
	seated, ok := gamingTableSenders(frame.Game, frame.SID, gcid)
	if !ok {
		// Not a table of this game the operator accepted in this group.
		// Accepting one reads its earlier frames back from the journal.
		return true
	}
	if isFinancialFrame(frame.Text) {
		event := GamingFrameEvent{Game: frame.Game, GCID: gcid, From: from, Frame: frame.Text, Financial: true}
		seq, fresh, err := b.persistGamingFrame(event)
		if err != nil {
			gameLog.Errorf("persist %q financial frame before processing: %v", frame.Game, err)
			return true
		}
		if !fresh {
			return true
		}
		event.Seq = seq
		// Financial wallet/node work must not block the BR notification loop.
		// The durable inbox heals a full worker queue locally.
		select {
		case gamingFinancialInbox <- event:
		default:
			gameLog.Warnf("financial worker queue full; durable message will be replayed locally")
		}

		return true
	}
	if seated != nil && !seated[from] {
		gameLog.Debugf("dropping %q frame from %s, who holds no seat at table %s", frame.Game, from, frame.SID)
		return true
	}
	seq, fresh, err := b.persistGamingFrame(GamingFrameEvent{
		Game: frame.Game, GCID: gcid, From: from, Frame: frame.Text,
	})
	if err != nil {
		gameLog.Errorf("persist %q frame before delivery: %v", frame.Game, err)
		return true
	}
	if !fresh {
		return true
	}
	gameLog.Debugf("delivering %q frame to %d subscriber(s)", frame.Game, b.subscribers(frame.Game))
	b.broadcast(GamingFrameEvent{
		Seq:   seq,
		Game:  frame.Game,
		GCID:  gcid,
		From:  from,
		Frame: frame.Text,
	})
	return true
}

// gamingGameRegistered reports whether the operator added a game. The
// registered list is the routing table: a frame for anything else is not this
// bridge's business.
func gamingGameRegistered(game string) bool {
	return gamingRegisteredIn(ReadGamingSettings(), game)
}

// gamingRegisteredIn is the same answer against settings already read.
func gamingRegisteredIn(s GamingSettings, game string) bool {
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
	if isFinancialFrame(frame) {
		return gamingbridge.GameSafe(errors.New("financial messages are reserved to the bridge"))
	}
	parsed, ok := parseGamingFrame(frame)
	if !ok {
		return ErrGamingNotAFrame
	}
	if parsed.Game != game {
		return ErrGamingWrongGame
	}
	if parsed.Version != "2" {
		return ErrGamingWireVersion
	}
	// The bridge cannot verify a full-payload MID from an isolated fragment.
	// Current game/control messages are bounded to one BR frame; this keeps the
	// hostile game from evading durable deduplication by inventing random MIDs.
	if parsed.Total != 1 || parsed.Seq != 1 || parsed.MID != gamingMessageID(parsed.Game, parsed.GameVersion, parsed.SID, parsed.Payload) {
		return ErrGamingMessageID
	}
	// A game says what it likes inside a frame, because its peers check that;
	// where the frame is sent is the host's decision. Lowercase only, so one
	// group chat cannot be named two ways in the per-game bookkeeping.
	if _, err := parseGamingGCID(gcid); err != nil {
		return err
	}
	if !gamingTableGroup(game, gcid) {
		return gamingbridge.GameSafe(errors.New("not a group of one of this game's tables"))
	}
	// Only the canonical envelope goes out, so no readable text rides along
	// in the header.
	if strings.TrimSpace(frame) != canonicalGamingFrame(parsed.Game, parsed.GameVersion, parsed.SID, parsed.MID, parsed.Expiry, parsed.Payload) {
		return gamingbridge.GameSafe(errors.New("frame is not in canonical form"))
	}
	fresh, err := claimOrReconcileGamingFrame(ctx, game, gcid, parsed, frame)
	if err != nil {
		return gamingbridge.GameSafe(err)
	}
	if !fresh {
		return nil
	}
	return sendClaimedGamingFrame(ctx, game, gcid, parsed, frame)
}

// parseGamingGCID checks a group chat id in the one spelling the gaming paths
// accept: lowercase, so one chat cannot be named two ways in the per-game
// bookkeeping.
func parseGamingGCID(gcid string) ([32]byte, error) {
	var id [32]byte
	if gcid != strings.ToLower(gcid) || len(gcid) != 2*len(id) {
		return id, ErrGamingBadGCID
	}
	if _, err := hex.Decode(id[:], []byte(gcid)); err != nil {
		return [32]byte{}, ErrGamingBadGCID
	}
	return id, nil
}

// ValidGamingGCID is parseGamingGCID for a caller that only needs the verdict.
func ValidGamingGCID(gcid string) bool {
	_, err := parseGamingGCID(gcid)
	return err == nil
}
