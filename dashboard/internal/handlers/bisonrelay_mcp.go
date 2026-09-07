// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// Proxies for brclientd's BR-MCP client engine (Settings > AI Agents >
// BR-MCP). brclientd speaks atoms; the frontend speaks DCR, converted by the
// services decoders that the agent-facing MCP resource shares.

// brMCPSettingsWire is the daemon-side settings shape the save path posts.
type brMCPSettingsWire = services.BRMCPSettingsWire

// mergeOwnedMCPSettings overlays the dashboard-owned settings keys onto the
// daemon's current settings JSON, so a field a newer brclientd stores
// survives the save; last_denied is reply-only and never posted back.
func mergeOwnedMCPSettings(current json.RawMessage, wire brMCPSettingsWire) (map[string]json.RawMessage, error) {
	merged := make(map[string]json.RawMessage)
	if err := json.Unmarshal(current, &merged); err != nil {
		return nil, err
	}
	delete(merged, "last_denied")
	ownedJSON, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	var owned map[string]json.RawMessage
	if err := json.Unmarshal(ownedJSON, &owned); err != nil {
		return nil, err
	}
	for k, v := range owned {
		merged[k] = v
	}
	return merged, nil
}

// redactBridgeToken blanks the bearer secret for a reply, recording only that
// one exists. The plaintext is returned by the call that mints it and never
// again, the way an agent token is (mcp.AgentInfo carries no token at all).
//
// TokenSet is kept when the daemon already reported it: brclientd holds a hash
// and sends token_set with no token, so deriving the flag from the value alone
// would report "no token" on every read.
func redactBridgeToken(s types.BRMCPSettings) types.BRMCPSettings {
	s.TokenSet = s.TokenSet || s.Token != ""
	s.Token = ""
	s.RecycleToken = false
	return s
}

// BisonrelayMCPSettingsHandler round-trips the BR-MCP client settings.
func BisonrelayMCPSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		view, err := services.FetchBRMCPSettings(r.Context())
		if err != nil {
			brWriteErr(w, err)
			return
		}
		writeJSON(w, redactBridgeToken(view))
	case http.MethodPost:
		var view types.BRMCPSettings
		if err := json.NewDecoder(r.Body).Decode(&view); err != nil {
			http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
			return
		}
		perCall, err := dcrutil.NewAmount(view.PerCallCapDcr)
		if err != nil || perCall < 0 {
			http.Error(w, "invalid per-call cap", http.StatusBadRequest)
			return
		}
		perDay, err := dcrutil.NewAmount(view.PerDayCapDcr)
		if err != nil || perDay < 0 {
			http.Error(w, "invalid per-day cap", http.StatusBadRequest)
			return
		}
		// Read the daemon's settings first, failing closed: the merge below
		// needs them.
		current, err := rpc.BrclientdMCPSettings(r.Context())
		if err != nil {
			brWriteErr(w, err)
			return
		}
		wire := brMCPSettingsWire{
			Enabled:             view.Enabled,
			RecycleToken:        view.RecycleToken,
			Mode:                view.Mode,
			PerCallCapAtoms:     int64(perCall),
			PerDayCapAtoms:      int64(perDay),
			AllowedBots:         view.AllowedBots,
			AllowedIPs:          view.AllowedIPs,
			ApprovalTimeoutSecs: view.ApprovalTimeoutSecs,
			TipWaitSecs:         view.TipWaitSecs,
		}
		// Merge over the daemon's current settings rather than replacing them,
		// so a field a newer brclientd stores survives the save.
		merged, err := mergeOwnedMCPSettings(current, wire)
		if err != nil {
			http.Error(w, "parse settings: "+err.Error(), http.StatusBadGateway)
			return
		}
		raw, err := rpc.BrclientdMCPApplySettings(r.Context(), merged)
		if err != nil {
			brWriteErr(w, err)
			return
		}
		applied, err := services.DecodeBRMCPSettings(raw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		// The one time the plaintext is shown is the reply to the call that
		// minted it; the operator has no other way to learn it.
		if view.RecycleToken {
			applied.TokenSet = applied.Token != ""
			writeJSON(w, applied)
			return
		}
		writeJSON(w, redactBridgeToken(applied))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// BisonrelayMCPPendingHandler lists payments awaiting approval.
func BisonrelayMCPPendingHandler(w http.ResponseWriter, r *http.Request) {
	pending, err := services.FetchBRMCPPending(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	writeJSON(w, struct {
		Pending []types.BRMCPPending `json:"pending"`
	}{Pending: pending})
}

// BisonrelayMCPResolvePendingHandler approves or denies one pending payment.
func BisonrelayMCPResolvePendingHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string `json:"id"`
		Approve bool   `json:"approve"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	brDo204(w, func() error { return rpc.BrclientdMCPResolvePending(r.Context(), req.ID, req.Approve) })
}

// BisonrelayMCPSpendHandler returns the spend log and the rolling-day total.
func BisonrelayMCPSpendHandler(w http.ResponseWriter, r *http.Request) {
	spend, err := services.FetchBRMCPSpend(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	writeJSON(w, spend)
}
