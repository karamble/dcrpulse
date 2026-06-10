// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/pkg/bisonw"
)

// The market-maker handlers proxy bisonw's webserver MM API. Unlike the rest of
// the DEX integration (which uses the RPC server), the market maker is driven
// through the webserver so bot and CEX configuration persists in the daemon's
// encrypted database. Each call requires the DEX app to be unlocked; the
// in-memory app password establishes the webserver session.

// mmWebClient returns the webserver client and the app password, writing the
// appropriate HTTP error and reporting ok=false when the DEX is locked or the
// webserver is unavailable.
func mmWebClient(w http.ResponseWriter) (*bisonw.WebClient, string, bool) {
	appPass, set := rpc.DcrdexAppPass()
	if !set {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return nil, "", false
	}
	c, err := rpc.DcrdexWebClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return nil, "", false
	}
	return c, appPass, true
}

// GetDcrdexMMStatusHandler returns the market-making status (bots + CEX state).
func GetDcrdexMMStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	status, err := client.MMStatus(ctx, appPass)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if len(status) == 0 {
		status = json.RawMessage("null")
	}
	w.Write(status)
}

// GetDcrdexMMMarketReportHandler returns the market report (oracle prices and
// fiat rates) for a market, identified by the host/baseID/quoteID query params.
// The bot configuration UI uses it for the placements chart, the oracle table,
// and lots-to-USD conversion.
func GetDcrdexMMMarketReportHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	host := q.Get("host")
	baseID, err1 := strconv.ParseUint(q.Get("baseID"), 10, 32)
	quoteID, err2 := strconv.ParseUint(q.Get("quoteID"), 10, 32)
	if host == "" || err1 != nil || err2 != nil {
		http.Error(w, "host, baseID and quoteID are required", http.StatusBadRequest)
		return
	}
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	report, err := client.MarketReport(ctx, appPass, host, uint32(baseID), uint32(quoteID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if len(report) == 0 {
		report = json.RawMessage("null")
	}
	w.Write(report)
}

// GetDcrdexMMRunLogsHandler returns a market-maker run's event log (the bot's
// DEX/CEX orders, deposits, and withdrawals) plus overview for the run
// identified by host/baseID/quoteID/startTime. n caps the events returned; the
// optional refID pages older events (the oldest event id already held).
func GetDcrdexMMRunLogsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	host := q.Get("host")
	baseID, err1 := strconv.ParseUint(q.Get("baseID"), 10, 32)
	quoteID, err2 := strconv.ParseUint(q.Get("quoteID"), 10, 32)
	startTime, err3 := strconv.ParseInt(q.Get("startTime"), 10, 64)
	if host == "" || err1 != nil || err2 != nil || err3 != nil {
		http.Error(w, "host, baseID, quoteID and startTime are required", http.StatusBadRequest)
		return
	}
	n, err := strconv.ParseUint(q.Get("n"), 10, 64)
	if err != nil || n == 0 {
		n = 50
	}
	var refID *uint64
	if s := q.Get("refID"); s != "" {
		if v, perr := strconv.ParseUint(s, 10, 64); perr == nil {
			refID = &v
		}
	}
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	logs, err := client.RunLogs(ctx, appPass, host, uint32(baseID), uint32(quoteID), startTime, n, refID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if len(logs) == 0 {
		logs = json.RawMessage("null")
	}
	w.Write(logs)
}

// UpdateDcrdexMMBotConfigHandler persists (and validates) a bot config. The
// request body is a bisonw mm.BotConfig built by the frontend and forwarded
// verbatim.
func UpdateDcrdexMMBotConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(body) == 0 {
		http.Error(w, "config is required", http.StatusBadRequest)
		return
	}
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.UpdateBotConfig(ctx, appPass, body); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// RemoveDcrdexMMBotConfigHandler deletes a stored bot config.
func RemoveDcrdexMMBotConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Host    string `json:"host"`
		BaseID  uint32 `json:"baseID"`
		QuoteID uint32 `json:"quoteID"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Host == "" {
		http.Error(w, "host is required", http.StatusBadRequest)
		return
	}
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.RemoveBotConfig(ctx, appPass, req.Host, req.BaseID, req.QuoteID); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// UpdateDcrdexMMCexConfigHandler stores CEX API credentials. The request body is
// a bisonw mm.CEXConfig {name, apiKey, apiSecret}.
func UpdateDcrdexMMCexConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil || len(body) == 0 {
		http.Error(w, "config is required", http.StatusBadRequest)
		return
	}
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.UpdateCEXConfig(ctx, appPass, body); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// StartDcrdexMMBotHandler starts a configured bot. The request body is a bisonw
// mm.StartConfig (MarketWithHost plus optional alloc/autoRebalance). This spends
// real funds; the frontend gates it behind an explicit confirmation.
func StartDcrdexMMBotHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(body) == 0 {
		http.Error(w, "start config is required", http.StatusBadRequest)
		return
	}
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := client.StartBot(ctx, appPass, body); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// StopDcrdexMMBotHandler stops a running bot on the given market.
func StopDcrdexMMBotHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Host    string `json:"host"`
		BaseID  uint32 `json:"baseID"`
		QuoteID uint32 `json:"quoteID"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Host == "" {
		http.Error(w, "host is required", http.StatusBadRequest)
		return
	}
	client, appPass, ok := mmWebClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.StopBot(ctx, appPass, req.Host, req.BaseID, req.QuoteID); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
