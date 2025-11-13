// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// WalletExistsHandler checks if a wallet database exists
func WalletExistsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp, err := services.CheckWalletExists(ctx)
	if err != nil {
		log.Printf("Error checking wallet existence: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// WalletLoadedHandler checks if a wallet is currently loaded and ready
func WalletLoadedHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	loaded, err := services.CheckWalletLoaded(ctx)
	resp := types.WalletLoadedResponse{
		Loaded: loaded,
	}

	if err != nil {
		resp.Error = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GenerateSeedHandler generates a new cryptographic seed
func GenerateSeedHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var req types.GenerateSeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Use default if no request body
		req.SeedLength = 33
	}

	resp, err := services.GenerateSeed(ctx, req.SeedLength)
	if err != nil {
		log.Printf("Error generating seed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// CreateWalletHandler creates a new wallet
func CreateWalletHandler(w http.ResponseWriter, r *http.Request) {
	var req types.CreateWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input
	if req.PrivatePassphrase == "" {
		http.Error(w, "Private passphrase is required", http.StatusBadRequest)
		return
	}
	if req.SeedHex == "" {
		http.Error(w, "Seed is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	err := services.CreateNewWallet(ctx, req.PublicPassphrase, req.PrivatePassphrase, req.SeedHex)
	if err != nil {
		log.Printf("Error creating wallet: %v", err)
		resp := types.CreateWalletResponse{
			Success: false,
			Message: err.Error(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Wallet created successfully
	resp := types.CreateWalletResponse{
		Success: true,
		Message: "Wallet created successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// OpenWalletHandler opens an existing wallet
func OpenWalletHandler(w http.ResponseWriter, r *http.Request) {
	var req types.OpenWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Public passphrase can be empty if wallet was created without one
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	err := services.OpenWallet(ctx, req.PublicPassphrase)
	if err != nil {
		log.Printf("Error opening wallet: %v", err)
		resp := types.OpenWalletResponse{
			Success: false,
			Message: err.Error(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp := types.OpenWalletResponse{
		Success: true,
		Message: "Wallet opened successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
