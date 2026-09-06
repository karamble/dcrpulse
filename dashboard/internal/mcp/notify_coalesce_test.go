// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A burst of audit rows must not become a burst of goroutines parked in a
// stalled subscriber's write, which is what one notification per row cost.
func TestCoalescedNotifierDoesNotPileUp(t *testing.T) {
	entered := make(chan struct{}, 128)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseAll)

	var calls atomic.Int64
	c := &coalescedNotifier{uri: "dcrpulse://test/coalesce", ch: make(chan struct{}, 1),
		notify: func(string) {
			calls.Add(1)
			entered <- struct{}{}
			<-release
		}}

	c.signal()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the first signal never produced a notification")
	}

	// The sender is now inside a notification that will not return. Every signal
	// raised behind it must still hand back at once.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 64; i++ {
			c.signal()
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("signal blocked while a notification was in flight; recordSpend would wait on it")
	}

	releaseAll()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the signals raised during the notification produced none of their own")
	}
	select {
	case <-entered:
		t.Fatal("more than one notification was queued behind the in-flight one")
	case <-time.After(200 * time.Millisecond):
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("65 signals produced %d notifications, want 2: one in flight and one covering the rest", got)
	}
}

// Coalescing must not swallow the update that arrives while a notification is
// already on its way, or a client that re-reads on the last notification it saw
// misses every row recorded after it.
func TestCoalescedNotifierLosesNothing(t *testing.T) {
	entered := make(chan struct{}, 8)
	proceed := make(chan struct{}, 8)
	c := &coalescedNotifier{uri: "dcrpulse://test/lossless", ch: make(chan struct{}, 1),
		notify: func(string) {
			entered <- struct{}{}
			<-proceed
		}}

	c.signal()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the first signal never produced a notification")
	}

	// The row recorded while the notification is in flight.
	c.signal()
	proceed <- struct{}{}

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("an update raised during a notification was swallowed; a client " +
			"re-reading on the last notification would never see those rows")
	}
	proceed <- struct{}{}
}

// The point of the fix: recordSpend hands the notification to the bounded sender
// instead of spawning one goroutine per row.
func TestRecordSpendUsesTheBoundedNotifier(t *testing.T) {
	useTempAuditFile(t)

	var calls, inFlight, maxInFlight atomic.Int64
	stub := &coalescedNotifier{uri: resAudit, ch: make(chan struct{}, 1),
		notify: func(string) {
			calls.Add(1)
			n := inFlight.Add(1)
			for {
				m := maxInFlight.Load()
				if n <= m || maxInFlight.CompareAndSwap(m, n) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
			inFlight.Add(-1)
		}}
	prev := auditNotify
	auditNotify = stub
	t.Cleanup(func() { auditNotify = prev })

	a := testAgent("notify-bound", "spender", nil)
	for i := 0; i < 64; i++ {
		recordSpend(a, "wallet_send", 0, 0, "", "denied", "over the per-transaction cap")
	}

	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatal("recordSpend did not notify through the bounded sender; it is spawning again")
	}
	time.Sleep(50 * time.Millisecond) // let the last queued notification finish
	if got := maxInFlight.Load(); got != 1 {
		t.Errorf("%d notifications ran at once; the sender must be the only one", got)
	}
	if got := calls.Load(); got >= 64 {
		t.Errorf("64 rows produced %d notifications; the burst was not coalesced", got)
	}
}

// The guarantee coalescing has to keep: some notification starts after the last
// row is in the ring, so a client that re-reads on the last one it saw sees every
// row. Asserted through recordSpend, since that is where the rows come from.
func TestAuditNotificationsSeeEveryRow(t *testing.T) {
	useTempAuditFile(t)

	seen := make(chan string, 64)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseAll)

	stub := &coalescedNotifier{uri: resAudit, ch: make(chan struct{}, 1),
		notify: func(string) {
			// The newest row visible when this notification starts.
			if rows := AuditLog(1); len(rows) > 0 {
				seen <- rows[0].Detail
			} else {
				seen <- ""
			}
			<-release
		}}
	prev := auditNotify
	auditNotify = stub
	t.Cleanup(func() { auditNotify = prev })

	a := testAgent("notify-order", "spender", nil)
	recordSpend(a, "wallet_send", 0, 0, "", "denied", "burst-0")
	select {
	case <-seen:
	case <-time.After(5 * time.Second):
		t.Fatal("a spend produced no audit notification; subscribers are never woken")
	}

	// These land while that notification is in flight, so their signals are
	// dropped; the one pending signal has to cover them.
	const last = "burst-20"
	for i := 1; i <= 20; i++ {
		detail := "burst-" + strconv.Itoa(i)
		recordSpend(a, "wallet_send", 0, 0, "", "denied", detail)
	}
	releaseAll()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case got := <-seen:
			if got == last {
				return
			}
		case <-deadline:
			t.Fatalf("no notification started after %q was recorded; a client re-reading "+
				"on the last notification it saw would miss those rows", last)
		}
	}
}
