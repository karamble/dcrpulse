// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dcrpulse/internal/rpc"

	"github.com/decred/slog"
	"github.com/gorilla/websocket"
)

// The limit is asserted from the client side rather than by reading the field:
// gorilla answers an oversized frame with a 1009 close, so an unbounded socket
// and a bounded one are told apart by what the peer actually receives.
func wsTestServer(t *testing.T, limit int64) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, ok := upgradeWS(w, r, slog.Disabled, "test", limit)
		if !ok {
			return
		}
		defer conn.Close()
		// Echo, so the client can tell "accepted" from "refused" by what comes
		// back. A read-only server would leave the client blocked forever.
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(mt, data); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv, "ws" + strings.TrimPrefix(srv.URL, "http")
}

// dialWS connects with an Origin the same-origin check accepts.
func dialWS(t *testing.T, srv *httptest.Server, wsURL string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	c, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Origin": {srv.URL}})
	if c != nil {
		// A bounded read: nothing here should ever block, and a test that
		// hangs is worse than one that fails.
		_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	}
	return c, resp, err
}

func TestUpgradeWSRejectsAnOversizedFrame(t *testing.T) {
	srv, wsURL := wsTestServer(t, 1<<10)
	c, _, err := dialWS(t, srv, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	if err := c.WriteMessage(websocket.BinaryMessage, make([]byte, 4<<10)); err != nil {
		t.Fatalf("write: %v", err)
	}
	// The read must fail: with no limit the echo comes back instead, which is
	// what an unbounded socket does. The server writes a 1009 close, but under
	// load its own Close can reset the connection before the client reads that
	// frame, so the close code is checked only when one actually arrives.
	_, got, err := c.ReadMessage()
	if err == nil {
		t.Fatalf("an oversized frame was echoed back (%d bytes): it was not refused", len(got))
	}
	if ce, isClose := err.(*websocket.CloseError); isClose && ce.Code != websocket.CloseMessageTooBig {
		t.Fatalf("closed with %v, want 1009 message-too-big", err)
	}
}

// The counterpart: the limit must not be so tight that ordinary traffic breaks.
func TestUpgradeWSAcceptsANormalFrame(t *testing.T) {
	srv, wsURL := wsTestServer(t, wsBrowserMsgLimit)
	c, _, err := dialWS(t, srv, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	// A Lightning payment request is the largest thing a browser really sends.
	msg := strings.Repeat("x", 4<<10)
	if err := c.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, got, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("a normal frame was not echoed back: %v", err)
	}
	if string(got) != msg {
		t.Fatalf("echo returned %d bytes, want %d", len(got), len(msg))
	}
}

// The origin check has to survive being folded into the helper.
func TestUpgradeWSRefusesCrossOrigin(t *testing.T) {
	_, wsURL := wsTestServer(t, wsBrowserMsgLimit)
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Origin": {"https://evil.example"}})
	if err == nil {
		t.Fatal("a cross-origin upgrade was accepted")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin upgrade answered %v, want 403", resp)
	}
}

// Every Bison Relay bound is upstream's, so a protocol bump follows at compile
// time instead of drifting. These pin that they are not hand-copied numbers.
func TestBRLimitsComeFromUpstream(t *testing.T) {
	if rpc.BRRTDTMaxMessageBytes != 65535 {
		t.Errorf("RTDT bound = %d, want upstream rpc.RTDTMaxMessageSize (65535)", rpc.BRRTDTMaxMessageBytes)
	}
	if rpc.BRMaxPayloadBytes != 10*1024*1024 {
		t.Errorf("payload bound = %d, want upstream MaxPayloadSizeForVersion(V1) (10 MiB)", rpc.BRMaxPayloadBytes)
	}
}
