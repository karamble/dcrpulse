// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"dcrpulse/internal/alerts"
	"dcrpulse/internal/rpc"
)

// BisonrelayEvent is the wrapper the dashboard broadcasts to browser-WS
// subscribers. Type is one of "pm", "kx", "gcm". Payload is the raw event
// JSON brclientd emitted; the browser decides how to render each kind.
type BisonrelayEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type BisonrelayEventBus struct {
	bus eventBus[BisonrelayEvent]
}

var (
	bisonrelayBus     *BisonrelayEventBus
	bisonrelayBusOnce sync.Once
)

// Bisonrelay returns the singleton event bus. The browser-facing WS handler
// and the brclientd stream consumers both register through it.
func Bisonrelay() *BisonrelayEventBus {
	bisonrelayBusOnce.Do(func() {
		bisonrelayBus = &BisonrelayEventBus{}
	})
	return bisonrelayBus
}

// Subscribe registers a browser-WS handler and returns a buffered channel
// it should drain. The cancel func removes the subscriber.
func (b *BisonrelayEventBus) Subscribe(buf int) (<-chan BisonrelayEvent, func()) {
	if buf <= 0 {
		buf = 32
	}
	return b.bus.subscribe(buf)
}

// PublishBisonrelayEvent lets dashboard subsystems push an event of their
// own to every connected browser listener. The payload reaches all open
// sessions, so callers must keep it free of anything sensitive.
func PublishBisonrelayEvent(evtType string, payload json.RawMessage) {
	Bisonrelay().broadcast(BisonrelayEvent{Type: evtType, Payload: payload})
}

func (b *BisonrelayEventBus) broadcast(evt BisonrelayEvent) {
	if dropped := b.bus.publish(evt); dropped > 0 {
		log.Printf("br event bus: subscriber buffer full, dropping %s (%d subscribers)", evt.Type, dropped)
	}
}

// StartBisonrelayStreams subscribes to brclientd's KX-completion and
// download-completion clientrpc streams and broadcasts each event into the
// dashboard event bus, Acking immediately since brclientd's clientdb is the
// source of truth. PM and GC messages are not sourced here; they arrive via
// StartBrclientdNotifs (the in-process /notifications stream). Blocks until
// ctx is cancelled.
func StartBisonrelayStreams(ctx context.Context) {
	// A watch-only wallet has no Bison Relay daemon; idle the clientrpc WS loop
	// for it instead of spamming cert-not-found reconnects.
	rpc.SkipBrclientdWS = func() bool { return ActiveWalletIsWatchOnly(context.Background()) }
	ws := rpc.BrclientdWS()
	go func() {
		if err := ws.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("brclientd-ws run exited: %v", err)
		}
	}()
	// PM and GC messages are delivered via brclientd's in-process /notifications
	// stream ("pm" / "gc-message"), which fires once per newly-received message.
	// We deliberately do not subscribe to ChatService.PMStream/GCMStream here:
	// those are replay-log streams that re-stream the whole backlog on every
	// (re)subscribe, which re-badged already-read conversations on restart.
	go runStream(ctx, ws, "ChatService.KXStream", "ChatService.AckKXCompleted", "kx", func(payload json.RawMessage) (any, bool) {
		var kx struct {
			SequenceID int64 `json:"sequenceId"`
		}
		_ = json.Unmarshal(payload, &kx)
		if kx.SequenceID == 0 {
			return nil, false
		}
		return map[string]int64{"sequenceId": kx.SequenceID}, true
	})
	go runStream(ctx, ws, "ContentService.DownloadsCompletedStream", "ContentService.AckDownloadCompleted", "download", func(payload json.RawMessage) (any, bool) {
		var dl struct {
			SequenceID int64 `json:"sequenceId"`
		}
		_ = json.Unmarshal(payload, &dl)
		if dl.SequenceID == 0 {
			return nil, false
		}
		return map[string]int64{"sequenceId": dl.SequenceID}, true
	})
}

// notifReconnect cancels the in-flight /notifications attempt so the stream
// redials immediately (e.g. after a wallet switch repoints the brclientd certs).
var (
	notifReconnectMu sync.Mutex
	notifReconnect   context.CancelFunc
)

