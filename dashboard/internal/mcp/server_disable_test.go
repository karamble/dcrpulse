// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// surfaceUpForTest marks the agent surface up for one test, as a running
// listener would. The surface is down by default, exactly as in a process that
// has never enabled MCP.
func surfaceUpForTest(t *testing.T) {
	t.Helper()
	surface.setUp()
	t.Cleanup(surface.setDown)
}

// The leak this closes: a subscription stream parked on its context survives the
// listener's shutdown, because net/http never interrupts an active request. The
// toggle has to end it directly.
func TestDisableEndsOpenListenStreams(t *testing.T) {
	surfaceUpForTest(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, ok := surface.addListen("disable-test", cancel)
	if !ok {
		t.Fatal("a listen should register while the surface is up")
	}

	surface.setDown()

	select {
	case <-ctx.Done():
	default:
		t.Fatal("turning the surface off must end the open listen stream")
	}
	// The sweep also forgets it, so a later removeListen is harmless.
	surface.removeListen(id)
}

// No stream may open while the surface is down, or a disable would be followed
// by a fresh feed.
func TestNoListenOpensWhileDown(t *testing.T) {
	surface.setDown()
	if _, ok := surface.addListen("disable-test", func() {}); ok {
		t.Fatal("a listen must not register while the surface is down")
	}
}

// listenGate refuses the listen method specifically, and lets everything else
// through: a tool call in flight during a disable must not be interrupted.
func TestListenGateRefusesOnlyListens(t *testing.T) {
	surface.setDown()

	var reached bool
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		reached = true
		return nil, nil
	}
	gated := listenGate(testAgent("gate-test", "gate", nil))(next)

	if _, err := gated(context.Background(), "subscriptions/listen", nil); !errors.Is(err, errSurfaceDown) {
		t.Fatalf("listen while down: err = %v, want errSurfaceDown", err)
	}
	if reached {
		t.Fatal("a refused listen must not reach the handler")
	}

	reached = false
	if _, err := gated(context.Background(), "tools/call", nil); err != nil {
		t.Fatalf("a tool call must pass the gate even while down: %v", err)
	}
	if !reached {
		t.Fatal("a tool call must reach the handler")
	}
}

// A registered listen is handed a context the sweep can cancel, and the gate
// tidies up after itself when the stream ends on its own.
func TestListenGateRegistersAndReleases(t *testing.T) {
	surfaceUpForTest(t)

	done := make(chan struct{})
	next := func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		close(done)
		<-ctx.Done() // parked, as the real listen handler is
		return nil, nil
	}
	go func() { _, _ = listenGate(testAgent("gate-open", "gate", nil))(next)(context.Background(), "subscriptions/listen", nil) }()
	<-done

	surface.mu.Lock()
	n := len(surface.listens)
	surface.mu.Unlock()
	if n != 1 {
		t.Fatalf("registered listens = %d, want 1", n)
	}

	surface.setDown()
	surface.mu.Lock()
	n = len(surface.listens)
	surface.mu.Unlock()
	if n != 0 {
		t.Fatalf("after the sweep: listens = %d, want 0", n)
	}
}

// The HTTP gate sits outside auth, so a request arriving after a disable is
// refused before it can reach a scoped server.
func TestSurfaceGateRefusesWhileDown(t *testing.T) {
	var inner bool
	h := surfaceGate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { inner = true }))

	surface.setDown()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if inner {
		t.Fatal("a refused request must not reach the inner handler")
	}

	surfaceUpForTest(t)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if !inner {
		t.Fatal("a request must pass while the surface is up")
	}
}

// A discarded server keeps whatever listens were open against it, and nothing
// notifies it again, so those streams would go quiet with neither side told.
// The sweep is per agent: it must not touch anyone else's, and it must not take
// the surface down the way the off-toggle does.
func TestEndAgentListensIsPerAgent(t *testing.T) {
	surfaceUpForTest(t)

	mineA, mineB, theirs := make(chan struct{}), make(chan struct{}), make(chan struct{})
	for _, reg := range []struct {
		agent string
		done  chan struct{}
	}{{"sweep-me", mineA}, {"sweep-me", mineB}, {"leave-me", theirs}} {
		done := reg.done
		if _, ok := surface.addListen(reg.agent, func() { close(done) }); !ok {
			t.Fatalf("%s could not register a listen", reg.agent)
		}
	}

	if n := surface.endAgentListens("sweep-me"); n != 2 {
		t.Errorf("swept %d listens, want 2", n)
	}
	for name, ch := range map[string]chan struct{}{"first": mineA, "second": mineB} {
		select {
		case <-ch:
		default:
			t.Errorf("the agent's %s listen was not ended", name)
		}
	}
	select {
	case <-theirs:
		t.Error("another agent's listen was ended by a per-agent sweep")
	default:
	}

	if !surface.isUp() {
		t.Error("the sweep took the surface down; it is not a shutdown")
	}
	surface.mu.Lock()
	left := len(surface.listens)
	surface.mu.Unlock()
	if left != 1 {
		t.Errorf("%d entries left, want only the other agent's: the swept slots must free at once", left)
	}
}

// Freeing the slot is the finding's real bite: removeListen runs in the gate's
// defer, which fires when the parked handler returns, and invalidation does not
// cause that. So the sweep has to drop the entry itself or the agent loses a slot
// per subscribed resource, permanently.
func TestSweptListensFreeTheirSlots(t *testing.T) {
	surfaceUpForTest(t)

	for i := 0; i < maxListensPerAgent; i++ {
		if _, ok := surface.addListen("slot-agent", func() {}); !ok {
			t.Fatalf("could not register listen %d", i+1)
		}
	}
	if _, ok := surface.addListen("slot-agent", func() {}); ok {
		t.Fatal("the cap did not apply, so this test would assert nothing")
	}

	surface.endAgentListens("slot-agent")

	for i := 0; i < maxListensPerAgent; i++ {
		if _, ok := surface.addListen("slot-agent", func() {}); !ok {
			t.Fatalf("only %d slots came back after the sweep; a swept stream still holds its slot", i)
		}
	}
}
