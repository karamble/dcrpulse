// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
	"sync"
	"testing"
	"time"
)

// pipeListener hands the server one end of a net.Pipe. Loopback is no good for
// this: the kernel absorbs thousands of notifications before a write blocks, so
// a test built on it passes with the deadline removed. net.Pipe is synchronous
// and unbuffered, so the first unread write blocks immediately, and it honours
// SetWriteDeadline.
type pipeListener struct {
	conns    chan net.Conn
	done     chan struct{}
	closeOne sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn), done: make(chan struct{})}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.closeOne.Do(func() { close(l.done) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

// dial returns the client end of a fresh pipe, handing the server the other.
func (l *pipeListener) dial(t *testing.T) net.Conn {
	t.Helper()
	client, server := net.Pipe()
	select {
	case l.conns <- server:
	case <-time.After(5 * time.Second):
		t.Fatal("the server never accepted the pipe connection")
	}
	t.Cleanup(func() { client.Close() })
	return client
}

// stallHarness serves the listener's real per-agent chain over pipes.
type stallHarness struct {
	ln     *pipeListener
	token  string
	agent  string
	closed chan struct{}
}

func newStallHarness(t *testing.T, agentID string, domains map[string]bool) *stallHarness {
	t.Helper()
	surfaceUpForTest(t)

	r := newRegistry()
	token := agentID + "-token"
	r.addToken(agentID, agentID, token)
	if domains != nil {
		r.setDomains(agentID, domainList(domains))
	}
	t.Cleanup(func() { invalidateAgentServer(agentID) })

	h := &stallHarness{ln: newPipeListener(), token: token, agent: agentID, closed: make(chan struct{})}
	var once sync.Once
	srv := &http.Server{
		Handler: agentHandler(r),
		ConnState: func(_ net.Conn, s http.ConnState) {
			if s == http.StateClosed {
				once.Do(func() { close(h.closed) })
			}
		},
	}
	go srv.Serve(h.ln)
	t.Cleanup(func() { h.ln.Close(); srv.Close() })
	return h
}

func domainList(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for d, ok := range m {
		if ok {
			out = append(out, d)
		}
	}
	return out
}

// openListen opens a subscriptions/listen stream and reads until the server's
// acknowledgement, which is the only signal that the subscription is actually
// registered. Stalling before it would prove nothing, because the 2026-07-28
// subscribe is asynchronous. After it returns, the caller simply stops reading.
func openListen(t *testing.T, h *stallHarness, uri string) net.Conn {
	t.Helper()
	c := h.ln.dial(t)

	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"subscriptions/listen","params":`+
		`{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",`+
		`"io.modelcontextprotocol/clientCapabilities":{}},`+
		`"notifications":{"resourceSubscriptions":[%q]}}}`, uri)
	req := "POST / HTTP/1.1\r\nHost: mcp\r\n" +
		"Authorization: Bearer " + h.token + "\r\n" +
		"Content-Type: application/json\r\n" +
		"Accept: application/json, text/event-stream\r\n" +
		"Mcp-Protocol-Version: 2026-07-28\r\n" +
		"Mcp-Method: subscriptions/listen\r\n" +
		fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body)) + body

	if err := c.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte(req)); err != nil {
		t.Fatalf("writing the listen request: %v", err)
	}

	br := bufio.NewReader(c)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading the listen response: %v", err)
		}
		if strings.Contains(line, "subscriptions/acknowledged") {
			break
		}
		if strings.HasPrefix(line, "HTTP/1.1") && !strings.Contains(line, "200") {
			rest := make([]byte, 2048)
			n, _ := br.Read(rest)
			t.Fatalf("listen was refused: %s // %s", strings.TrimSpace(line), rest[:n])
		}
	}
	// Clear the deadline so the stall is the peer's own doing, then stop reading.
	if err := c.SetDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	return c
}

func shortWriteTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := listenWriteTimeout
	listenWriteTimeout = d
	t.Cleanup(func() { listenWriteTimeout = prev })
}

// The finding: an agent with only the default domain opens a listen stream,
// stops reading, and the shared feed goroutine that walks every agent's server
// parks in the socket write forever. The write deadline is what reaps it.
func TestStalledListenStreamIsReaped(t *testing.T) {
	shortWriteTimeout(t, 150*time.Millisecond)
	h := newStallHarness(t, "stall-agent", map[string]bool{"node": true})
	openListen(t, h, resNodeSync)

	srv := serverFor(t, h.agent)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Twice: a wedged stream would block the first, and feedNodeSync really
		// does notify twice in a row.
		for i := 0; i < 2; i++ {
			_ = srv.ResourceUpdated(context.Background(),
				&mcp.ResourceUpdatedNotificationParams{URI: resNodeSync})
		}
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("the notification is still parked in the stalled stream's write; " +
			"every other agent's notifications are stuck behind it")
	}

	select {
	case <-h.closed:
	case <-time.After(15 * time.Second):
		t.Fatal("the stalled connection was never torn down")
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		surface.mu.Lock()
		n := len(surface.listens)
		surface.mu.Unlock()
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d listen stream(s) still registered after the reap", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A client opens one listen stream per resource it subscribes to, so the cap has
// to sit above what a legitimate agent needs while still bounding what one token
// can park.
func TestListenCapIsPerAgent(t *testing.T) {
	surfaceUpForTest(t)

	var ids []uint64
	for i := 0; i < maxListensPerAgent; i++ {
		id, ok := surface.addListen("cap-a", func() {})
		if !ok {
			t.Fatalf("agent a was refused its %d listen, below the cap of %d", i+1, maxListensPerAgent)
		}
		ids = append(ids, id)
	}
	if _, ok := surface.addListen("cap-a", func() {}); ok {
		t.Error("agent a was allowed past its cap")
	}
	if _, ok := surface.addListen("cap-b", func() {}); !ok {
		t.Error("agent b was refused because another agent was at its cap")
	}

	surface.removeListen(ids[0])
	if _, ok := surface.addListen("cap-a", func() {}); !ok {
		t.Error("agent a was still refused after one of its streams closed")
	}
}

// recordSpend runs after the transaction is away and before the caller gets its
// id back, so it must not wait on anyone's feed.
func TestRecordSpendDoesNotWaitOnListeners(t *testing.T) {
	useTempAuditFile(t)
	shortWriteTimeout(t, 30*time.Second) // long: only an async notify can beat it
	h := newStallHarness(t, "spend-stall", map[string]bool{"node": true, "audit": true})
	openListen(t, h, resAudit)
	_ = serverFor(t, h.agent)

	done := make(chan struct{})
	go func() {
		defer close(done)
		recordSpend(testAgent(h.agent, "spend", nil), "wallet_send", 0, 1.0, "addr", "ok", "txid")
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("recordSpend is waiting on a stalled audit listener; the transaction is " +
			"already broadcast and the caller cannot get its id back")
	}
}

// serverFor returns the cached scoped server the listener built for this agent,
// which is the one the feeds notify.
func serverFor(t *testing.T, id string) *mcp.Server {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		serversMu.Lock()
		s := servers[id]
		serversMu.Unlock()
		if s != nil {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatal("no scoped server was cached for the agent; the listen never reached one")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
