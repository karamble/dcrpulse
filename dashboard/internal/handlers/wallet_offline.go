// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// reDuplicateTx matches dcrwallet/dcrd errors meaning the transaction is already
// known (already broadcast / in the mempool). That is a benign "already done"
// outcome, not a failure.
var reDuplicateTx = regexp.MustCompile(`(?i)already have|already exists|duplicate|in mempool|transaction already`)

// decodeSignedTxInput resolves the request's signed-transaction bytes. A
// hardware-wallet file is uploaded base64-encoded (binary-safe, since the Passport
// ".dcrtx" file is raw serialized bytes); a pasted hex or "=== ... ===" export
// comes through as plain text.
func decodeSignedTxInput(b64, text string) ([]byte, error) {
	if strings.TrimSpace(b64) != "" {
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
		if err != nil {
			return nil, fmt.Errorf("invalid base64 file data")
		}
		return data, nil
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("signedTx or signedTxB64 required")
	}
	return []byte(text), nil
}

// DecodeSignedTransactionHandler decodes a signed transaction (file or pasted hex)
// into a preview for the user to verify. It uses no private keys and is allowed for
// watch-only wallets.
func DecodeSignedTransactionHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil || rpc.DecodeMessageClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}
	var req types.DecodeSignedTxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	data, err := decodeSignedTxInput(req.SignedTxB64, req.SignedTx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	preview, err := services.PreviewSignedTransaction(ctx, data)
	if err != nil {
		if services.IsDaemonUnreachable(err) {
			respondDaemonError(w, r, services.LogComponentDcrwallet, err)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preview)
}

// BroadcastSignedTransactionHandler publishes an already-signed transaction. It
// uses no private keys and is allowed for watch-only wallets.
func BroadcastSignedTransactionHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}
	var req types.BroadcastSignedTxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	data, err := decodeSignedTxInput(req.SignedTxB64, req.SignedTx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	txBytes, tx, err := services.ParseSignedTransaction(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	txid := tx.TxHash().String()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	txHash, err := services.BroadcastSignedTransaction(ctx, txBytes)
	if err != nil {
		low := strings.ToLower(err.Error())
		switch {
		case services.IsDaemonUnreachable(err):
			respondDaemonError(w, r, services.LogComponentDcrwallet, err)
		case reDuplicateTx.MatchString(low):
			// Already broadcast: report success with the known txid.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(types.BroadcastSignedTxResponse{TxHash: txid, AlreadyBroadcast: true})
		case strings.Contains(low, "missing") || strings.Contains(low, "orphan") || strings.Contains(low, "spent"):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			log.Printf("BroadcastSignedTransaction failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.BroadcastSignedTxResponse{TxHash: txHash})
}

// ParseAccountExportHandler decodes a device account-export file (accounts.dcr)
// into validated entries for the import UI. Parse-only: nothing is imported,
// no keys are needed, and the file carries only public key material.
func ParseAccountExportHandler(w http.ResponseWriter, r *http.Request) {
	var req types.ParseAccountExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.FileB64))
	if err != nil || len(data) == 0 {
		http.Error(w, "invalid base64 file data", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	entries, err := services.ParseAccountExport(ctx, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// The create-new-wallet wizard imports into a wallet that does not exist
	// yet; the open wallet's accounts must not flag its entries.
	if !req.NewWallet {
		services.AnnotateAccountExportConflicts(ctx, entries)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.ParseAccountExportResponse{Entries: entries})
}

// DeviceBalanceHandler exports the wallet's per-account balances and the DCR/USD
// rate as a CBOR BalanceUpdate for an air-gapped device's display (a balance.dcr
// microSD file or a UR QR). It uses no private keys and is allowed for watch-only
// wallets.
func DeviceBalanceHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	export, err := services.BuildDeviceBalance(ctx)
	if err != nil {
		if services.IsDaemonUnreachable(err) {
			respondDaemonError(w, r, services.LogComponentDcrwallet, err)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(export)
}

// BuildSignRequestHandler constructs an unsigned transaction and returns it as a
// base64 CBOR SignRequest for an air-gapped hardware wallet to sign. It uses no
// private keys and is allowed for watch-only wallets.
func BuildSignRequestHandler(w http.ResponseWriter, r *http.Request) {
	if rpc.WalletGrpcClient == nil || rpc.DecodeMessageClient == nil {
		http.Error(w, "wallet not loaded", http.StatusServiceUnavailable)
		return
	}
	var req types.ConstructTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	recipients, err := resolveTxOutputs(ctx, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	export, err := services.BuildSignRequest(ctx, req.SourceAccount, recipients, req.SendAll)
	if err != nil {
		if services.IsDaemonUnreachable(err) {
			respondDaemonError(w, r, services.LogComponentDcrwallet, err)
			return
		}
		// A missing BIP44 index mapping for an imported xpub account is a client
		// fixable condition (re-import specifying the account index).
		if strings.Contains(err.Error(), "BIP44 account index") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(export)
}
