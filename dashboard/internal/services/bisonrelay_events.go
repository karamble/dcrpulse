// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"log"
	"sync"

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

// StartBisonrelayStreams subscribes to brclientd's chat streams and
// broadcasts each event into the dashboard event bus, Acking immediately
// since brclientd's clientdb is the source of truth for chat history.
// Blocks until ctx is cancelled.
func StartBisonrelayStreams(ctx context.Context) {
	ws := rpc.BrclientdWS()
	go func() {
		if err := ws.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("brclientd-ws run exited: %v", err)
		}
	}()
	go runStream(ctx, ws, "ChatService.PMStream", "ChatService.AckReceivedPM", "pm", func(payload json.RawMessage) (any, bool) {
		var pm struct {
			SequenceID int64 `json:"sequenceId"`
		}
		_ = json.Unmarshal(payload, &pm)
		if pm.SequenceID == 0 {
			return nil, false
		}
		return map[string]int64{"sequenceId": pm.SequenceID}, true
	})
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
	go runStream(ctx, ws, "ChatService.GCMStream", "ChatService.AckReceivedGCM", "gcm", func(payload json.RawMessage) (any, bool) {
		var gcm struct {
			SequenceID int64 `json:"sequenceId"`
		}
		_ = json.Unmarshal(payload, &gcm)
		if gcm.SequenceID == 0 {
			return nil, false
		}
		return map[string]int64{"sequenceId": gcm.SequenceID}, true
	})
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
