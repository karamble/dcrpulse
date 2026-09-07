// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"net/http"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
)

// The handlers wrote their JSON replies by hand, each setting the same header
// before encoding, and answered an unwired daemon client in four different
// wordings. Both live here now so a reply shape is stated once.

// writeJSON sends v as a JSON body with a 200.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONStatus sends v as a JSON body with an explicit status code.
func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONError sends the {success,message} envelope the wallet routes use.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSONStatus(w, status, map[string]any{"success": false, "message": msg})
}

// msgWalletUnavailable is the single wording for "the wallet is not wired up
// yet", which the routes used to phrase four different ways for one condition.
const msgWalletUnavailable = "wallet is not available yet"

// walletUnavailable writes that answer. Callers that check a combination of
// clients keep their own condition and call this for the reply.
func walletUnavailable(w http.ResponseWriter) {
	http.Error(w, msgWalletUnavailable, http.StatusServiceUnavailable)
}

// walletRPCReady and walletGRPCReady fold the two single-client guards that
// appear on nearly every route: check, answer, and tell the caller to stop.
func walletRPCReady(w http.ResponseWriter) bool {
	if rpc.WalletClient == nil {
		walletUnavailable(w)
		return false
	}
	return true
}

func walletGRPCReady(w http.ResponseWriter) bool {
	if rpc.WalletGrpcClient == nil {
		walletUnavailable(w)
		return false
	}
	return true
}

// dcrdReadyForWallet answers 503 when dcrd's polled state says wallet RPC
// cannot serve yet, and reports whether the handler may continue.
func dcrdReadyForWallet(w http.ResponseWriter) bool {
	if gate, reason := services.NodeWalletGateState(); gate != services.GateOK {
		http.Error(w, reason, http.StatusServiceUnavailable)
		return false
	}
	return true
}
