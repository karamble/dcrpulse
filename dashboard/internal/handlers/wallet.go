// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v4/rpc/walletrpc"

	"github.com/gorilla/websocket"
)

// Global rescan stream management
var (
	activeRescanStream   pb.WalletService_RescanClient
	activeRescanMutex    sync.RWMutex
	rescanStreamChannels []chan *pb.RescanResponse
	rescanChannelsMutex  sync.Mutex
)

// startRescanViaGrpc initiates a blockchain rescan using gRPC and broadcasts progress
func startRescanViaGrpc(beginHeight int32) {
	if rpc.WalletGrpcClient == nil {
		log.Println("❌ Cannot start gRPC rescan: gRPC client not initialized")
		return
	}

	ctx := context.Background()
	req := &pb.RescanRequest{
		BeginHeight: beginHeight,
	}

	stream, err := rpc.WalletGrpcClient.Rescan(ctx, req)
	if err != nil {
		log.Printf("❌ Failed to start gRPC rescan: %v", err)
		return
	}

	log.Println("✅ gRPC rescan stream started - broadcasting progress updates")

	// Store active stream
	activeRescanMutex.Lock()
	activeRescanStream = stream
	activeRescanMutex.Unlock()

	// Receive and broadcast progress updates
	for {
		update, err := stream.Recv()
		if err == io.EOF {
			log.Println("✅ gRPC rescan stream completed")
			break
		}
		if err != nil {
			log.Printf("❌ gRPC rescan stream error: %v", err)
			break
		}

		// Broadcast to all listening WebSocket clients
		rescanChannelsMutex.Lock()
		for _, ch := range rescanStreamChannels {
			select {
			case ch <- update:
			default:
				// Channel full, skip
			}
		}
		rescanChannelsMutex.Unlock()

		log.Printf("📊 Rescan progress: block %d", update.RescannedThrough)
	}

	// Clear active stream
	activeRescanMutex.Lock()
	activeRescanStream = nil
	activeRescanMutex.Unlock()

	// Notify all listeners that stream ended
	rescanChannelsMutex.Lock()
	for _, ch := range rescanStreamChannels {
		close(ch)
	}
	rescanStreamChannels = nil
	rescanChannelsMutex.Unlock()

	log.Println("✅ Rescan completed - all transactions imported")
}

// subscribeToRescanUpdates creates a channel that receives rescan progress updates
func subscribeToRescanUpdates() chan *pb.RescanResponse {
	ch := make(chan *pb.RescanResponse, 10)
	rescanChannelsMutex.Lock()
	rescanStreamChannels = append(rescanStreamChannels, ch)
	rescanChannelsMutex.Unlock()
	return ch
}

// unsubscribeFromRescanUpdates removes a channel from receiving updates
func unsubscribeFromRescanUpdates(ch chan *pb.RescanResponse) {
	rescanChannelsMutex.Lock()
	defer rescanChannelsMutex.Unlock()

	for i, c := range rescanStreamChannels {
		if c == ch {
			rescanStreamChannels = append(rescanStreamChannels[:i], rescanStreamChannels[i+1:]...)
			break
		}
	}
}

// Pending rescan tracking is now in services package

