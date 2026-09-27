// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/companyzero/bisonrelay/clientrpc/types"
	"github.com/karamble/dcrgaming-sdk/pkg/gaming/bridge"
	gamingwire "github.com/karamble/dcrgaming-sdk/pkg/gaming/wire"
	"google.golang.org/protobuf/encoding/protojson"

	"dcrpulse/internal/rpc"
)

// gamingIntakeRetry bounds the wait between attempts to journal a frame.
const gamingIntakeRetry = 30 * time.Second

// gamingIntakeFirstRetry is the first wait; settable for tests.
var gamingIntakeFirstRetry = time.Second

// StartGamingIntake reads every group message from brclientd's
// ChatService.GCMStream and hands them to the gaming bridge. The stream is
// Bison Relay's own replay log: a message stays there until acknowledged, so
// frames that arrive while the dashboard is down are read on the next start,
// each with the sender's authenticated UID. A message is acknowledged only
// once its frame is in the journal.
func StartGamingIntake(ctx context.Context, b *bridge.Bridge) {
	ws := rpc.BrclientdWS()
	q := newGamingIntakeQueue()
	// The callback runs on the client's read goroutine and must not call back
	// into the client, so it only queues.
	cancel := ws.Subscribe("ChatService.GCMStream", map[string]uint64{"unackedFrom": 0}, q.push)
	go func() {
		defer cancel()
		receive := func(payload json.RawMessage) (uint64, error) {
			return receiveGCM(payload, b.ReceiveGroupMessage)
		}
		runGamingIntake(ctx, q, receive, func(ctx context.Context, seq uint64) error {
			callCtx, cancelCall := context.WithTimeout(ctx, ackTimeout)
			defer cancelCall()
			return ws.Call(callCtx, "ChatService.AckReceivedGCM", map[string]uint64{"sequenceId": seq}, nil)
		})
	}()
	// Frames journaled before a restart but never delivered.
	go b.RecoverHistory()
}

// gamingIntakeQueue holds stream payloads in arrival order until the intake
// loop takes them.
type gamingIntakeQueue struct {
	mu      sync.Mutex
	items   []json.RawMessage
	pending chan struct{}
}

func newGamingIntakeQueue() *gamingIntakeQueue {
	return &gamingIntakeQueue{pending: make(chan struct{}, 1)}
}

func (q *gamingIntakeQueue) push(payload json.RawMessage) {
	q.mu.Lock()
	q.items = append(q.items, append(json.RawMessage(nil), payload...))
	q.mu.Unlock()
	select {
	case q.pending <- struct{}{}:
	default:
	}
}

func (q *gamingIntakeQueue) pop() (json.RawMessage, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil, false
	}
	p := q.items[0]
	q.items = q.items[1:]
	return p, true
}

// runGamingIntake takes messages one at a time and acknowledges the highest
// one handled once the queue is empty. Acks are cumulative, so a message that
// cannot be journaled is retried until it can: acknowledging anything after it
// would drop it from the replay log.
func runGamingIntake(ctx context.Context, q *gamingIntakeQueue,
	receive func(json.RawMessage) (uint64, error),
	ack func(context.Context, uint64) error) {

	var handled, acked uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-q.pending:
		}
		for {
			payload, ok := q.pop()
			if !ok {
				break
			}
			for wait := gamingIntakeFirstRetry; ; wait = min(2*wait, gamingIntakeRetry) {
				seq, err := receive(payload)
				if err == nil {
					handled = max(handled, seq)
					break
				}
				gameLog.Errorf("gaming intake: %v (retrying in %v)", err, wait)
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
				}
			}
		}
		if handled > acked {
			if err := ack(ctx, handled); err != nil {
				if ctx.Err() != nil {
					return
				}
				gameLog.Warnf("gaming intake: ack %d: %v", handled, err)
				continue
			}
			acked = handled
		}
	}
}

// receiveGCM hands one GCMStream message to the bridge through receive and
// returns its sequence id. A payload that cannot be read, or a message that cannot be attributed to
// a sender and group, is skipped: it would never read, and the next ack covers
// it.
func receiveGCM(payload json.RawMessage, receive func(bridge.GroupMessage) error) (uint64, error) {
	var m types.GCReceivedMsg
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(payload, &m); err != nil {
		gameLog.Warnf("gaming intake: undecodable group message: %v", err)
		return 0, nil
	}
	text := m.GetMsg().GetMessage()
	if len(m.GetUid()) != 32 || len(m.GetMsg().GetId()) != 32 {
		if gamingwire.IsEnvelope(text) {
			gameLog.Warnf("gaming intake: frame without a valid sender or group")
		}
		return m.GetSequenceId(), nil
	}
	msg := bridge.GroupMessage{Text: text, Time: time.UnixMilli(m.GetTimestampMs())}
	copy(msg.GCID[:], m.GetMsg().GetId())
	copy(msg.From[:], m.GetUid())
	if err := receive(msg); err != nil {
		return 0, err
	}
	return m.GetSequenceId(), nil
}
