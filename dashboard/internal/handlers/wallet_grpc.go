// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"log"
	"net/http"
	"time"

	"dcrpulse/internal/services"

	"github.com/gorilla/websocket"
)

// StreamRescanGrpcHandler streams the SyncSnapshot to WebSocket clients.
// On connect: pushes the current snapshot immediately. Then forwards every
// snapshot update as the RpcSync supervisor + user-initiated rescans feed
// the snapshot. Replaces the previous heuristic polling + log-parsing path.
func StreamRescanGrpcHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	log.Println("🔌 WebSocket: Client connected for sync state stream")

	if err := conn.WriteJSON(snapshotPayload(services.GetSyncSnapshot())); err != nil {
		return
	}

	ch, unsubscribe := services.SubscribeSyncEvents()
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
		case snap, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteJSON(snapshotPayload(snap)); err != nil {
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
