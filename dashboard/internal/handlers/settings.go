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
	"strings"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// GetSettingsHandler returns the per-wallet + global settings envelope.
// Missing keys yield safe defaults so the UI always has something to
// render.
func GetSettingsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	network, _ := services.CurrentNetwork(ctx)
	walletName := services.CurrentWalletName()

	walletOut := types.WalletSettings{GapLimit: 200}
	globalOut := types.GlobalSettings{
		ExternalRequests: types.ExternalRequestSettings{
			VSPListing: true,
			Politeia:   true,
			Brseeder:   true,
		},
	}

	if network != "" {
		if wc, err := config.LoadWalletCfg(network, walletName); err == nil {
			var gap int
			if ok, _ := wc.Get(config.KeyGapLimit, &gap); ok && gap > 0 {
				walletOut.GapLimit = gap
			}
			var currency string
			if ok, _ := wc.Get("currency_display", &currency); ok {
				walletOut.CurrencyDisplay = currency
			}
		}
	}

	if gc, err := config.LoadGlobalCfg(); err == nil {
		allowed, _ := gc.AllowedExternalRequests()
		if allowed != nil {
			if v, ok := allowed[config.ExternalRequestVSPListing]; ok {
				globalOut.ExternalRequests.VSPListing = v
			}
			if v, ok := allowed[config.ExternalRequestPoliteia]; ok {
				globalOut.ExternalRequests.Politeia = v
			}
			if v, ok := allowed[config.ExternalRequestBrseeder]; ok {
				globalOut.ExternalRequests.Brseeder = v
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.SettingsEnvelope{
		Wallet: &walletOut,
		Global: &globalOut,
	})
}

// SaveSettingsHandler updates either or both subsections. Partial
// envelopes are accepted; unknown Decrediton keys in the underlying
// files are preserved by the WalletCfg/GlobalCfg layers.
func SaveSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var req types.SettingsEnvelope
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if req.Wallet != nil {
		network, err := services.CurrentNetwork(ctx)
		if err != nil {
			log.Printf("settings save: network lookup: %v", err)
			http.Error(w, "network not available", http.StatusServiceUnavailable)
			return
		}
		wc, err := config.LoadWalletCfg(network, services.CurrentWalletName())
		if err != nil {
			log.Printf("settings save: load wallet cfg: %v", err)
			http.Error(w, "failed to load settings", http.StatusInternalServerError)
			return
		}
		if req.Wallet.GapLimit > 0 {
			if err := wc.Set(config.KeyGapLimit, req.Wallet.GapLimit); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if req.Wallet.CurrencyDisplay != "" {
			if err := wc.Set("currency_display", req.Wallet.CurrencyDisplay); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if err := wc.Save(); err != nil {
			log.Printf("settings save: save wallet cfg: %v", err)
			http.Error(w, "failed to save settings", http.StatusInternalServerError)
			return
		}
	}

	if req.Global != nil {
		gc, err := config.LoadGlobalCfg()
		if err != nil {
			log.Printf("settings save: load global cfg: %v", err)
			http.Error(w, "failed to load global settings", http.StatusInternalServerError)
			return
		}
		allowed, _ := gc.AllowedExternalRequests()
		if allowed == nil {
			allowed = map[string]bool{}
		}
		allowed[config.ExternalRequestVSPListing] = req.Global.ExternalRequests.VSPListing
		allowed[config.ExternalRequestPoliteia] = req.Global.ExternalRequests.Politeia
		allowed[config.ExternalRequestBrseeder] = req.Global.ExternalRequests.Brseeder
		if err := gc.SetAllowedExternalRequests(allowed); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := gc.Save(); err != nil {
			log.Printf("settings save: save global cfg: %v", err)
			http.Error(w, "failed to save global settings", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// ChangePassphraseHandler rotates the wallet's private passphrase.
func ChangePassphraseHandler(w http.ResponseWriter, r *http.Request) {
	var req types.ChangePassphraseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.NewPassphrase == "" {
		http.Error(w, "newPassphrase required", http.StatusBadRequest)
		return
	}
	if len(req.NewPassphrase) < 8 {
		http.Error(w, "new passphrase must be at least 8 characters", http.StatusBadRequest)
		return
	}
	if len(req.OldPassphrase) > 1024 || len(req.NewPassphrase) > 1024 {
		http.Error(w, "passphrase too long", http.StatusBadRequest)
		return
	}

	oldPass := []byte(req.OldPassphrase)
	newPass := []byte(req.NewPassphrase)
	defer func() {
		for i := range oldPass {
			oldPass[i] = 0
		}
		for i := range newPass {
			newPass[i] = 0
		}
	}()

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	if err := services.ChangePrivatePassphrase(ctx, oldPass, newPass); err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		switch {
		case strings.Contains(lower, "passphrase"), strings.Contains(lower, "decrypt"):
			http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
		default:
			log.Printf("ChangePrivatePassphrase failed: %v", err)
			http.Error(w, msg, http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetLogsHandler returns the tail of a daemon log file.
// Query params: component=dcrd|dcrwallet, lines=N (default 200, max 5000).
func GetLogsHandler(w http.ResponseWriter, r *http.Request) {
	component := r.URL.Query().Get("component")
	if component == "" {
		component = "dcrwallet"
	}
	linesStr := r.URL.Query().Get("lines")
	lines := 200
	if linesStr != "" {
		if n, err := strconv.Atoi(linesStr); err == nil && n > 0 {
			lines = n
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	out, err := services.TailLog(ctx, services.LogComponent(component), lines)
	if err != nil {
		log.Printf("TailLog(%s): %v", component, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"component": component,
		"lines":     out,
	})
}

// DiscoverAddressesHandler triggers a chain scan for previously-used
// addresses under the requested gap limit. Long-running.
func DiscoverAddressesHandler(w http.ResponseWriter, r *http.Request) {
	var req types.DiscoverUsageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	if req.GapLimit == 0 {
		req.GapLimit = 200
	}
	if req.GapLimit > 10000 {
		http.Error(w, "gapLimit too large (max 10000)", http.StatusBadRequest)
		return
	}

	passphrase := []byte(req.Passphrase)
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()

	// Persist the gap limit as a preference so the modal pre-fills it
	// next time. The actual scan value is taken from req.GapLimit.
	if network, err := services.CurrentNetwork(r.Context()); err == nil {
		if wc, err := config.LoadWalletCfg(network, services.CurrentWalletName()); err == nil {
			_ = wc.Set(config.KeyGapLimit, int(req.GapLimit))
			_ = wc.Save()
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	if err := services.DiscoverUsage(ctx, passphrase, req.DiscoverAccounts, req.GapLimit); err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		switch {
		case strings.Contains(lower, "passphrase"), strings.Contains(lower, "decrypt"):
			http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
		default:
			log.Printf("DiscoverUsage failed: %v", err)
			http.Error(w, msg, http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
