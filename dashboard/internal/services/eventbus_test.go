// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"sync"
	"testing"
)

// Every subsystem's bus used to be its own copy of this code, and the one copy
// that was written differently was the one that had the bug: it snapshotted the
// subscribers under the lock, released, and only then sent, so a cancel landing
// in that gap closed a channel the snapshot still held. A send on a closed
// channel panics whatever the buffer state, and the select's default arm is no
// protection. Both hot publishers ran on background goroutines with no recover
// above them, so it took the process down.
//
// A panic in any goroutine takes the test binary with it, so surviving the run
// IS the assertion here. Verified non-vacuous: drop the locking in publish back
// to a snapshot-then-send and this crashes with "send on closed channel" well
// inside one round.
func TestEventBusPublishRacesCancel(t *testing.T) {
	var b eventBus[int]

	var publishers, subscribers sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		publishers.Add(1)
		go func() {
			defer publishers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					b.publish(1)
				}
			}
		}()
	}

	// Subscribers that come and go the way browser tabs do. A buffer of one
	// with nothing draining it exercises the drop path too.
	for i := 0; i < 8; i++ {
		subscribers.Add(1)
		go func() {
			defer subscribers.Done()
			for j := 0; j < 250; j++ {
				_, cancel := b.subscribe(1)
				cancel()
			}
		}()
	}

	subscribers.Wait()
	close(stop)
	publishers.Wait()
}

// A second cancel is a silent no-op. The removal loop simply finds nothing the
// second time; were it to close unconditionally it would panic, and do so while
// holding the lock.
func TestEventBusCancelIsIdempotent(t *testing.T) {
	var b eventBus[int]
	_, cancel := b.subscribe(1)
	cancel()
	cancel()
}

func TestEventBusDeliversAndDropsWhenFull(t *testing.T) {
	var b eventBus[string]
	ch, cancel := b.subscribe(1)
	defer cancel()

	if dropped := b.publish("first"); dropped != 0 {
		t.Errorf("dropped %d on an empty buffer, want 0", dropped)
	}
	select {
	case got := <-ch:
		if got != "first" {
			t.Errorf("got %q, want %q", got, "first")
		}
	default:
		t.Fatal("subscriber received nothing")
	}

	// Fill the one slot, then overflow it. The publisher must return rather
	// than block, must report the drop, and must not evict what is queued.
	if dropped := b.publish("second"); dropped != 0 {
		t.Errorf("dropped %d filling an empty slot, want 0", dropped)
	}
	if dropped := b.publish("dropped"); dropped != 1 {
		t.Errorf("dropped %d overflowing the buffer, want 1", dropped)
	}
	if got := <-ch; got != "second" {
		t.Errorf("got %q, want the queued %q", got, "second")
	}
	select {
	case got := <-ch:
		t.Errorf("expected an empty channel, got %q", got)
	default:
	}
}

// Cancelling one subscriber must not disturb the others, which is the part a
// slice-removal loop is easy to get wrong.
func TestEventBusCancelLeavesOthersAlone(t *testing.T) {
	var b eventBus[int]
	a, cancelA := b.subscribe(4)
	c, cancelC := b.subscribe(4)
	defer cancelC()

	cancelA()
	b.publish(7)

	select {
	case got := <-c:
		if got != 7 {
			t.Errorf("surviving subscriber got %d, want 7", got)
		}
	default:
		t.Fatal("surviving subscriber received nothing after a sibling cancelled")
	}
	if _, open := <-a; open {
		t.Error("cancelled subscriber's channel is still open")
	}
}

func TestEventRingBoundsAndOrder(t *testing.T) {
	r := eventRing[int]{max: 3}
	for i := 1; i <= 5; i++ {
		r.add(i)
	}

	// Oldest first, oldest dropped past the cap.
	got := r.last(0)
	want := []int{3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("last(0) returned %d events, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("last(0)[%d] = %d, want %d", i, got[i], want[i])
		}
	}

	if n := len(r.last(2)); n != 2 {
		t.Errorf("last(2) returned %d, want 2", n)
	}
	if n := len(r.last(99)); n != 3 {
		t.Errorf("last(99) returned %d, want everything held, 3", n)
	}
	if n := len(r.last(-1)); n != 3 {
		t.Errorf("last(-1) returned %d, want everything held, 3", n)
	}
}

func TestEventRingEmpty(t *testing.T) {
	r := eventRing[int]{max: 3}
	if n := len(r.last(0)); n != 0 {
		t.Errorf("last(0) on an empty ring returned %d, want 0", n)
	}
	if n := len(r.last(5)); n != 0 {
		t.Errorf("last(5) on an empty ring returned %d, want 0", n)
	}
}

// The reader must hand back a copy. Aliasing the backing array would let a
// later add rewrite a slice an HTTP handler is still serialising.
func TestEventRingLastReturnsACopy(t *testing.T) {
	r := eventRing[int]{max: 4}
	r.add(1)
	r.add(2)

	got := r.last(0)
	got[0] = 99
	if again := r.last(0); again[0] != 1 {
		t.Errorf("mutating the returned slice changed the ring: got %d, want 1", again[0])
	}
}

func TestEventRingConcurrentAddAndRead(t *testing.T) {
	r := eventRing[int]{max: 50}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			r.add(i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			if n := len(r.last(10)); n > 10 {
				t.Errorf("last(10) returned %d events", n)
				return
			}
		}
	}()
	wg.Wait()

	if n := len(r.last(0)); n != 50 {
		t.Errorf("ring holds %d events, want its cap of 50", n)
	}
}
