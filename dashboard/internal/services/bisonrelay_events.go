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

	"dcrpulse/internal/rpc"
)

// BisonrelayEvent is the wrapper the dashboard broadcasts to browser-WS
// subscribers. Type is one of "pm", "kx", "gcm". Payload is the raw event
// JSON brclientd emitted; the browser decides how to render each kind.
type BisonrelayEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type bisonrelaySubscriber struct {
	ch chan BisonrelayEvent
}

type BisonrelayEventBus struct {
	mu          sync.RWMutex
	subscribers map[*bisonrelaySubscriber]struct{}
}

var (
	bisonrelayBus     *BisonrelayEventBus
	bisonrelayBusOnce sync.Once
)

// Bisonrelay returns the singleton event bus. The browser-facing WS handler
// and the brclientd stream consumers both register through it.
func Bisonrelay() *BisonrelayEventBus {
	bisonrelayBusOnce.Do(func() {
		bisonrelayBus = &BisonrelayEventBus{
			subscribers: make(map[*bisonrelaySubscriber]struct{}),
		}
	})
	return bisonrelayBus
}

// Subscribe registers a browser-WS handler and returns a buffered channel
// it should drain. The cancel func removes the subscriber.
func (b *BisonrelayEventBus) Subscribe(buf int) (<-chan BisonrelayEvent, func()) {
	if buf <= 0 {
		buf = 32
	}
	s := &bisonrelaySubscriber{ch: make(chan BisonrelayEvent, buf)}
	b.mu.Lock()
	b.subscribers[s] = struct{}{}
	b.mu.Unlock()
	return s.ch, func() {
		b.mu.Lock()
		delete(b.subscribers, s)
		b.mu.Unlock()
		close(s.ch)
	}
}

func (b *BisonrelayEventBus) broadcast(evt BisonrelayEvent) {
	b.mu.RLock()
	subs := make([]*bisonrelaySubscriber, 0, len(b.subscribers))
	for s := range b.subscribers {
		subs = append(subs, s)
	}
	b.mu.RUnlock()
	for _, s := range subs {
		select {
		case s.ch <- evt:
		default:
			log.Printf("br event bus: subscriber buffer full, dropping %s", evt.Type)
		}
	}
}

// StartBisonrelayStreams subscribes to brclientd's KX-completion and
// download-completion clientrpc streams and broadcasts each event into the
// dashboard event bus, Acking immediately since brclientd's clientdb is the
// source of truth. PM and GC messages are not sourced here; they arrive via
// StartBrclientdNotifs (the in-process /notifications stream). Blocks until
// ctx is cancelled.
func StartBisonrelayStreams(ctx context.Context) {
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
	go runStream(ctx, ws, "ChatService.DownloadsCompletedStream", "ChatService.AckDownloadCompleted", "download", func(payload json.RawMessage) (any, bool) {
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
			err := rpc.BrclientdStreamNotifications(attemptCtx, func(evt rpc.BrclientdNotifEvent) {
				bus.broadcast(BisonrelayEvent{Type: evt.Type, Payload: evt.Payload})
				backoff = minBackoff
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

func runStream(
	ctx context.Context,
	ws *rpc.BrclientdWSClient,
	method, ackMethod, evType string,
	ackParams func(json.RawMessage) (any, bool),
) {
	bus := Bisonrelay()
	cancel, err := ws.Subscribe(method, struct{}{}, func(payload json.RawMessage) {
		bus.broadcast(BisonrelayEvent{Type: evType, Payload: payload})
		if p, ok := ackParams(payload); ok {
			if err := ws.Call(ctx, ackMethod, p, nil); err != nil && ctx.Err() == nil {
				log.Printf("br %s ack: %v", evType, err)
			}
		}
	})
	if err != nil {
		log.Printf("br %s subscribe: %v", evType, err)
		return
	}
	<-ctx.Done()
	cancel()
}
