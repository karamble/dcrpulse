// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package services

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dcrpulse/internal/gamingbridge"
)

func waitGamingForward(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("forwarder did not stop")
	}
}

func TestGamingForwardFullBufferConcurrentStop(t *testing.T) {
	for _, capacity := range []int{1, 64} {
		in := make(chan GamingFrameEvent)
		var calls atomic.Int32
		out, stop := forwardGamingFrames(in, func() { calls.Add(1); close(in) }, capacity)
		for i := 0; i <= capacity; i++ {
			// The last handoff leaves the worker unable to send: out is full.
			in <- GamingFrameEvent{Seq: uint64(i + 1)}
		}
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); stop() }()
		}
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		waitGamingForward(t, done)
		// Only inspect buffered values after every stop has returned. Draining
		// earlier would unblock the old worker and conceal the original leak.
		for i := 0; i < capacity; i++ {
			if got := <-out; got.Seq != uint64(i+1) {
				t.Fatalf("buffered sequence = %d", got.Seq)
			}
		}
		select {
		case _, ok := <-out:
			if ok {
				t.Fatal("blocked frame sent after stop returned")
			}
		default:
			t.Fatal("worker has not closed output")
		}
		if calls.Load() != 1 {
			t.Fatalf("unsubscribe called %d times", calls.Load())
		}
	}
}

func TestGamingForwardCancelPreservesDurableReplay(t *testing.T) {
	withGamingWireDir(t)
	b := &GamingBus{subs: make(map[*gamingSubscriber]struct{})}
	var payloads []string
	for i := 0; i < 8; i++ {
		ev := GamingFrameEvent{Game: "poker", GCID: "table", From: "alice",
			Frame: strings.Replace(testFrame, "seq=1/1", "seq=1/"+strconv.Itoa(i+1), 1)}
		payloads = append(payloads, ev.Frame)
		if _, _, err := b.persistGamingFrame(ev); err != nil {
			t.Fatal(err)
		}
	}
	in, unsubscribe := b.SubscribeFrom("poker", 0, 1)
	out, stop := forwardGamingFrames(in, unsubscribe, 1)
	accepted := <-out
	if accepted.Seq != 1 {
		t.Fatalf("first accepted sequence = %d", accepted.Seq)
	}
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	waitGamingForward(t, done)
	if b.subscribers("poker") != 0 {
		t.Fatal("cancelled subscriber remains registered")
	}
	// A new bus proves recovery uses durable storage, not abandoned channels.
	restarted := &GamingBus{subs: make(map[*gamingSubscriber]struct{})}
	replay, cancel := restarted.SubscribeFrom("poker", accepted.Seq, 1)
	frames, finish := forwardGamingFrames(replay, cancel, 1)
	defer finish()
	for seq := uint64(2); seq <= 8; seq++ {
		select {
		case got := <-frames:
			if got.Seq != seq || got.GCID != "table" || got.From != "alice" || got.Frame != payloads[seq-1] {
				t.Fatalf("replayed %+v at sequence %d", got, seq)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("unaccepted durable frame missing")
		}
	}
	live := GamingFrameEvent{Game: "poker", GCID: "table", From: "bob", Frame: "live"}
	var err error
	live.Seq, _, err = restarted.persistGamingFrame(live)
	if err != nil {
		t.Fatal(err)
	}
	restarted.broadcast(live)
	select {
	case got := <-frames:
		if got.Seq != 9 || got.Frame != "live" || got.From != "bob" {
			t.Fatalf("live delivery after replay: %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("live delivery missing")
	}
}

func TestGamingForwardCancellation(t *testing.T) {
	for _, mode := range []string{"idle", "blocked", "closed-input"} {
		t.Run(mode, func(t *testing.T) {
			in := make(chan GamingFrameEvent)
			var calls atomic.Int32
			var closeInput sync.Once
			out, stop := forwardGamingFrames(in, func() {
				calls.Add(1)
				closeInput.Do(func() { close(in) })
			}, 0)
			if mode != "idle" {
				// The handoff proves the worker has received a frame. With no
				// output reader it cannot finish forwarding before cancellation.
				in <- GamingFrameEvent{Seq: 1, Frame: "pending"}
			}
			if mode == "closed-input" {
				closeInput.Do(func() { close(in) })
			}
			done := make(chan struct{})
			go func() { stop(); close(done) }()
			waitGamingForward(t, done)
			select {
			case _, ok := <-out:
				if ok {
					t.Fatal("cancelled forwarder still sends pending input")
				}
			default:
				t.Fatal("stop returned before output closed")
			}
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); stop() }()
			}
			wg.Wait()
			if calls.Load() != 1 {
				t.Fatalf("unsubscribe called %d times", calls.Load())
			}
		})
	}
}

func TestGamingForwardDrainsClosedInput(t *testing.T) {
	in := make(chan GamingFrameEvent, 3)
	for seq := uint64(1); seq <= 3; seq++ {
		in <- GamingFrameEvent{Seq: seq, GCID: "table", From: "alice", Frame: "payload"}
	}
	close(in)
	out, stop := forwardGamingFrames(in, func() {}, 1)
	defer stop()
	for seq := uint64(1); seq <= 3; seq++ {
		select {
		case got := <-out:
			want := gamingbridge.Frame{Seq: seq, GCID: "table", From: "alice", Frame: "payload"}
			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("queued input was not drained")
		}
	}
	if _, ok := <-out; ok {
		t.Fatal("output remained open")
	}
}
