// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"dcrpulse/internal/rpc"

	"github.com/gorilla/websocket"
)

// StreamRescanGrpcHandler streams rescan progress via WebSocket
// by subscribing to the gRPC rescan progress broadcast
func StreamRescanGrpcHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for development
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	log.Println("🔌 WebSocket: Client connected for rescan progress")

	// Subscribe to rescan progress updates
	progressCh := subscribeToRescanUpdates()
	defer unsubscribeFromRescanUpdates(progressCh)

	// Get chain height for progress calculation
	getChainHeight := func() int64 {
		if rpc.DcrdClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			height, err := rpc.DcrdClient.GetBlockCount(ctx)
			if err == nil {
				return height
			}
		}
		return 0
	}

	chainHeight := getChainHeight()

	// Keep-alive ticker
	keepAliveTicker := time.NewTicker(5 * time.Second)
	defer keepAliveTicker.Stop()

	// Initial check - send current status immediately
	activeRescanMutex.RLock()
	hasActiveRescan := activeRescanStream != nil
	activeRescanMutex.RUnlock()

	if !hasActiveRescan {
		// No active rescan - send "synced" status and keep connection open
		conn.WriteJSON(map[string]interface{}{
			"isRescanning": false,
			"message":      "Wallet fully synced",
			"progress":     100.0,
			"scanHeight":   chainHeight,
			"chainHeight":  chainHeight,
		})
	}

	log.Println("📡 Waiting for rescan progress updates...")

	for {
		select {
		case update, ok := <-progressCh:
			if !ok {
				// Channel closed - rescan finished
				log.Println("✅ Rescan complete - sending final sync status")
				conn.WriteJSON(map[string]interface{}{
					"isRescanning": false,
					"message":      "Wallet fully synced",
					"progress":     100.0,
					"scanHeight":   chainHeight,
					"chainHeight":  chainHeight,
				})
				return
			}

			// Update chain height periodically
			chainHeight = getChainHeight()

			// Calculate progress
			rescannedHeight := int64(update.RescannedThrough)
			progress := 0.0
			if chainHeight > 0 {
				progress = (float64(rescannedHeight) / float64(chainHeight)) * 100
				if progress > 100 {
					progress = 100
				}
			}

			message := fmt.Sprintf("Rescanning blockchain... %d/%d blocks", rescannedHeight, chainHeight)

			// Forward to WebSocket client
			progressData := map[string]interface{}{
				"isRescanning": true,
				"scanHeight":   rescannedHeight,
				"chainHeight":  chainHeight,
				"progress":     progress,
				"message":      message,
			}

			log.Printf("📊 Rescan progress: %d/%d (%.1f%%)", rescannedHeight, chainHeight, progress)

			if err := conn.WriteJSON(progressData); err != nil {
				log.Printf("❌ WebSocket write failed: %v", err)
				return
			}

		case <-keepAliveTicker.C:
			// Send ping to detect if client disconnected
			if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				log.Println("🔌 WebSocket client disconnected")
				return
			}
		}
	}
}
