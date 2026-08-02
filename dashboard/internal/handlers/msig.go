// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"dcrpulse/internal/msig"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// MsigWalletsHandler lists the active wallet's shared-wallet records.
func MsigWalletsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	walletName, wallets, err := msig.ListForActiveWallet(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		WalletName string               `json:"walletName"`
		Wallets    []*msig.WalletRecord `json:"wallets"`
	}{walletName, wallets})
}

// MsigInviteHandler starts a shared wallet round.
func MsigInviteHandler(w http.ResponseWriter, r *http.Request) {
	if rejectWatchOnly(w, r) {
		return
	}
	var req types.MsigInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	invitees := make([]msig.InviteePeer, 0, len(req.Invitees))
	for _, p := range req.Invitees {
		invitees = append(invitees, msig.InviteePeer{UID: p.UID, Nick: p.Nick})
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	rec, err := msig.CreateSharedWallet(ctx, req.Label, req.M, invitees, req.Account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rec)
}

// MsigAcceptHandler accepts an incoming invite.
func MsigAcceptHandler(w http.ResponseWriter, r *http.Request) {
	if rejectWatchOnly(w, r) {
		return
	}
	var req types.MsigActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := msig.AcceptInvite(ctx, req.ID, req.Account); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MsigDeclineHandler declines an incoming invite.
func MsigDeclineHandler(w http.ResponseWriter, r *http.Request) {
	var req types.MsigActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := msig.DeclineInvite(ctx, req.ID, req.Reason); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MsigCancelHandler withdraws a round this wallet initiated.
func MsigCancelHandler(w http.ResponseWriter, r *http.Request) {
	var req types.MsigActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := msig.CancelRound(ctx, req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MsigDetailHandler returns one record with live balance data when the
// owning wallet is active and the shared address is watched.
func MsigDetailHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id query param is required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	rec, walletName, isActive, err := msig.Detail(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	resp := struct {
		Record         *msig.WalletRecord    `json:"record"`
		WalletName     string                `json:"walletName"`
		IsActiveWallet bool                  `json:"isActiveWallet"`
		UTXOs          []services.SharedUTXO `json:"utxos,omitempty"`
		BalanceAtoms   int64                 `json:"balanceAtoms"`
	}{Record: rec, WalletName: walletName, IsActiveWallet: isActive}
	if isActive && rec.Address != "" && rec.Status == msig.StatusActive {
		if utxos, uerr := services.ListSharedUTXOs(ctx, rec.Address); uerr == nil {
			resp.UTXOs = utxos
			for _, u := range utxos {
				resp.BalanceAtoms += u.Atoms
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// MsigBackupHandler exports the backup card for one shared wallet.
func MsigBackupHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id query param is required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	card, err := msig.ExportBackupCard(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

// MsigPendingHandler lists open invites and deferred imports across all
// local wallets, for badges and banners.
func MsigPendingHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	items, err := msig.Pending(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Items []msig.PendingItem `json:"items"`
		Count int                `json:"count"`
	}{items, len(items)})
}

// MsigRefreshHandler retries unsent frames, resumes wallet-gated steps
// and replays missed frames from every contact.
func MsigRefreshHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	msig.RunSweepNow(ctx)
	msig.CatchUpAllContacts(ctx)
	w.WriteHeader(http.StatusNoContent)
}
