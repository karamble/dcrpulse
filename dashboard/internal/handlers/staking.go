// Copyright (c) 2015-2025 The Decred developers
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

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"

	"github.com/gorilla/websocket"
)

// ListVSPsHandler returns the registry + per-wallet used_vsps envelope.
// Registry is fetched only when the VSP-listing toggle is enabled;
// either source may be empty without an error response, the frontend
// renders accordingly.
func ListVSPsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	envelope := types.ListVSPsResponse{
		RegistryEnabled: services.VSPListingEnabled(),
		VSPs:            []types.VSPInfo{},
		UsedVSPs:        []types.VSPInfo{},
	}

	if envelope.RegistryEnabled {
		vsps, err := services.ListVSPs(ctx)
		if err != nil {
			log.Printf("ListVSPs failed: %v", err)
			envelope.RegistryError = err.Error()
		} else if vsps != nil {
			envelope.VSPs = vsps
		}
	}

	if used, err := services.GetUsedVSPs(ctx); err != nil {
		log.Printf("GetUsedVSPs failed: %v", err)
	} else if used != nil {
		envelope.UsedVSPs = used
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(envelope)
}

// VSPInfoHandler probes one VSP host's /api/v3/vspinfo.
func VSPInfoHandler(w http.ResponseWriter, r *http.Request) {
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	if host == "" {
		http.Error(w, "host query param required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	info, err := services.GetVSPInfo(ctx, host)
	if err != nil {
		log.Printf("GetVSPInfo(%s) failed: %v", host, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// PurchaseTicketsHandler triggers a ticket purchase via dcrwallet.
func PurchaseTicketsHandler(w http.ResponseWriter, r *http.Request) {
	var req types.PurchaseTicketsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	if req.NumTickets == 0 {
		http.Error(w, "numTickets must be > 0", http.StatusBadRequest)
		return
	}
	if req.VspHost == "" || req.VspPubkey == "" {
		http.Error(w, "vspHost and vspPubkey required", http.StatusBadRequest)
		return
	}

	passphrase := []byte(req.Passphrase)
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	resp, err := services.PurchaseTickets(ctx, req.Account, req.NumTickets, req.VspHost, req.VspPubkey, req.ChangeAccount, passphrase)
	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		switch {
		case strings.Contains(lower, "passphrase"), strings.Contains(lower, "decrypt"):
			http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
		case strings.Contains(lower, "insufficient"):
			http.Error(w, msg, http.StatusBadRequest)
		default:
			log.Printf("PurchaseTickets failed: %v", err)
			http.Error(w, msg, http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ListTicketsHandler returns every wallet ticket with status + VSP fee state.
func ListTicketsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	tickets, err := services.ListTickets(ctx)
	if err != nil {
		log.Printf("ListTickets failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tickets)
}

// AutobuyerStatusHandler returns running flag + last error + persisted settings.
func AutobuyerStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	status := services.AutobuyerStatusSnapshot(ctx)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// GetAutobuyerSettingsHandler returns the persisted settings or null.
func GetAutobuyerSettingsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	settings, err := services.LoadAutobuyerSettings(ctx)
	if err != nil {
		log.Printf("LoadAutobuyerSettings: %v", err)
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if settings == nil {
		w.Write([]byte("null"))
		return
	}
	json.NewEncoder(w).Encode(settings)
}

// SaveAutobuyerSettingsHandler atomically persists settings to disk.
func SaveAutobuyerSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var s types.AutobuyerSettings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if s.VspHost == "" || s.VspPubkey == "" {
		http.Error(w, "vspHost and vspPubkey required", http.StatusBadRequest)
		return
	}
	if s.BalanceToMaintain < 0 {
		http.Error(w, "balanceToMaintain must be >= 0", http.StatusBadRequest)
		return
	}
	saveCtx, saveCancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer saveCancel()
	if err := services.SaveAutobuyerSettings(saveCtx, &s); err != nil {
		log.Printf("SaveAutobuyerSettings: %v", err)
		http.Error(w, "failed to save settings", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// StartAutobuyerHandler launches the autobuyer supervisor.
func StartAutobuyerHandler(w http.ResponseWriter, r *http.Request) {
	var req types.StartAutobuyerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	if req.VspHost == "" || req.VspPubkey == "" {
		http.Error(w, "vspHost and vspPubkey required", http.StatusBadRequest)
		return
	}
	if req.BalanceToMaintain < 0 {
		http.Error(w, "balanceToMaintain must be >= 0", http.StatusBadRequest)
		return
	}

	passphrase := []byte(req.Passphrase)
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()

	settings := req.AutobuyerSettings
	if err := services.StartAutobuyer(&settings, passphrase); err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		switch {
		case strings.Contains(lower, "passphrase"), strings.Contains(lower, "decrypt"):
			http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
		case strings.Contains(lower, "already running"):
			http.Error(w, msg, http.StatusConflict)
		default:
			log.Printf("StartAutobuyer failed: %v", err)
			http.Error(w, msg, http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// StopAutobuyerHandler cancels the running supervisor (idempotent).
func StopAutobuyerHandler(w http.ResponseWriter, r *http.Request) {
	services.StopAutobuyer()
	w.WriteHeader(http.StatusNoContent)
}

// StreamAutobuyerEventsHandler upgrades to WebSocket and streams events.
func StreamAutobuyerEventsHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: middleware.SameOriginWS,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade autobuyer-events WebSocket: %v", err)
		return
	}
	defer conn.Close()

	for _, ev := range services.LastAutobuyerEvents(200) {
		if err := conn.WriteJSON(ev); err != nil {
			return
		}
	}

	ch, unsubscribe := services.SubscribeAutobuyerEvents()
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

// SyncFailedVSPTicketsHandler retries VSP fee payments for failed tickets.
func SyncFailedVSPTicketsHandler(w http.ResponseWriter, r *http.Request) {
	var req types.SyncFailedVSPTicketsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	if req.VspHost == "" || req.VspPubkey == "" {
		http.Error(w, "vspHost and vspPubkey required", http.StatusBadRequest)
		return
	}

	passphrase := []byte(req.Passphrase)
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	summary, err := services.SyncFailedVSPTickets(ctx, req.VspHost, req.VspPubkey, req.Account, req.ChangeAccount, passphrase)
	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		switch {
		case strings.Contains(lower, "passphrase"), strings.Contains(lower, "decrypt"):
			http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
		default:
			log.Printf("SyncFailedVSPTickets failed: %v", err)
			http.Error(w, msg, http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}
