// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/rpc"
)

// Proxies for brclientd's BR-MCP client engine (Settings > AI Agents >
// BR-MCP). brclientd speaks atoms; the frontend speaks DCR, converted here
// via dcrutil.

func brMCPJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// brMCPSettingsWire mirrors brclientd's mcpclient.json shape. The listener
// address is not here - it is brclientd startup config (mcplisten), not a
// runtime setting.
type brMCPSettingsWire struct {
	Enabled             bool     `json:"enabled"`
	Token               string   `json:"token"`
	Mode                string   `json:"mode"`
	PerCallCapAtoms     int64    `json:"per_call_cap_atoms"`
	PerDayCapAtoms      int64    `json:"per_day_cap_atoms"`
	AllowedBots         []string `json:"allowed_bots"`
	ApprovalTimeoutSecs int      `json:"approval_timeout_secs"`
	TipWaitSecs         int      `json:"tip_wait_secs"`
}

// brMCPSettingsView is the DCR-denominated frontend shape.
type brMCPSettingsView struct {
	Enabled             bool     `json:"enabled"`
	Token               string   `json:"token"`
	Mode                string   `json:"mode"`
	PerCallCapDcr       float64  `json:"perCallCapDcr"`
	PerDayCapDcr        float64  `json:"perDayCapDcr"`
	AllowedBots         []string `json:"allowedBots"`
	ApprovalTimeoutSecs int      `json:"approvalTimeoutSecs"`
	TipWaitSecs         int      `json:"tipWaitSecs"`
}

func brMCPSettingsToView(w brMCPSettingsWire) brMCPSettingsView {
	if w.AllowedBots == nil {
		w.AllowedBots = []string{}
	}
	return brMCPSettingsView{
		Enabled:             w.Enabled,
		Token:               w.Token,
		Mode:                w.Mode,
		PerCallCapDcr:       dcrutil.Amount(w.PerCallCapAtoms).ToCoin(),
		PerDayCapDcr:        dcrutil.Amount(w.PerDayCapAtoms).ToCoin(),
		AllowedBots:         w.AllowedBots,
		ApprovalTimeoutSecs: w.ApprovalTimeoutSecs,
		TipWaitSecs:         w.TipWaitSecs,
	}
}

// BisonrelayMCPSettingsHandler round-trips the BR-MCP client settings.
func BisonrelayMCPSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		raw, err := rpc.BrclientdMCPSettings(r.Context())
		if err != nil {
			brWriteErr(w, err)
			return
		}
		var wire brMCPSettingsWire
		if err := json.Unmarshal(raw, &wire); err != nil {
			http.Error(w, "parse settings: "+err.Error(), http.StatusBadGateway)
			return
		}
		brMCPJSON(w, brMCPSettingsToView(wire))
	case http.MethodPost:
		var view brMCPSettingsView
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
		wire := brMCPSettingsWire{
			Enabled:             view.Enabled,
			Token:               view.Token,
			Mode:                view.Mode,
			PerCallCapAtoms:     int64(perCall),
			PerDayCapAtoms:      int64(perDay),
			AllowedBots:         view.AllowedBots,
			ApprovalTimeoutSecs: view.ApprovalTimeoutSecs,
			TipWaitSecs:         view.TipWaitSecs,
		}
		raw, err := rpc.BrclientdMCPApplySettings(r.Context(), wire)
		if err != nil {
			brWriteErr(w, err)
			return
		}
		var applied brMCPSettingsWire
		if err := json.Unmarshal(raw, &applied); err != nil {
			http.Error(w, "parse settings: "+err.Error(), http.StatusBadGateway)
			return
		}
		brMCPJSON(w, brMCPSettingsToView(applied))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// BisonrelayMCPPendingHandler lists payments awaiting approval.
func BisonrelayMCPPendingHandler(w http.ResponseWriter, r *http.Request) {
	raw, err := rpc.BrclientdMCPPending(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	var wire struct {
		Pending []struct {
			ID      string `json:"id"`
			Bot     string `json:"bot"`
			Tool    string `json:"tool"`
			Atoms   int64  `json:"atoms"`
			Created int64  `json:"created"`
		} `json:"pending"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		http.Error(w, "parse pending: "+err.Error(), http.StatusBadGateway)
		return
	}
	type entry struct {
		ID        string  `json:"id"`
		Bot       string  `json:"bot"`
		Tool      string  `json:"tool"`
		AmountDcr float64 `json:"amountDcr"`
		Created   int64   `json:"created"`
	}
	out := make([]entry, 0, len(wire.Pending))
	for _, p := range wire.Pending {
		out = append(out, entry{
			ID: p.ID, Bot: p.Bot, Tool: p.Tool,
			AmountDcr: dcrutil.Amount(p.Atoms).ToCoin(),
			Created:   p.Created,
		})
	}
	brMCPJSON(w, struct {
		Pending []entry `json:"pending"`
	}{Pending: out})
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
	if err := rpc.BrclientdMCPResolvePending(r.Context(), req.ID, req.Approve); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayMCPSpendHandler returns the spend log and the rolling-day total.
func BisonrelayMCPSpendHandler(w http.ResponseWriter, r *http.Request) {
	raw, err := rpc.BrclientdMCPSpend(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	var wire struct {
		Entries []struct {
			TS    int64  `json:"ts"`
			Bot   string `json:"bot"`
			Tool  string `json:"tool"`
			Rail  string `json:"rail"`
			Atoms int64  `json:"atoms"`
		} `json:"entries"`
		TodayAtoms int64 `json:"today_atoms"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		http.Error(w, "parse spend: "+err.Error(), http.StatusBadGateway)
		return
	}
	type entry struct {
		TS        int64   `json:"ts"`
		Bot       string  `json:"bot"`
		Tool      string  `json:"tool"`
		Rail      string  `json:"rail"`
		AmountDcr float64 `json:"amountDcr"`
	}
	out := make([]entry, 0, len(wire.Entries))
	for _, e := range wire.Entries {
		out = append(out, entry{
			TS: e.TS, Bot: e.Bot, Tool: e.Tool, Rail: e.Rail,
			AmountDcr: dcrutil.Amount(e.Atoms).ToCoin(),
		})
	}
	brMCPJSON(w, struct {
		Entries  []entry `json:"entries"`
		TodayDcr float64 `json:"todayDcr"`
	}{Entries: out, TodayDcr: dcrutil.Amount(wire.TodayAtoms).ToCoin()})
}
