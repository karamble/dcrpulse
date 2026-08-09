// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"

	"github.com/gorilla/websocket"
)

// GetDashboardDataHandler handles requests for complete dashboard data
func GetDashboardDataHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.DcrdClient == nil {
		// Usually a first boot where dcrd has not written its RPC cert yet, so
		// give the startup explanation rather than a configuration error.
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		http.Error(w, services.DaemonStartupHint(ctx, services.LogComponentDcrd).Message,
			http.StatusServiceUnavailable)
		return
	}

	data, err := services.FetchDashboardData()
	if err != nil {
		nodeLog.Errorf("Error fetching dashboard data: %v", err)
		respondDaemonError(w, r, services.LogComponentDcrd, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// StreamNodeSyncHandler streams dcrd sync-progress snapshots over a WebSocket,
// pushed on each block-connected notification instead of the 30s poll. Mirrors
// the wallet's StreamRescanProgressHandler.
func StreamNodeSyncHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: middleware.SameOriginWS}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		nodeLog.Errorf("Failed to upgrade node-sync WebSocket: %v", err)
		return
	}
	defer conn.Close()

	if err := conn.WriteJSON(services.GetNodeSyncSnapshot()); err != nil {
		return
	}

	ch, unsubscribe := services.SubscribeNodeSyncEvents()
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
			if err := conn.WriteJSON(snap); err != nil {
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

// HealthCheckHandler handles health check requests
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":             "healthy",
		"rpcConnected":       rpc.DcrdClient != nil,
		"walletRPCConnected": rpc.WalletClient != nil,
		"dcrdTLS":            rpc.DcrdUsesTLS(),
		"walletTLS":          rpc.WalletUsesTLS(),
		"time":               time.Now(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
