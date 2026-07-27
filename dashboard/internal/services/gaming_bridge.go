package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
)

// Errors a game can be told about. They are deliberately unrevealing: a game
// learns that the tunnel refused, not what else the host is carrying.
var (
	ErrGamingGameNotInstalled = errors.New("game is not installed")
	ErrGamingNotAFrame        = errors.New("not a gaming frame")
	ErrGamingWrongGame        = errors.New("frame belongs to another game")
)

// The gaming bridge is a tunnel between installed games and Bison Relay.
//
// Games are untrusted. They run beside a wallet, a dcrlnd node and a Bison
// Relay identity, and they play for real money against strangers - so they are
// never handed the credentials for any of it. The bridge holds brclientd's
// client certificate and does the talking, which is the entire reason it
// exists.
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

// StartGamingBridge subscribes to inbound group chat messages and routes game
// frames. It blocks until ctx is cancelled.
func StartGamingBridge(ctx context.Context) {
	ws := rpc.BrclientdWS()
	bus := Gaming()

	cancel, err := ws.Subscribe("ChatService.GCMStream", struct{}{}, func(payload json.RawMessage) {
		var msg struct {
			UID        []byte `json:"uid"`
			SequenceID int64  `json:"sequenceId"`
			Msg        struct {
				ID      []byte `json:"id"`
				Message string `json:"message"`
			} `json:"msg"`
		}
		if err := json.Unmarshal(payload, &msg); err != nil {
			return
		}

		// Ack whatever arrives, frame or not. The stream is shared with
		// the chat path's own delivery and resumes from the last acked
		// id; declining to ack a chat message would replay it forever.
		if msg.SequenceID != 0 {
			ackParams := map[string]int64{"sequenceId": msg.SequenceID}
			if err := ws.Call(ctx, "ChatService.AckReceivedGCM", ackParams, nil); err != nil && ctx.Err() == nil {
				log.Printf("gaming bridge: ack gcm: %v", err)
			}
		}

		frame, ok := parseGamingFrame(msg.Msg.Message)
		if !ok {
			return // ordinary conversation
		}
		if !gamingGameInstalled(frame.Game) {
			// A game this installation does not have. Dropping it is
			// what makes the namespace work: a new game can appear
			// without every existing host being taught about it.
			return
		}
		bus.broadcast(GamingFrameEvent{
			Game:  frame.Game,
			GCID:  hex.EncodeToString(msg.Msg.ID),
			From:  hex.EncodeToString(msg.UID),
			Frame: frame.Text,
		})
	})
	if err != nil {
		log.Printf("gaming bridge: subscribe to GCMStream: %v", err)
		return
	}
	log.Printf("gaming bridge: routing game frames")
	<-ctx.Done()
	cancel()
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

// maxBundleBytes bounds a game binary. A static Go binary is tens of megabytes;
// this leaves room without letting a bad URL fill the sandbox's volume.
const maxBundleBytes = 128 << 20

// FetchGamingBundle retrieves a game's binary, or its signature, on the
// sandbox's behalf.
//
// The sandbox has no route off the host, so it cannot fetch anything itself.
// That is not a limitation to work around: it makes this the single point where
// anything enters the sandbox, and the host can refuse a game the user never
// installed rather than discovering afterwards what was downloaded.
//
// The host does not verify the signature. The portal does, because the portal
// is what executes the binary, and the thing that runs code should be the thing
// that checks it - if this host were compromised it still could not put
// arbitrary code into the sandbox.
func FetchGamingBundle(ctx context.Context, game string, signature bool) (io.ReadCloser, error) {
	if !gamingGameInstalled(game) {
		return nil, ErrGamingGameNotInstalled
	}
	var entry *types.GamingGame
	for i := range gamingCatalogue {
		if gamingCatalogue[i].ID == game {
			entry = &gamingCatalogue[i]
			break
		}
	}
	if entry == nil {
		return nil, ErrGamingGameNotInstalled
	}

	// The URL comes from this build's catalogue, never from the caller. A
	// game that could name its own URL could ask the host to fetch anything
	// reachable from here, which is precisely what the sandbox gives up.
	url := entry.BundleURL
	if signature {
		url = entry.BundleSigURL
	}
	if url == "" {
		return nil, fmt.Errorf("no bundle published for %s", game)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build bundle request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch bundle: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("bundle source returned %s", resp.Status)
	}
	return readCloser{Reader: io.LimitReader(resp.Body, maxBundleBytes), Closer: resp.Body}, nil
}

type readCloser struct {
	io.Reader
	io.Closer
}
