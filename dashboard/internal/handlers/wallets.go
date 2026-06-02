// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// daemonSwitchTimeout bounds operations that relaunch the dcrwallet daemon and
// reconnect gRPC (select / create).
const daemonSwitchTimeout = 90 * time.Second

// ListWalletsHandler returns every wallet available on disk plus the active one.
func ListWalletsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	wallets, err := services.ListWallets(ctx)
	if err != nil {
		log.Printf("Error listing wallets: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"wallets": wallets,
		"active":  services.ActiveWalletName(),
	})
}

// SelectWalletHandler switches the active wallet, relaunching the daemon.
func SelectWalletHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string `json:"name"`
		PublicPassphrase string `json:"publicPassphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), daemonSwitchTimeout)
	defer cancel()

	if err := services.SwitchWallet(ctx, req.Name, req.PublicPassphrase); err != nil {
		log.Printf("Error selecting wallet %q: %v", req.Name, err)
		status := http.StatusInternalServerError
		// A passphrase mismatch surfaces from OpenWallet; report it as a 401 so
		// the UI can prompt again (mirrors OpenWalletHandler).
		if strings.Contains(err.Error(), "passphrase") || strings.Contains(err.Error(), "open wallet") {
			status = http.StatusUnauthorized
		}
		writeJSONError(w, status, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "active": services.ActiveWalletName()})
}

// CloseWalletHandler closes the active wallet and returns to the wallet list.
func CloseWalletHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := services.CloseActiveWallet(ctx); err != nil {
		log.Printf("Error closing wallet: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// CreateNamedWalletHandler creates a new named wallet and makes it active.
func CreateNamedWalletHandler(w http.ResponseWriter, r *http.Request) {
	var req types.CreateWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	name := req.Name
	if name == "" {
		name = config.DefaultWalletName
	}
	if err := services.ValidateWalletName(name); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateCreateWalletPassphrases(&req); err != "" {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	if req.SeedHex == "" {
		writeJSONError(w, http.StatusBadRequest, "Seed is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), daemonSwitchTimeout)
	defer cancel()

	if err := services.CreateNamedWallet(ctx, name, req.PublicPassphrase, req.PrivatePassphrase, req.SeedHex, req.DiscoverAccounts); err != nil {
		log.Printf("Error creating wallet %q: %v", name, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(types.CreateWalletResponse{Success: false, Message: err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.CreateWalletResponse{Success: true, Message: "Wallet created successfully"})
}

// RenameWalletHandler renames a non-active, non-default wallet.
func RenameWalletHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := services.RenameWallet(ctx, req.From, req.To); err != nil {
		log.Printf("Error renaming wallet %q -> %q: %v", req.From, req.To, err)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// DeleteWalletHandler backs up and removes a non-active wallet.
func DeleteWalletHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := services.DeleteWallet(ctx, req.Name); err != nil {
		log.Printf("Error deleting wallet %q: %v", req.Name, err)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"success": false, "message": msg})
}

// validateCreateWalletPassphrases mirrors CreateWalletHandler's checks; returns
// an error message string, or "" when valid.
func validateCreateWalletPassphrases(req *types.CreateWalletRequest) string {
	if req.PrivatePassphrase == "" {
		return "Private passphrase is required"
	}
	if len(req.PrivatePassphrase) < 8 {
		return "Private passphrase must be at least 8 characters"
	}
	if len(req.PrivatePassphrase) > 1024 || len(req.ConfirmPrivatePassphrase) > 1024 ||
		len(req.PublicPassphrase) > 1024 || len(req.ConfirmPublicPassphrase) > 1024 {
		return "Passphrase too long"
	}
	if req.PrivatePassphrase != req.ConfirmPrivatePassphrase {
		return "Private passphrases do not match"
	}
	if req.PublicPassphrase != "" {
		if len(req.PublicPassphrase) < 8 {
			return "Public passphrase must be at least 8 characters"
		}
		if req.PublicPassphrase != req.ConfirmPublicPassphrase {
			return "Public passphrases do not match"
		}
	}
	return ""
}
