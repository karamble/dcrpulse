// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"dcrpulse/internal/services"
)

// SearchHandler handles universal search requests
func SearchHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Missing search query", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := services.UniversalSearch(ctx, query)
	if err != nil {
		log.Printf("Search error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GetRecentBlocksHandler returns a list of recent blocks with pagination
func GetRecentBlocksHandler(w http.ResponseWriter, r *http.Request) {
	// Get page parameter (default 1)
	pageStr := r.URL.Query().Get("page")
	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	// Get pageSize parameter (default 10, max 100)
	pageSizeStr := r.URL.Query().Get("pageSize")
	pageSize := 10
	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
			if pageSize > 100 {
				pageSize = 100 // Limit to 100 blocks per page
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	response, err := services.FetchRecentBlocksPaginated(ctx, page, pageSize)
	if err != nil {
		log.Printf("Error fetching recent blocks: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetBlockByHeightHandler returns detailed block info by height
func GetBlockByHeightHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	heightStr := vars["height"]

	height, err := strconv.ParseInt(heightStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid block height", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	block, err := services.FetchBlockByHeight(ctx, height)
	if err != nil {
		log.Printf("Error fetching block %d: %v", height, err)
		http.Error(w, "Block not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(block)
}

// GetBlockByHashHandler returns detailed block info by hash
func GetBlockByHashHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hash := vars["hash"]

	if hash == "" {
		http.Error(w, "Missing block hash", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	block, err := services.FetchBlockByHash(ctx, hash)
	if err != nil {
		log.Printf("Error fetching block %s: %v", hash, err)
		http.Error(w, "Block not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(block)
}

// GetTransactionHandler returns detailed transaction info
func GetTransactionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	txHash := vars["txhash"]

	if txHash == "" {
		http.Error(w, "Missing transaction hash", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := services.FetchTransaction(ctx, txHash)
	if err != nil {
		log.Printf("Error fetching transaction %s: %v", txHash, err)
		http.Error(w, "Transaction not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tx)
}

// GetAddressHandler returns address information (limited without addrindex)
func GetAddressHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]

	if address == "" {
		http.Error(w, "Missing address", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info, err := services.FetchAddressInfo(ctx, address)
	if err != nil {
		log.Printf("Error fetching address info for %s: %v", address, err)
		http.Error(w, "Failed to fetch address information", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// GetMempoolTransactionsHandler returns all current mempool transactions
func GetMempoolTransactionsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mempool, err := services.FetchMempoolTransactions(ctx)
	if err != nil {
		log.Printf("Error fetching mempool transactions: %v", err)
		http.Error(w, "Failed to fetch mempool transactions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mempool)
}