// GetWalletStatusHandler handles requests for wallet status
func GetWalletStatusHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletClient == nil {
		http.Error(w, "Wallet RPC client not initialized", http.StatusServiceUnavailable)
		return
	}

	// Check if dcrd is still syncing before attempting wallet operations
	if rpc.DcrdClient != nil {
		checkCtx, checkCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer checkCancel()

		chainInfo, err := rpc.DcrdClient.GetBlockChainInfo(checkCtx)
		if err == nil && chainInfo.InitialBlockDownload {
			// dcrd is still syncing - return a user-friendly message
			syncProgress := float64(0)
			if chainInfo.SyncHeight > 0 {
				if chainInfo.Headers > 0 {
					syncProgress = (float64(chainInfo.Headers) / float64(chainInfo.SyncHeight)) * 100
				} else if chainInfo.Blocks > 0 {
					syncProgress = (float64(chainInfo.Blocks) / float64(chainInfo.SyncHeight)) * 100
				}
			}

			errorMsg := fmt.Sprintf("Blockchain is syncing (%.1f%% complete). Wallet will be available once sync is complete.", syncProgress)
			http.Error(w, errorMsg, http.StatusServiceUnavailable)
			return
		}
	}

	status, err := services.FetchWalletStatus()
	if err != nil {
		log.Printf("Error fetching wallet status: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// GetWalletDashboardHandler handles requests for complete wallet dashboard data
func GetWalletDashboardHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletClient == nil {
		http.Error(w, "Wallet RPC client not initialized", http.StatusServiceUnavailable)
		return
	}

	// Check if dcrd is still syncing before attempting wallet operations
	if rpc.DcrdClient != nil {
		checkCtx, checkCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer checkCancel()

		chainInfo, err := rpc.DcrdClient.GetBlockChainInfo(checkCtx)
		if err == nil && chainInfo.InitialBlockDownload {
			// dcrd is still syncing - return a user-friendly message
			syncProgress := float64(0)
			if chainInfo.SyncHeight > 0 {
				if chainInfo.Headers > 0 {
					syncProgress = (float64(chainInfo.Headers) / float64(chainInfo.SyncHeight)) * 100
				} else if chainInfo.Blocks > 0 {
					syncProgress = (float64(chainInfo.Blocks) / float64(chainInfo.SyncHeight)) * 100
				}
			}

			errorMsg := fmt.Sprintf("Blockchain is syncing (%.1f%% complete). Wallet will be available once sync is complete.", syncProgress)
			http.Error(w, errorMsg, http.StatusServiceUnavailable)
			return
		}
	}

	// Create a context with timeout to prevent hanging on slow RPC calls
	// Use 20 seconds to accommodate wallet rescans which can slow down RPC responses
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	// Use a channel to handle the async fetch with timeout
	type result struct {
		data *types.WalletDashboardData
		err  error
	}
	resultChan := make(chan result, 1)

	go func() {
		data, err := services.FetchWalletDashboardDataWithContext(ctx)
		resultChan <- result{data, err}
	}()

	select {
	case res := <-resultChan:
		if res.err != nil {
			log.Printf("Error fetching wallet dashboard data: %v", res.err)
			// Return partial data if available
			if res.data != nil {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(res.data)
				return
			}
			http.Error(w, res.err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res.data)
	case <-ctx.Done():
		log.Printf("Wallet dashboard request timed out")
		http.Error(w, "Wallet dashboard request timed out - wallet may be rescanning", http.StatusRequestTimeout)
	}
}

// ImportXpubHandler handles xpub import requests
func ImportXpubHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletClient == nil {
		http.Error(w, "Wallet RPC client not initialized", http.StatusServiceUnavailable)
		return
	}

	var req types.ImportXpubRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate xpub format (Decred mainnet xpubs start with "dpub")
	if !strings.HasPrefix(req.Xpub, "dpub") && !strings.HasPrefix(req.Xpub, "tpub") {
		response := types.ImportXpubResponse{
			Success: false,
			Message: "Invalid xpub format. Decred mainnet xpubs must start with 'dpub'",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Set default account name if not provided
	accountName := req.AccountName
	if accountName == "" {
		accountName = "imported"
	}

	// Import the xpub asynchronously
	// We run it in a goroutine and return immediately so the frontend doesn't timeout
	log.Printf("Starting xpub import for account: %s", accountName)

	// Start the import in a goroutine
	// WebSocket stream will automatically detect and show rescan progress
	go func() {
		ctx := context.Background()

		// Step 1: Import xpub
		params := []json.RawMessage{
			json.RawMessage(fmt.Sprintf(`"%s"`, accountName)),
			json.RawMessage(fmt.Sprintf(`"%s"`, req.Xpub)),
		}

		log.Printf("Step 1/3: Importing xpub for account '%s'", accountName)
		result, err := rpc.WalletClient.RawRequest(ctx, "importxpub", params)
		if err != nil {
			log.Printf("Failed to import xpub: %v", err)
			return
		}
		log.Printf("Xpub import completed: %v", string(result))

		// Step 2: Discover address usage
		log.Printf("Step 2/3: Discovering address usage across blockchain...")
		_, err = rpc.WalletClient.RawRequest(ctx, "discoverusage", nil)
		if err != nil {
			log.Printf("Failed to discover address usage: %v", err)
			return
		}
		log.Printf("Address discovery completed - wallet database updated")

		// Step 3: Wait for wallet to be ready, then rescan from block 0 via gRPC
		log.Printf("Step 3/3: Waiting 5 seconds for wallet to load transaction filter...")
		time.Sleep(5 * time.Second)

		// Start gRPC rescan from genesis
		log.Printf("Starting gRPC rescan from block 0...")
		startRescanViaGrpc(0)
	}()

	// Return immediately - the frontend will poll wallet status to track rescan progress
	response := types.ImportXpubResponse{
		Success: true,
		Message: fmt.Sprintf("Xpub import started for account '%s'. Now discovering addresses and rescanning blockchain. This typically takes 5-30 minutes.", accountName),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// RescanWalletHandler handles wallet rescan requests
func RescanWalletHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletClient == nil {
		http.Error(w, "Wallet RPC client not initialized", http.StatusServiceUnavailable)
		return
	}

	var req types.RescanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Default to full rescan from genesis
		req.BeginHeight = 0
	}

	// Start rescan in a goroutine - it's a long-running operation
	// The gRPC Rescan() method will stream progress updates that the WebSocket handler can forward
	log.Printf("Starting wallet rescan from block %d via gRPC", req.BeginHeight)

	go func() {
		ctx := context.Background()

		// Step 1: Discover address usage via JSON-RPC
		log.Printf("Step 1/2: Discovering address usage across blockchain for all accounts...")
		_, err := rpc.WalletClient.RawRequest(ctx, "discoverusage", nil)
		if err != nil {
			log.Printf("Failed to discover address usage: %v", err)
			return
		}
		log.Printf("Address discovery completed - wallet database updated")

		// Step 2: Wait for wallet to load transaction filter
		log.Printf("Step 2/2: Waiting 5 seconds for wallet to load transaction filter...")
		time.Sleep(5 * time.Second)

		// Step 3: Start rescan via gRPC - this provides a progress stream
		log.Printf("Starting gRPC rescan from block %d...", req.BeginHeight)
		startRescanViaGrpc(int32(req.BeginHeight))
	}()

	// Return immediately so frontend can start polling for progress
	response := types.RescanResponse{
		Success: true,
		Message: fmt.Sprintf("Discovering addresses and rescanning blockchain from block %d. This may take 30+ minutes.", req.BeginHeight),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetSyncProgressHandler handles requests for wallet sync progress from log files
func GetSyncProgressHandler(w http.ResponseWriter, r *http.Request) {
	// Get sync progress from log file parsing
	isRescanning, scanHeight, err := services.ParseWalletLogsForRescan()
	if err != nil {
		log.Printf("Error parsing wallet logs: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get chain height from dcrd for progress calculation
	var progress float64 = 100.0
	var chainHeight int64 = 0
	var message string = "No active rescan"

	if isRescanning && rpc.DcrdClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		height, err := rpc.DcrdClient.GetBlockCount(ctx)
		if err == nil {
			chainHeight = height
			progress = (float64(scanHeight) / float64(chainHeight)) * 100
			message = fmt.Sprintf("Rescanning... %d/%d blocks", scanHeight, chainHeight)
		}
	}

	response := types.SyncProgressResponse{
		IsRescanning: isRescanning,
		ScanHeight:   scanHeight,
		ChainHeight:  chainHeight,
		Progress:     progress,
		Message:      message,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListTransactionsHandler handles requests for wallet transaction history
func ListTransactionsHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletClient == nil {
		http.Error(w, "Wallet RPC client not initialized", http.StatusServiceUnavailable)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	count := 50 // default
	from := 0   // default

	if c := query.Get("count"); c != "" {
		if parsed, err := fmt.Sscanf(c, "%d", &count); err == nil && parsed == 1 {
			// count parsed successfully
		}
	}
	if f := query.Get("from"); f != "" {
		if parsed, err := fmt.Sscanf(f, "%d", &from); err == nil && parsed == 1 {
			// from parsed successfully
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Fetch transactions
	transactions, err := services.ListTransactions(ctx, count, from)
	if err != nil {
		log.Printf("Error listing transactions: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(transactions)
}

// StreamRescanProgressHandler streams rescan progress via WebSocket using gRPC
func StreamRescanProgressHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil {
		http.Error(w, "Wallet gRPC client not initialized", http.StatusServiceUnavailable)
		return
	}

	// Upgrade HTTP connection to WebSocket
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins (configure appropriately for production)
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	log.Println("WebSocket connection established for rescan progress streaming")

	// Get chain height from dcrd for progress calculation
	chainHeight := int64(1)
	if rpc.DcrdClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		height, err := rpc.DcrdClient.GetBlockCount(ctx)
		cancel()
		if err == nil {
			chainHeight = height
		}
	}

	log.Println("Starting rescan progress monitoring (polling log-based method)")

	// Poll rescan progress and stream to WebSocket
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Track consecutive "not rescanning" responses to avoid premature close
	notRescanningCount := 0

	// Check if a rescan is pending
	isPending, pendingGracePeriod := services.IsPendingRescan()
	gracePeriodTicks := 8            // Default grace period
	maxNotRescanningBeforeClose := 5 // Default: Wait for 5 consecutive "not rescanning" before closing

	if isPending {
		gracePeriodTicks = pendingGracePeriod
		maxNotRescanningBeforeClose = 30 // Wait 30 more seconds after grace period when rescan is pending
		log.Printf("Pending rescan detected - using extended grace period of %d seconds and extended timeout of %d checks", gracePeriodTicks, maxNotRescanningBeforeClose)
	}

	tickCount := 0

	for {
		select {
		case <-ticker.C:
			tickCount++
			// Check rescan progress
			isRescanning, scanHeight, err := services.ParseWalletLogsForRescan()
			if err != nil {
				log.Printf("Error parsing logs: %v", err)
				continue
			}

			// Update chain height
			if rpc.DcrdClient != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				height, err := rpc.DcrdClient.GetBlockCount(ctx)
				cancel()
				if err == nil {
					chainHeight = height
				}
			}

			progress := (float64(scanHeight) / float64(chainHeight)) * 100
			if progress > 100 {
				progress = 100
			}

			progressData := map[string]interface{}{
				"isRescanning": isRescanning,
				"scanHeight":   scanHeight,
				"chainHeight":  chainHeight,
				"progress":     progress,
			}

			if isRescanning {
				progressData["message"] = fmt.Sprintf("Rescanning... %d/%d blocks", scanHeight, chainHeight)
				log.Printf("Rescan progress: %d/%d (%.2f%%)", scanHeight, chainHeight, progress)
				notRescanningCount = 0 // Reset counter
				// Pending rescan flag is cleared automatically in CheckRescanProgress
			} else {
				// Only start counting "not rescanning" after grace period
				if tickCount > gracePeriodTicks {
					notRescanningCount++
					log.Printf("Rescan not detected in logs (count: %d/%d, after grace period)", notRescanningCount, maxNotRescanningBeforeClose)
					progressData["message"] = "Checking rescan status..."
				} else {
					log.Printf("Grace period: %d/%d seconds - waiting for rescan to start", tickCount, gracePeriodTicks)
					if isPending {
						progressData["message"] = "Discovering addresses, rescan will start soon..."
					} else {
						progressData["message"] = "Starting rescan..."
					}
				}
			}

			// Send update to client
			if err := conn.WriteJSON(progressData); err != nil {
				log.Printf("Failed to write to WebSocket: %v", err)
				return
			}

			// Only close if we've had multiple consecutive "not rescanning" responses AFTER grace period
			if notRescanningCount >= maxNotRescanningBeforeClose {
				log.Println("Rescan complete (no activity detected), closing WebSocket stream")
				progressData["message"] = "Rescan complete"
				progressData["isRescanning"] = false
				conn.WriteJSON(progressData)
				services.ClearPendingRescan() // Clear pending flag when closing
				return
			}
		}
	}

	log.Println("WebSocket connection closed")
}
