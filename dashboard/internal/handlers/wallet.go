// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"

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

		// Update the unified SyncSnapshot so all subscribers (incl. the
		// WebSocket fan-out below) see consistent rescan state.
		services.MarkRescanProgress(update.RescannedThrough)

		// Broadcast to all listening WebSocket clients (legacy channel —
		// SyncSnapshot subscribers get the same data via the new path).
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

	services.MarkRescanFinished()
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

func GetSyncProgressHandler(w http.ResponseWriter, r *http.Request) {
	snap := services.GetSyncSnapshot()
	payload := snapshotPayload(snap)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
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

func StreamRescanProgressHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: middleware.SameOriginWS,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	// Initial snapshot.
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

// snapshotPayload renders a SyncSnapshot as the WebSocket / sync-progress JSON.
func snapshotPayload(snap services.SyncSnapshot) map[string]interface{} {
	isRescanning := snap.Phase == services.SyncPhaseFetchingCfilters ||
		snap.Phase == services.SyncPhaseFetchingHeaders ||
		snap.Phase == services.SyncPhaseDiscoverAddresses ||
		snap.Phase == services.SyncPhaseRescanning

	scanHeight, chainHeight := phaseProgress(snap)
	progress := snap.RescanProgressPc
	switch snap.Phase {
	case services.SyncPhaseRescanning:
		// progress = snap.RescanProgressPc (already assigned)
	case services.SyncPhaseFetchingHeaders:
		// Time-based ratio, mirroring Decrediton's
		// AnimatedLinearProgressFull: how far through the blockchain's
		// timestamp range we have synced.
		if snap.FirstHeaderTime > 0 && snap.LastHeaderTime > snap.FirstHeaderTime {
			now := time.Now().Unix()
			denom := now - snap.FirstHeaderTime
			if denom > 0 {
				p := float64(snap.LastHeaderTime-snap.FirstHeaderTime) / float64(denom) * 100
				if p > 100 {
					p = 100
				}
				progress = p
			}
		} else {
			progress = 0
		}
	default:
		if chainHeight > 0 {
			progress = float64(scanHeight) / float64(chainHeight) * 100
			if progress > 100 {
				progress = 100
			}
		}
	}
	message := ""
	switch snap.Phase {
	case services.SyncPhaseRescanning:
		message = fmt.Sprintf("Rescanning... %d/%d blocks (%.1f%%)", snap.RescanThrough, snap.RescanFrom, snap.RescanProgressPc)
	case services.SyncPhaseFetchingCfilters:
		if snap.CfiltersEnd > snap.CfiltersStart {
			message = fmt.Sprintf("Fetching committed filters (block %d → %d)", snap.CfiltersStart, snap.CfiltersEnd)
		} else {
			message = "Fetching committed filters"
		}
	case services.SyncPhaseFetchingHeaders:
		if snap.HeadersCount > 0 {
			message = fmt.Sprintf("Fetching block headers (%d fetched)", snap.HeadersCount)
		} else {
			message = "Fetching block headers"
		}
	case services.SyncPhaseDiscoverAddresses:
		message = "Discovering addresses"
	case services.SyncPhaseSynced:
		message = "Synced"
	case services.SyncPhaseUnsynced:
		message = "Not yet synced"
	default:
		message = "Sync state unknown"
	}
	if !snap.DaemonConnected {
		message = "Disconnected from dcrd"
	}
	return map[string]interface{}{
		"isRescanning":    isRescanning,
		"scanHeight":      scanHeight,
		"chainHeight":     chainHeight,
		"progress":        progress,
		"message":         message,
		"phase":           string(snap.Phase),
		"daemonConnected": snap.DaemonConnected,
		"peerCount":       snap.PeerCount,
		"cfiltersStart":   snap.CfiltersStart,
		"cfiltersEnd":     snap.CfiltersEnd,
		"headersCount":    snap.HeadersCount,
		"firstHeaderTime": snap.FirstHeaderTime,
		"lastHeaderTime":  snap.LastHeaderTime,
	}
}

// phaseProgress returns (numerator, denominator) for the progress bar in the current sync phase.
func phaseProgress(snap services.SyncSnapshot) (int64, int64) {
	chainTip := int64(0)
	if rpc.DcrdClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if h, err := rpc.DcrdClient.GetBlockCount(ctx); err == nil {
			chainTip = h
		}
		cancel()
	}
	switch snap.Phase {
	case services.SyncPhaseRescanning:
		return int64(snap.RescanThrough), snap.RescanFrom
	case services.SyncPhaseFetchingHeaders:
		// HeadersCount is fetched-this-session; FetchHeadersNotification
		// has no usable height for a block-of-block bar. Time-based
		// progress is computed in snapshotPayload; return (0, 0) so the
		// frontend hides the "Block X of Y" line.
		return 0, 0
	case services.SyncPhaseFetchingCfilters:
		return int64(snap.CfiltersEnd), chainTip
	default:
		return 0, chainTip
	}
}

func GetAccountsHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	accounts, err := services.FetchAllAccounts(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accounts)
}

// importedAccountNumber is dcrwallet's reserved bucket for unencrypted
// private-key imports. It cannot be renamed and is never returned by
// NextAccount.
const importedAccountNumber uint32 = 2147483647

func CreateAccountHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		AccountName string `json:"accountName"`
		Passphrase  string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.AccountName)
	if name == "" {
		http.Error(w, "accountName required", http.StatusBadRequest)
		return
	}
	if len(name) > 50 {
		http.Error(w, "accountName must be 50 characters or fewer", http.StatusBadRequest)
		return
	}
	if strings.EqualFold(name, "imported") {
		http.Error(w, "'imported' is a reserved account name", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	passphrase := []byte(req.Passphrase)
	req.Passphrase = ""

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	num, err := services.CreateAccount(ctx, name, passphrase)
	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		switch {
		case strings.Contains(lower, "passphrase"), strings.Contains(lower, "decrypt"):
			http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
		case strings.Contains(lower, "already"):
			http.Error(w, "An account with that name already exists", http.StatusConflict)
		default:
			log.Printf("CreateAccount failed: %v", err)
			http.Error(w, "create account failed", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]uint32{"accountNumber": num})
}

func RenameAccountHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		AccountNumber uint32 `json:"accountNumber"`
		NewName       string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.NewName)
	if name == "" {
		http.Error(w, "newName required", http.StatusBadRequest)
		return
	}
	if len(name) > 50 {
		http.Error(w, "newName must be 50 characters or fewer", http.StatusBadRequest)
		return
	}
	if strings.EqualFold(name, "imported") {
		http.Error(w, "'imported' is a reserved account name", http.StatusBadRequest)
		return
	}
	if req.AccountNumber == importedAccountNumber {
		http.Error(w, "the imported account cannot be renamed", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := services.RenameAccount(ctx, req.AccountNumber, name); err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		switch {
		case strings.Contains(lower, "already"):
			http.Error(w, "An account with that name already exists", http.StatusConflict)
		case strings.Contains(lower, "not found"):
			http.Error(w, "account not found", http.StatusNotFound)
		default:
			log.Printf("RenameAccount failed: %v", err)
			http.Error(w, "rename failed", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{}"))
}

type privacyStatusResponse struct {
	Configured      bool    `json:"configured"`
	MixedAccount    *uint32 `json:"mixedAccount,omitempty"`
	ChangeAccount   *uint32 `json:"changeAccount,omitempty"`
	MixerRunning    bool    `json:"mixerRunning"`
	LastError       string  `json:"lastError,omitempty"`
	CsppsolverState string  `json:"csppsolverState"` // "active" | "missing" | "unknown"
}

func PrivacyStatusHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	mixed, change, configured, err := services.FindPrivacyAccounts(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	services.RefreshCsppsolverStateIfUnknown()
	resp := privacyStatusResponse{
		Configured:      configured,
		MixerRunning:    services.IsMixerRunning(),
		LastError:       services.LastMixerError(),
		CsppsolverState: services.CsppsolverState(),
	}
	if configured {
		m, c := mixed, change
		resp.MixedAccount = &m
		resp.ChangeAccount = &c
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func PrivacySetupHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	passphrase := []byte(req.Passphrase)
	req.Passphrase = ""

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	mixed, change, err := services.SetupPrivacyAccounts(ctx, passphrase)
	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		switch {
		case strings.Contains(lower, "passphrase"), strings.Contains(lower, "decrypt"):
			http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
		default:
			log.Printf("PrivacySetup failed: %v", err)
			http.Error(w, "privacy setup failed", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]uint32{
		"mixedAccount":  mixed,
		"changeAccount": change,
	})
}

func PrivacyStartHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.AccountMixerClient == nil {
		http.Error(w, "mixer gRPC client not initialized", http.StatusServiceUnavailable)
		return
	}

	if services.IsMixerRunning() {
		http.Error(w, "mixer already running", http.StatusConflict)
		return
	}
	// The mixer, the autobuyer, and a manual ticket purchase all spend the
	// mixed account, so the mixer must not start while any of them is active
	// (the autobuyer mixes its buys inline; a manual purchase pauses and
	// restarts the mixer itself).
	if services.IsAutobuyerRunning() {
		http.Error(w, "stop the ticket autobuyer before starting the mixer", http.StatusConflict)
		return
	}
	if services.IsTicketPurchaseInProgress() {
		http.Error(w, "a ticket purchase is in progress; try again once it finishes", http.StatusConflict)
		return
	}

	var req struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	passphrase := []byte(req.Passphrase)
	req.Passphrase = ""

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	mixed, change, configured, err := services.FindPrivacyAccounts(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !configured {
		http.Error(w, "privacy not configured — run setup first", http.StatusBadRequest)
		return
	}

	if err := services.StartMixer(passphrase, mixed, 0, change); err != nil {
		log.Printf("StartMixer failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{}"))
}

func PrivacyStopHandler(w http.ResponseWriter, r *http.Request) {
	services.StopMixer()
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{}"))
}

// MixerDebugHandler reads or toggles MIXC + TKBY debug logging on dcrwallet.
func MixerDebugHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"enabled": services.MixerDebugEnabled()})
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := services.SetMixerDebug(ctx, req.Enabled); err != nil {
		log.Printf("SetMixerDebug failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"enabled": req.Enabled})
}

func StreamMixerEventsHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: middleware.SameOriginWS,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade mixer-events WebSocket: %v", err)
		return
	}
	defer conn.Close()

	for _, ev := range services.LastMixerEvents(200) {
		if err := conn.WriteJSON(ev); err != nil {
			return
		}
	}

	ch, unsubscribe := services.SubscribeMixerEvents()
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

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteJSON(ev); err != nil {
				return
			}
		case <-notify:
			return
		}
	}
}

func GetAccountExtendedPubKeyHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}
	accountStr := r.URL.Query().Get("accountNumber")
	if accountStr == "" {
		http.Error(w, "accountNumber required", http.StatusBadRequest)
		return
	}
	accountU64, err := strconv.ParseUint(accountStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid accountNumber", http.StatusBadRequest)
		return
	}
	accountNum := uint32(accountU64)
	if accountNum == importedAccountNumber {
		http.Error(w, "the imported account has no extended pubkey", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	xpub, err := services.GetAccountExtendedPubKey(ctx, accountNum)
	if err != nil {
		log.Printf("GetAccountExtendedPubKey failed: %v", err)
		http.Error(w, "failed to fetch extended pubkey", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"xpub": xpub})
}

func ValidateAddressHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}
	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, "address required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	resp, err := services.ValidateAddress(ctx, address)
	if err != nil {
		http.Error(w, fmt.Sprintf("validate failed: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.ValidateAddressResponse{
		IsValid:       resp.IsValid,
		IsMine:        resp.IsMine,
		AccountNumber: resp.AccountNumber,
	})
}

func ConstructTransactionHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil || rpc.DecodeMessageClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}
	var req types.ConstructTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Address == "" {
		http.Error(w, "address required", http.StatusBadRequest)
		return
	}
	if !req.SendAll && req.AmountAtoms <= 0 {
		http.Error(w, "amount must be positive", http.StatusBadRequest)
		return
	}
	if req.AmountAtoms > 2_100_000_000_000_000 {
		http.Error(w, "amount exceeds total supply", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if vResp, err := services.ValidateAddress(ctx, req.Address); err != nil || !vResp.IsValid {
		http.Error(w, "address has invalid format", http.StatusBadRequest)
		return
	}

	cResp, err := services.ConstructTransaction(ctx, req.SourceAccount, req.Address, req.AmountAtoms, req.SendAll)
	if err != nil {
		log.Printf("ConstructTransaction failed: %v", err)
		http.Error(w, fmt.Sprintf("construct failed: %v", err), http.StatusInternalServerError)
		return
	}

	decoded, err := services.DecodeRawTransaction(ctx, cResp.UnsignedTransaction)
	if err != nil {
		log.Printf("DecodeRawTransaction failed: %v", err)
		http.Error(w, fmt.Sprintf("decode failed: %v", err), http.StatusInternalServerError)
		return
	}
	var inputs, outputs int64
	for _, in := range decoded.Inputs {
		inputs += in.AmountIn
	}
	for _, out := range decoded.Outputs {
		outputs += out.Value
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.ConstructTransactionResponse{
		UnsignedTxHex:       hex.EncodeToString(cResp.UnsignedTransaction),
		InputsTotalAtoms:    inputs,
		OutputsTotalAtoms:   outputs,
		FeeAtoms:            inputs - outputs,
		EstimatedSignedSize: cResp.EstimatedSignedSize,
	})
}

func SignPublishTransactionHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}

	var req types.SignPublishTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.UnsignedTxHex == "" || req.Passphrase == "" {
		http.Error(w, "unsignedTxHex and passphrase required", http.StatusBadRequest)
		return
	}
	if len(req.Passphrase) > 1024 {
		http.Error(w, "passphrase too long", http.StatusBadRequest)
		return
	}
	txBytes, err := hex.DecodeString(req.UnsignedTxHex)
	if err != nil {
		http.Error(w, "invalid unsigned tx hex", http.StatusBadRequest)
		return
	}
	passphrase := []byte(req.Passphrase)
	req.Passphrase = ""

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	txHash, err := services.SignAndPublishTransaction(ctx, req.SourceAccount, txBytes, passphrase)
	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		switch {
		case errors.Is(err, services.ErrSpendWhileMixing):
			http.Error(w, msg, http.StatusConflict)
		case strings.Contains(lower, "watching only"), strings.Contains(lower, "watchingonly"):
			http.Error(w, "This account is watch-only — cannot sign", http.StatusBadRequest)
		case strings.Contains(lower, "passphrase"), strings.Contains(lower, "decrypt"):
			http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
		default:
			log.Printf("SignAndPublishTransaction failed: %v", err)
			http.Error(w, "sign/publish failed", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.SignPublishTransactionResponse{TxHash: txHash})
}

func NextAddressHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletClient == nil || rpc.WalletGrpcClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}
	accountStr := r.URL.Query().Get("account")
	if accountStr == "" {
		accountStr = "0"
	}
	accountU64, err := strconv.ParseUint(accountStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid account", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	addr, err := services.GetNextAddress(ctx, uint32(accountU64))
	if err != nil {
		log.Printf("NextAddress RPC failed: %v", err)
		http.Error(w, fmt.Sprintf("failed to derive address: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.NextAddressResponse{
		Address:       addr,
		AccountNumber: uint32(accountU64),
	})
}
