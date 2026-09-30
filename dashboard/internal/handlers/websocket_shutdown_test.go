// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/decred/slog"
	"github.com/gorilla/websocket"
)

// net/http's Shutdown leaves hijacked connections alone, so an open socket is
// told the server is going away and its handler returns.
func TestShutdownClosesOpenWebSockets(t *testing.T) {
	returned := make(chan struct{})
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, ok := upgradeWS(w, r, slog.Disabled, "test", wsBrowserMsgLimit)
		if !ok {
			return
		}
		defer close(returned)
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	srv.Config.RegisterOnShutdown(CloseWebSockets)
	srv.Start()
	t.Cleanup(srv.Close)

	c, _, err := dialWS(t, srv, "ws"+strings.TrimPrefix(srv.URL, "http"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Config.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	_, _, err = c.ReadMessage()
	var ce *websocket.CloseError
	if !errors.As(err, &ce) || ce.Code != websocket.CloseGoingAway {
		t.Fatalf("socket read %v, want a 1001 going-away close", err)
	}
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("the socket's handler is still running after shutdown")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		n := 0
		openSockets.Range(func(any, any) bool { n++; return true })
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d closed socket(s) still registered", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
