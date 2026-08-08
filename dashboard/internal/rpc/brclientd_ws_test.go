// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		name         string
		current      time.Duration
		connectedFor time.Duration
		want         time.Duration
	}{
		{"first failure doubles", wsMinBackoff, 0, 2 * time.Second},
		{"keeps doubling", 4 * time.Second, time.Second, 8 * time.Second},

		// Clamping after doubling is what keeps the ceiling honest. Clamping
		// before it lets 40s double to 80s, well past the 30s maximum.
		{"clamps at the ceiling", 20 * time.Second, time.Second, wsMaxBackoff},
		{"stays at the ceiling", wsMaxBackoff, time.Second, wsMaxBackoff},

		// A connection that ran healthily must not inherit the delay earned by
		// whatever went wrong at startup, or every later drop waits the full
		// ceiling for the life of the process.
		{"healthy connection resets", wsMaxBackoff, wsHealthyFor, wsMinBackoff},
		{"healthy well past the bar resets", wsMaxBackoff, time.Hour, wsMinBackoff},
		{"just short of healthy does not reset", wsMaxBackoff, wsHealthyFor - time.Millisecond, wsMaxBackoff},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextBackoff(tc.current, tc.connectedFor); got != tc.want {
				t.Errorf("nextBackoff(%s, %s) = %s, want %s",
					tc.current, tc.connectedFor, got, tc.want)
			}
		})
	}
}

// wsPair stands up a plaintext WebSocket server and returns the client end plus
// the server end. dialAndServe is bypassed deliberately: it insists on wss and
// pinned certificates, and none of that is what these tests are about.
func wsPair(t *testing.T) (client, server *websocket.Conn) {
	t.Helper()

	up := websocket.Upgrader{}
	serverCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		serverCh <- c
	}))
	t.Cleanup(srv.Close)

	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	select {
	case s := <-serverCh:
		t.Cleanup(func() { s.Close() })
		return c, s
	case <-time.After(5 * time.Second):
		t.Fatal("server never accepted the connection")
		return nil, nil
	}
}

func testClient(conn *websocket.Conn) *BrclientdWSClient {
	c := &BrclientdWSClient{
		pending:         make(map[string]chan inboundMsg),
		streamsByMethod: make(map[string]*subscription),
		closed:          make(chan struct{}),
	}
	c.conn = conn
	return c
}

// A response must reach the Call that is waiting for it.
func TestReadLoopDeliversResponse(t *testing.T) {
	clientConn, serverConn := wsPair(t)
	c := testClient(clientConn)
	go c.readLoop(clientConn)

	// Echo one request back as a result.
	go func() {
		var req map[string]any
		if err := serverConn.ReadJSON(&req); err != nil {
			return
		}
		_ = serverConn.WriteJSON(map[string]any{
			"jsonrpc": "2.0",
			"id":      req["id"],
			"result":  json.RawMessage(`{"ok":true}`),
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var got struct {
		OK bool `json:"ok"`
	}
	if err := c.Call(ctx, "Test.Method", struct{}{}, &got); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !got.OK {
		t.Error("result was not decoded into the caller's value")
	}
}

// readLoop used to look up a pending channel and delete it in two separate
// critical sections, then send outside the lock; failAllPending could close the
// channel in that gap and the send would panic. The fix is to look up and
// delete under one lock, so exactly one party ever owns a channel.
//
// Note on what this test does and does not prove: the old interleaving is a
// panic, not a data race, so -race cannot predict it, and the window is too
// narrow to hit reliably. Reverting the fix does not make this test fail. It is
// a stress test for concurrent calls and teardown, and it does catch a leaked
// pending entry. The atomicity itself holds by construction, not by this test.
func TestPendingCallsSurviveConcurrentTeardown(t *testing.T) {
	clientConn, serverConn := wsPair(t)
	c := testClient(clientConn)

	// Answer everything, so responses and teardown race constantly.
	go func() {
		for {
			var req map[string]any
			if err := serverConn.ReadJSON(&req); err != nil {
				return
			}
			_ = serverConn.WriteJSON(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result":  json.RawMessage(`{}`),
			})
		}
	}()
	go c.readLoop(clientConn)

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			// The error is irrelevant; not panicking is the assertion.
			_ = c.Call(ctx, "Test.Method", struct{}{}, nil)
		}()
	}
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.failAllPending()
		}()
	}
	wg.Wait()

	// Every entry must be accounted for; a leak here means a Call would block
	// until its context expired rather than being woken.
	c.mu.Lock()
	left := len(c.pending)
	c.mu.Unlock()
	if left != 0 {
		t.Errorf("%d pending entries left behind", left)
	}
}

// A Call whose response never arrives must give up on its context rather than
// blocking for the life of the process.
func TestCallHonoursContextDeadline(t *testing.T) {
	clientConn, serverConn := wsPair(t)
	c := testClient(clientConn)

	// Read the request and deliberately never answer it.
	go func() {
		var req map[string]any
		_ = serverConn.ReadJSON(&req)
	}()
	go c.readLoop(clientConn)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- c.Call(ctx, "Test.Method", struct{}{}, nil) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Call returned nil for a request that was never answered")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Call ignored its context deadline")
	}
}
