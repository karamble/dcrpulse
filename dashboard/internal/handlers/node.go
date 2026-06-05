// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"log"
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
		http.Error(w, "RPC client not initialized", http.StatusServiceUnavailable)
		return
	}

	data, err := services.FetchDashboardData()
	if err != nil {
		log.Printf("Error fetching dashboard data: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// GetNodeStatusHandler handles requests for node status
func GetNodeStatusHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.DcrdClient == nil {
		http.Error(w, "RPC client not initialized", http.StatusServiceUnavailable)
		return
	}

	status, err := services.FetchNodeStatus()
	if err != nil {
		log.Printf("Error fetching node status: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// StreamNodeSyncHandler streams dcrd sync-progress snapshots over a WebSocket,
// pushed on each block-connected notification instead of the 30s poll. Mirrors
// the wallet's StreamRescanProgressHandler.
func StreamNodeSyncHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: middleware.SameOriginWS}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade node-sync WebSocket: %v", err)
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

// GetBlockchainInfoHandler handles requests for blockchain information
func GetBlockchainInfoHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.DcrdClient == nil {
		http.Error(w, "RPC client not initialized", http.StatusServiceUnavailable)
		return
	}

	info, err := services.FetchBlockchainInfo()
	if err != nil {
		log.Printf("Error fetching blockchain info: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// GetPeersHandler handles requests for peer information
func GetPeersHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.DcrdClient == nil {
		http.Error(w, "RPC client not initialized", http.StatusServiceUnavailable)
		return
	}

	peers, err := services.FetchPeers()
	if err != nil {
		log.Printf("Error fetching peers: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

// HealthCheckHandler handles health check requests
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":             "healthy",
		"rpcConnected":       rpc.DcrdClient != nil,
		"walletRPCConnected": rpc.WalletClient != nil,
		"time":               time.Now(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
