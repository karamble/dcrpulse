// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "sync"

// eventBus is the fan-out half of a subsystem's event plumbing: subscribers
// register a buffered channel and every publish tries each one without
// blocking, dropping for anyone who has fallen behind.
//
// The locking here is load-bearing and easy to get subtly wrong, which is why
// there is one implementation rather than one per subsystem. Three rules:
//
//   - The lock is held across the whole send loop. Snapshotting the subscribers
//     and sending after the unlock lets a concurrent cancel close a channel that
//     the snapshot still holds, and a send on a closed channel panics whatever
//     the buffer state; a select's default arm is no protection.
//   - close happens in the same critical section as the removal, for the same
//     reason, and only when the subscriber was still registered, so a second
//     cancel is a no-op rather than a close of a closed channel.
//   - Nothing that can block or take another lock runs inside the loop. The
//     sends are non-blocking so the critical section stays bounded; a stalled
//     log write in here would pin the bus and wedge every unsubscribe behind it.
type eventBus[T any] struct {
	mu   sync.Mutex
	subs []chan T
}

// subscribe registers a listener and returns its channel plus the cleanup func
// to call when it detaches. buf sizes the channel: large enough that a
// momentarily busy consumer does not miss events, small enough that a dead one
// cannot grow without bound.
func (b *eventBus[T]) subscribe(buf int) (<-chan T, func()) {
	ch := make(chan T, buf)
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for i, sub := range b.subs {
			if sub == ch {
				b.subs = append(b.subs[:i], b.subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
}

// publish delivers to every current subscriber, skipping any whose buffer is
// full. Reports how many were skipped so a caller can log it after returning,
// which must not happen while the lock is held.
func (b *eventBus[T]) publish(ev T) int {
	dropped := 0
	b.mu.Lock()
	for _, sub := range b.subs {
		select {
		case sub <- ev:
		default:
			dropped++
		}
	}
	b.mu.Unlock()
	return dropped
}

// eventRing is the history half: a bounded window of the most recent events,
// kept so a page that opens mid-run has something to show before the first live
// event arrives.
type eventRing[T any] struct {
	mu   sync.Mutex
	max  int
	evts []T
}

// add appends and trims to the cap, dropping the oldest.
func (r *eventRing[T]) add(ev T) {
	r.mu.Lock()
	r.evts = append(r.evts, ev)
	if len(r.evts) > r.max {
		r.evts = r.evts[len(r.evts)-r.max:]
	}
	r.mu.Unlock()
}

// last returns a copy of the newest n events, oldest first. n <= 0 or larger
// than the window returns everything held. The copy matters: the caller must
// not be handed a slice that later publishes will write through.
func (r *eventRing[T]) last(n int) []T {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 || n > len(r.evts) {
		n = len(r.evts)
	}
	out := make([]T, n)
	copy(out, r.evts[len(r.evts)-n:])
	return out
}
