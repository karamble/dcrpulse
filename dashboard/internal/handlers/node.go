// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
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

// ConnectRPCHandler handles RPC connection requests
func ConnectRPCHandler(w http.ResponseWriter, r *http.Request) {
	var req types.RPCConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	config := rpc.Config{
		RPCHost:     req.Host,
		RPCPort:     req.Port,
		RPCUser:     req.Username,
		RPCPassword: req.Password,
	}

	err := rpc.InitDcrdClient(config)
	response := types.RPCConnectionResponse{
		Success: err == nil,
		Message: "Connected successfully",
	}

	if err != nil {
		response.Message = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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
