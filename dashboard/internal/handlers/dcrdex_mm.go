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

	"dcrpulse/pkg/bisonw"
)

// The market-maker handlers proxy bisonw's webserver MM API, which persists
// bot and CEX configuration in the daemon's encrypted database. Each call
// rides the webserver cookie session established at unlock.

// GetDcrdexMMStatusHandler returns the market-making status (bots + CEX state).
func GetDcrdexMMStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	client, ok := dexWebSession(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	status, err := client.MMStatus(ctx)
	if err != nil {
		dexWriteErr(w, err)
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
	client, ok := dexWebSession(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	report, err := client.MarketReport(ctx, host, uint32(baseID), uint32(quoteID))
	if err != nil {
		dexWriteErr(w, err)
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
	client, ok := dexWebSession(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	logs, err := client.RunLogs(ctx, host, uint32(baseID), uint32(quoteID), startTime, n, refID)
	if err != nil {
		dexWriteErr(w, err)
		return
	}
	if len(logs) == 0 {
		logs = json.RawMessage("null")
	}
	w.Write(logs)
}

// GetDcrdexMMArchivedRunsHandler returns the market-maker run history: past runs
// (start time, market, profit), newest first, for the run-history view.
func GetDcrdexMMArchivedRunsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	client, ok := dexWebSession(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	runs, err := client.ArchivedRuns(ctx)
	if err != nil {
		dexWriteErr(w, err)
		return
	}
	if len(runs) == 0 {
		runs = json.RawMessage("[]")
	}
	w.Write(runs)
}

// mmConfigUpdate posts a raw config body; the limit differs per member
// because a bot config carries markets while CEX credentials are small.
func mmConfigUpdate(w http.ResponseWriter, r *http.Request, limit int64,
	act func(ctx context.Context, client *bisonw.WebClient, body []byte) error) {
	w.Header().Set("Content-Type", "application/json")
	body, err := io.ReadAll(io.LimitReader(r.Body, limit))
	if err != nil || len(body) == 0 {
		http.Error(w, "config is required", http.StatusBadRequest)
		return
	}
	client, ok := dexWebSession(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := act(ctx, client, body); err != nil {
		dexWriteErr(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// UpdateDcrdexMMBotConfigHandler persists (and validates) a bot config. The
// request body is a bisonw mm.BotConfig built by the frontend and forwarded
// verbatim.
func UpdateDcrdexMMBotConfigHandler(w http.ResponseWriter, r *http.Request) {
	mmConfigUpdate(w, r, 1<<20, func(ctx context.Context, client *bisonw.WebClient, body []byte) error {
		return client.UpdateBotConfig(ctx, body)
	})
}

// mmMarketAction decodes a {host, baseID, quoteID} action; the members differ
// only in the webclient call.
func mmMarketAction(w http.ResponseWriter, r *http.Request,
	act func(ctx context.Context, client *bisonw.WebClient, host string, baseID, quoteID uint32) error) {
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
	client, ok := dexWebSession(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := act(ctx, client, req.Host, req.BaseID, req.QuoteID); err != nil {
		dexWriteErr(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// RemoveDcrdexMMBotConfigHandler deletes a stored bot config.
func RemoveDcrdexMMBotConfigHandler(w http.ResponseWriter, r *http.Request) {
	mmMarketAction(w, r, func(ctx context.Context, client *bisonw.WebClient, host string, baseID, quoteID uint32) error {
		return client.RemoveBotConfig(ctx, host, baseID, quoteID)
	})
}

// UpdateDcrdexMMCexConfigHandler stores CEX API credentials. The request body is
// a bisonw mm.CEXConfig {name, apiKey, apiSecret}.
func UpdateDcrdexMMCexConfigHandler(w http.ResponseWriter, r *http.Request) {
	mmConfigUpdate(w, r, 1<<16, func(ctx context.Context, client *bisonw.WebClient, body []byte) error {
		return client.UpdateCEXConfig(ctx, body)
	})
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
	client, ok := dexWebSession(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := client.StartBot(ctx, body); err != nil {
		dexWriteErr(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// StopDcrdexMMBotHandler stops a running bot on the given market.
func StopDcrdexMMBotHandler(w http.ResponseWriter, r *http.Request) {
	mmMarketAction(w, r, func(ctx context.Context, client *bisonw.WebClient, host string, baseID, quoteID uint32) error {
		return client.StopBot(ctx, host, baseID, quoteID)
	})
}
