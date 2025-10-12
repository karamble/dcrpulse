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
	"dcrpulse/internal/services"

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

	// Wallet sync status ticker - ONLY check when no active gRPC rescan
	syncStatusTicker := time.NewTicker(3 * time.Second)
	defer syncStatusTicker.Stop()

	// Track if we have an active gRPC rescan
	hasActiveGrpcRescan := false

	// Helper to check wallet sync status (for initial sync, not user-triggered rescans)
	checkWalletSync := func() map[string]interface{} {
		if rpc.WalletClient == nil {
			return map[string]interface{}{
				"isRescanning": false,
				"message":      "Wallet not connected",
				"progress":     0.0,
				"scanHeight":   0,
				"chainHeight":  0,
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Get wallet height
		_, walletHeight, err := rpc.WalletClient.GetBestBlock(ctx)
		if err != nil {
			return nil // Skip this update on error
		}

		// Get chain height
		chainHeight := getChainHeight()
		if chainHeight == 0 {
			return nil // Skip if we can't get chain height
		}

		// Calculate blocks behind
		blocksBehind := chainHeight - walletHeight

		// During initial sync, wallet is typically 100+ blocks behind
		// Use CheckRescanProgress to get accurate state
		isRescanning, _, checkErr := services.CheckRescanProgress()
		if checkErr == nil && isRescanning && blocksBehind > 10 {
			progress := (float64(walletHeight) / float64(chainHeight)) * 100
			if progress > 100 {
				progress = 100
			}

			return map[string]interface{}{
				"isRescanning": true,
				"scanHeight":   walletHeight,
				"chainHeight":  chainHeight,
				"progress":     progress,
				"message":      fmt.Sprintf("Syncing wallet... %d/%d blocks", walletHeight, chainHeight),
			}
		}

		// Wallet is synced
		return map[string]interface{}{
			"isRescanning": false,
			"message":      "Wallet fully synced",
			"progress":     100.0,
			"scanHeight":   walletHeight,
			"chainHeight":  chainHeight,
		}
	}

	// Initial check - send current status immediately
	initialStatus := checkWalletSync()
	if initialStatus != nil {
		conn.WriteJSON(initialStatus)
	}

	log.Println("📡 Monitoring wallet sync and rescan progress (gRPC + RPC)")

	for {
		select {
		case update, ok := <-progressCh:
			// Priority 1: gRPC rescan updates (user-triggered rescans)
			if !ok {
				// Channel closed - rescan finished
				log.Println("✅ gRPC Rescan complete")
				hasActiveGrpcRescan = false
				// Check if wallet sync is still ongoing
				status := checkWalletSync()
				if status != nil {
					conn.WriteJSON(status)
				}
				continue
			}

			// Active gRPC rescan - this takes priority
			hasActiveGrpcRescan = true

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

			log.Printf("📊 gRPC Rescan progress: %d/%d (%.1f%%)", rescannedHeight, chainHeight, progress)

			if err := conn.WriteJSON(progressData); err != nil {
				log.Printf("❌ WebSocket write failed: %v", err)
				return
			}

		case <-syncStatusTicker.C:
			// Priority 2: Check wallet sync ONLY if no active gRPC rescan
			if hasActiveGrpcRescan {
				continue // Skip - gRPC rescan is active
			}

			status := checkWalletSync()
			if status != nil {
				if err := conn.WriteJSON(status); err != nil {
					log.Printf("❌ WebSocket write failed: %v", err)
					return
				}

				// Log progress if syncing
				if isSyncing, ok := status["isRescanning"].(bool); ok && isSyncing {
					scanHeight, _ := status["scanHeight"].(int64)
					chainHeight, _ := status["chainHeight"].(int64)
					progress, _ := status["progress"].(float64)
					log.Printf("📊 Wallet sync progress: %d/%d (%.1f%%)", scanHeight, chainHeight, progress)
				}
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
