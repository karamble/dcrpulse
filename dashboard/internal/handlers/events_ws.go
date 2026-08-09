// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http"
	"time"

	"dcrpulse/internal/middleware"

	"github.com/decred/slog"
	"github.com/gorilla/websocket"
)

// streamEventsWS is the shared body of the event-streamer endpoints: upgrade,
// replay the retained events, then forward live ones until the client
// disconnects. The subscription is opened only after the replay, matching the
// handlers this replaces. The ping keeps a half-open socket detectable: a
// quiet stream emits nothing for hours, and only traffic makes the browser
// notice the connection died.
func streamEventsWS[T any](w http.ResponseWriter, r *http.Request, log slog.Logger, label string, replay []T, subscribe func() (<-chan T, func())) {
	upgrader := websocket.Upgrader{
		CheckOrigin: middleware.SameOriginWS,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade %s WebSocket: %v", label, err)
		return
	}
	defer conn.Close()

	for _, ev := range replay {
		if err := conn.WriteJSON(ev); err != nil {
			return
		}
	}

	ch, unsubscribe := subscribe()
	defer unsubscribe()

	notify := make(chan struct{})
	go func() {
		defer close(notify)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteJSON(ev); err != nil {
				return
			}
		case <-keepAlive.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-notify:
			return
		}
	}
}