// ReconnectBrclientdNotifs drops the current /notifications stream so it redials
// at once, rebuilding its cert-pinned client. No-op if the stream is not
// running yet.
func ReconnectBrclientdNotifs() {
	notifReconnectMu.Lock()
	cancel := notifReconnect
	notifReconnectMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// StartBrclientdNotifs subscribes to brclientd's /notifications JSONL
// stream and rebroadcasts each event to the dashboard event bus, using the
// same {type, payload} envelope BR's clientrpc streams use. Reconnects on
// failure with a short backoff.
func StartBrclientdNotifs(ctx context.Context) {
	go func() {
		bus := Bisonrelay()
		const minBackoff = 2 * time.Second
		const maxBackoff = 30 * time.Second
		backoff := minBackoff
		for {
			if ctx.Err() != nil {
				return
			}
			attemptCtx, cancel := context.WithCancel(ctx)
			notifReconnectMu.Lock()
			notifReconnect = cancel
			notifReconnectMu.Unlock()

			// A watch-only wallet has no Bison Relay daemon; idle (interruptibly)
			// instead of dialing brclientd, so it does not spam cert-not-found
			// reconnects. A switch to a full wallet calls ReconnectBrclientdNotifs,
			// which cancels attemptCtx and wakes the idle to re-check.
			if ActiveWalletIsWatchOnly(ctx) {
				select {
				case <-attemptCtx.Done():
				case <-time.After(minBackoff):
				}
				cancel()
				continue
			}

			err := rpc.BrclientdStreamNotifications(attemptCtx, func(evt rpc.BrclientdNotifEvent) {
				// Keepalives prove the stream is healthy but carry
				// nothing for the browser fan-out or the event bus.
				if evt.Type != "keepalive" {
					bus.broadcast(BisonrelayEvent{Type: evt.Type, Payload: evt.Payload})
				}
				backoff = minBackoff
				alerts.Resolve("br_disconnected", "")
			})
			forced := attemptCtx.Err() != nil
			cancel()
			if ctx.Err() != nil {
				return
			}
			if forced {
				// Cert change on a wallet switch: redial now, no error log.
				backoff = minBackoff
				continue
			}
			if err != nil {
				log.Printf("brclientd notifications stream: %v (reconnecting in %s)", err, backoff)
				alerts.Raise("br_disconnected", "", err.Error())
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}()
}

// ackTimeout bounds a single acknowledgement. Pruning brclientd's replay log is
// housekeeping: if the daemon is slow, give up and retry on the next event
// rather than pinning the acker for the life of the process.
const ackTimeout = 30 * time.Second

func runStream(
	ctx context.Context,
	ws *rpc.BrclientdWSClient,
	method, ackMethod, evType string,
	ackParams func(json.RawMessage) (any, bool),
) {
	bus := Bisonrelay()

	// The subscriber callback runs on the client's read goroutine, so it must
	// not call back into the client: acking from there blocks waiting for a
	// response only that same goroutine can deliver, which wedges the socket
	// for good. The callback therefore only records what to ack, and a
	// separate goroutine below does the calling.
	acks := newAckTracker()

	cancel, err := ws.Subscribe(method, struct{}{}, func(payload json.RawMessage) {
		bus.broadcast(BisonrelayEvent{Type: evType, Payload: payload})
		if p, ok := ackParams(payload); ok {
			if seq, ok := sequenceIDOf(p); ok {
				acks.record(seq)
			}
		}
	})
	if err != nil {
		log.Printf("br %s subscribe: %v", evType, err)
		return
	}
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		case <-acks.pending:
		}

		seq, ok := acks.next()
		if !ok {
			continue
		}

		callCtx, cancelCall := context.WithTimeout(ctx, ackTimeout)
		err := ws.Call(callCtx, ackMethod, map[string]int64{"sequenceId": seq}, nil)
		cancelCall()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("br %s ack %d: %v", evType, seq, err)
			continue
		}
		acks.confirm(seq)
	}
}

// ackTracker holds the highest sequence id seen for one stream and wakes the
// acker when it advances.
//
// Acks are cumulative: acknowledging a sequence id clears brclientd's replay
// log at or below it, and doing so twice is harmless. So a burst of events
// needs only one call, for the highest id, and only ever one is in flight.
type ackTracker struct {
	mu      sync.Mutex
	highest int64
	acked   int64
	// pending is a coalescing nudge, the same shape as TriggerNodeSyncRefresh:
	// a burst collapses into one wake-up and the acker reads the latest mark
	// when it gets there.
	pending chan struct{}
}

func newAckTracker() *ackTracker {
	return &ackTracker{pending: make(chan struct{}, 1)}
}

// record notes a sequence id. Ids may arrive out of order, so the mark only
// ever moves forward.
func (a *ackTracker) record(seq int64) {
	a.mu.Lock()
	advanced := seq > a.highest
	if advanced {
		a.highest = seq
	}
	a.mu.Unlock()
	if !advanced {
		return
	}
	select {
	case a.pending <- struct{}{}:
	default:
	}
}

// next reports the id to acknowledge, or false when nothing new has arrived.
func (a *ackTracker) next() (int64, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.highest <= a.acked {
		return 0, false
	}
	return a.highest, true
}

// confirm records a successful ack. A failed one is deliberately not recorded,
// so the next event retries it.
func (a *ackTracker) confirm(seq int64) {
	a.mu.Lock()
	if seq > a.acked {
		a.acked = seq
	}
	a.mu.Unlock()
}

// sequenceIDOf reads the sequence id back out of the ack parameters built by
// the stream's ackParams closure.
func sequenceIDOf(params any) (int64, bool) {
	m, ok := params.(map[string]int64)
	if !ok {
		return 0, false
	}
	seq, ok := m["sequenceId"]
	return seq, ok
}
