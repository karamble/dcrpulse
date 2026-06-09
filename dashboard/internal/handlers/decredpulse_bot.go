// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
)

// JoinDecredPulseHandler requests an invite into the community "Decred Pulse"
// group chat from the brulse invite bot and redeems it locally. Redeeming the
// invite starts key exchange with the bot; once it completes the bot sends a
// group-chat invite which the frontend accepts. No funds are involved.
func JoinDecredPulseHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Resolve the local user's own Bison Relay public identity and convert it
	// from brclientd's base64 encoding to the hex the bot expects.
	raw, err := rpc.BrclientdUserPublicIdentity(ctx)
	if err != nil {
		http.Error(w, "could not read local identity: "+err.Error(), http.StatusBadGateway)
		return
	}
	var id struct {
		Identity string `json:"identity"`
	}
	if err := json.Unmarshal(raw, &id); err != nil || id.Identity == "" {
		http.Error(w, "could not determine local identity", http.StatusBadGateway)
		return
	}
	idBytes, err := base64.StdEncoding.DecodeString(id.Identity)
	if err != nil || len(idBytes) == 0 {
		http.Error(w, "malformed local identity", http.StatusBadGateway)
		return
	}
	pubkeyHex := hex.EncodeToString(idBytes)

	// Ask the bot for an invite (solving its proof-of-work challenge).
	inviteKey, err := services.RequestDecredPulseInvite(ctx, pubkeyHex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Redeem the invite to begin KX with the bot.
	if err := rpc.BrclientdRedeemPaidInviteKey(ctx, inviteKey); err != nil {
		http.Error(w, "could not redeem invite: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
