// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/companyzero/bisonrelay/clientrpc/types"
	"google.golang.org/protobuf/encoding/protojson"

	"dcrpulse/internal/gamingcore"
)

// gcmPayload is a GCMStream event exactly as brclientd's clientrpc sends it.
func gcmPayload(t *testing.T, gcid, uid byte, text string, seq uint64) json.RawMessage {
	t.Helper()
	raw, err := protojson.Marshal(&types.GCReceivedMsg{
		Uid:         bytes.Repeat([]byte{uid}, 32),
		Nick:        "peer",
		Msg:         &types.RMGroupMessage{Id: bytes.Repeat([]byte{gcid}, 32), Message: text},
		TimestampMs: 1700000000000,
		SequenceId:  seq,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestReceiveGCMHandsOnlyAttributableMessages(t *testing.T) {
	var got []gamingcore.GroupMessage
	prev := gamingReceive
	gamingReceive = func(m gamingcore.GroupMessage) error { got = append(got, m); return nil }
	t.Cleanup(func() { gamingReceive = prev })

	seq, err := receiveGCM(gcmPayload(t, 0xaa, 0x22, testFrame, 7))
	if err != nil || seq != 7 || len(got) != 1 {
		t.Fatalf("frame = %d, %v, handed %d", seq, err, len(got))
	}
	if got[0].GCID != [32]byte(bytes.Repeat([]byte{0xaa}, 32)) || got[0].From != [32]byte(bytes.Repeat([]byte{0x22}, 32)) ||
		got[0].Text != testFrame || !got[0].Time.Equal(time.UnixMilli(1700000000000)) {
		t.Fatalf("handed %+v", got[0])
	}
	// A message without a full sender cannot be attributed and is skipped.
	bad := gcmPayload(t, 0xaa, 0x22, testFrame, 9)
	var m types.GCReceivedMsg
	_ = protojson.Unmarshal(bad, &m)
	m.Uid = m.Uid[:16]
	bad, _ = protojson.Marshal(&m)
	if seq, err = receiveGCM(bad); err != nil || seq != 9 {
		t.Fatalf("short uid = %d, %v", seq, err)
	}
	if seq, err = receiveGCM(json.RawMessage(`{`)); err != nil || seq != 0 {
		t.Fatalf("garbage = %d, %v", seq, err)
	}
	if len(got) != 1 {
		t.Fatalf("handed %d messages, want only the attributable one", len(got))
	}
	// A message the bridge could not keep is not acknowledged.
	gamingReceive = func(gamingcore.GroupMessage) error { return errors.New("disk full") }
	if seq, err = receiveGCM(gcmPayload(t, 0xaa, 0x22, testFrame, 10)); err == nil || seq != 0 {
		t.Fatalf("unkept frame = %d, %v", seq, err)
	}
}

type intakeRun struct {
	mu       sync.Mutex
	received []string
	acks     []uint64
}

func (r *intakeRun) got() ([]string, []uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.received...), append([]uint64(nil), r.acks...)
}

func startIntake(t *testing.T, q *gamingIntakeQueue, receive func(string) (uint64, error), ackErr func(uint64) error) (*intakeRun, context.CancelFunc) {
	t.Helper()
	old := gamingIntakeFirstRetry
	gamingIntakeFirstRetry = time.Millisecond
	run := &intakeRun{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runGamingIntake(ctx, q, func(p json.RawMessage) (uint64, error) {
			var s string
			_ = json.Unmarshal(p, &s)
			run.mu.Lock()
			run.received = append(run.received, s)
			run.mu.Unlock()
			return receive(s)
		}, func(_ context.Context, seq uint64) error {
			run.mu.Lock()
			run.acks = append(run.acks, seq)
			run.mu.Unlock()
			return ackErr(seq)
		})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		gamingIntakeFirstRetry = old
	})
	return run, cancel
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestGamingIntakeAcksOnlyWhatIsJournaled(t *testing.T) {
	q := newGamingIntakeQueue()
	failures := 2
	var mu sync.Mutex
	run, _ := startIntake(t, q, func(s string) (uint64, error) {
		mu.Lock()
		defer mu.Unlock()
		if s == "b" && failures > 0 {
			failures--
			return 0, errors.New("disk full")
		}
		return map[string]uint64{"a": 1, "b": 2, "c": 3}[s], nil
	}, func(uint64) error { return nil })

	for _, s := range []string{"a", "b", "c"} {
		raw, _ := json.Marshal(s)
		q.push(raw)
	}
	waitFor(t, func() bool { _, acks := run.got(); return len(acks) > 0 })
	received, acks := run.got()
	if len(received) != 5 || received[1] != "b" || received[2] != "b" || received[3] != "b" || received[4] != "c" {
		t.Fatalf("received %v", received)
	}
	if len(acks) != 1 || acks[0] != 3 {
		t.Fatalf("acks %v", acks)
	}
}

func TestGamingIntakeNeverAcksPastAFrameItCannotJournal(t *testing.T) {
	q := newGamingIntakeQueue()
	run, _ := startIntake(t, q, func(s string) (uint64, error) {
		if s == "b" {
			return 0, errors.New("disk full")
		}
		return map[string]uint64{"a": 1, "c": 3}[s], nil
	}, func(uint64) error { return nil })

	for _, s := range []string{"a", "b", "c"} {
		raw, _ := json.Marshal(s)
		q.push(raw)
	}
	waitFor(t, func() bool { received, _ := run.got(); return len(received) >= 4 })
	received, acks := run.got()
	for _, s := range received[1:] {
		if s != "b" {
			t.Fatalf("moved past the failing frame: %v", received)
		}
	}
	if len(acks) != 0 {
		t.Fatalf("acked %v while a frame was unjournaled", acks)
	}
}

func TestGamingIntakeRetriesAFailedAck(t *testing.T) {
	q := newGamingIntakeQueue()
	fail := true
	var mu sync.Mutex
	run, _ := startIntake(t, q, func(s string) (uint64, error) {
		return map[string]uint64{"a": 1, "b": 2}[s], nil
	}, func(uint64) error {
		mu.Lock()
		defer mu.Unlock()
		if fail {
			fail = false
			return errors.New("socket closed")
		}
		return nil
	})

	raw, _ := json.Marshal("a")
	q.push(raw)
	waitFor(t, func() bool { _, acks := run.got(); return len(acks) == 1 })
	// A reconnect replays the unacknowledged message; its ack is still owed.
	q.push(raw)
	waitFor(t, func() bool { _, acks := run.got(); return len(acks) == 2 })
	if _, acks := run.got(); acks[0] != 1 || acks[1] != 1 {
		t.Fatalf("acks %v", acks)
	}
}
