// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// ListVSPsHandler returns the cached public VSP registry.
func ListVSPsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	vsps, err := services.ListVSPs(ctx)
	if err != nil {
		log.Printf("ListVSPs failed: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vsps)
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
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
	}

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
