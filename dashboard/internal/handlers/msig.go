// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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

// msigPassphraseError maps wallet unlock failures to 401 the way the
// send path does, so the UI can re-prompt instead of showing a raw error.
func msigPassphraseError(w http.ResponseWriter, err error) {
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "passphrase") ||
		strings.Contains(strings.ToLower(msg), "decrypt") {
		http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
		return
	}
	http.Error(w, msg, http.StatusBadRequest)
}

// MsigProposeHandler builds and dispatches a shared wallet payment.
func MsigProposeHandler(w http.ResponseWriter, r *http.Request) {
	if rejectWatchOnly(w, r) {
		return
	}
	var req types.MsigProposeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Passphrase) > 1024 {
		http.Error(w, "Passphrase too long", http.StatusBadRequest)
		return
	}
	passphrase := []byte(req.Passphrase)
	req.Passphrase = ""
	recipients := make([]msig.Recipient, 0, len(req.Recipients))
	for _, rc := range req.Recipients {
		recipients = append(recipients, msig.Recipient{Address: rc.Address, Atoms: rc.AmountAtoms})
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	prop, err := msig.ProposeSpend(ctx, req.WalletID, recipients, req.QueueUIDs, req.Note,
		time.Duration(req.HopTTLSecs)*time.Second, req.Account, passphrase)
	if err != nil {
		msigPassphraseError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(prop)
}

// MsigSignHandler adds this wallet's signature to an incoming request.
func MsigSignHandler(w http.ResponseWriter, r *http.Request) {
	if rejectWatchOnly(w, r) {
		return
	}
	var req types.MsigProposalActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.WalletID == "" || req.TxID == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Passphrase) > 1024 {
		http.Error(w, "Passphrase too long", http.StatusBadRequest)
		return
	}
	passphrase := []byte(req.Passphrase)
	req.Passphrase = ""
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := msig.SignIncomingProposal(ctx, req.WalletID, req.TxID, req.Account, passphrase); err != nil {
		msigPassphraseError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MsigRejectHandler declines an incoming payment request.
func MsigRejectHandler(w http.ResponseWriter, r *http.Request) {
	var req types.MsigProposalActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.WalletID == "" || req.TxID == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := msig.RejectIncomingProposal(ctx, req.WalletID, req.TxID, req.Reason); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MsigAbortHandler cancels a payment this wallet proposed.
func MsigAbortHandler(w http.ResponseWriter, r *http.Request) {
	var req types.MsigProposalActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.WalletID == "" || req.TxID == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := msig.AbortProposal(ctx, req.WalletID, req.TxID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MsigRebroadcastHandler retries a fully signed payment.
func MsigRebroadcastHandler(w http.ResponseWriter, r *http.Request) {
	var req types.MsigProposalActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.WalletID == "" || req.TxID == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := msig.RebroadcastProposal(ctx, req.WalletID, req.TxID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MsigRestoreHandler imports a backup card into the active wallet.
func MsigRestoreHandler(w http.ResponseWriter, r *http.Request) {
	if rejectWatchOnly(w, r) {
		return
	}
	var card msig.BackupCard
	if err := json.NewDecoder(r.Body).Decode(&card); err != nil {
		http.Error(w, "The backup file could not be read", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	rec, err := msig.ImportBackupCard(ctx, &card)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rec)
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
