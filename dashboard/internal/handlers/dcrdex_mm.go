// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/services"
	"dcrpulse/pkg/bisonw"
)

// The market-maker handlers proxy bisonw's webserver MM API, which persists bot
// and CEX configuration to mm_cfg.json in the daemon's appdata. Each call rides
// the webserver cookie session established at unlock.
//
// The routes that reconfigure or refund an already-running bot live on bisonw's
// RPC server instead. Those take a config file by its path on the daemon's own
// filesystem: read-only queries name bisonw's own mm_cfg.json, and a config
// push stages its own file in the shared control directory (see stageMMConfig).

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

// mmConfigPath names bisonw's market-maker config file for the active wallet
// and network as the daemon sees it, after confirming through the dashboard's
// read-only mount of the same volume that the file is there. That turns a
// wrong path into a clear answer here instead of an opaque daemon error, and
// pins the assumption that bisonw is using its default location.
func mmConfigPath(ctx context.Context) (string, error) {
	network, err := services.CurrentNetwork(ctx)
	if err != nil || network == "" {
		network = "mainnet"
	}
	wallet := services.CurrentWalletName()
	local := config.DcrdexMMConfigLocalPath(wallet, network)
	if _, err := os.Stat(local); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no market-maker config saved yet (%s); save a bot config first", local)
		}
		return "", err
	}
	return config.DcrdexMMConfigPath(wallet, network), nil
}

// GetDcrdexMMAvailableBalancesHandler reports what a bot on this market may
// still allocate, in atoms keyed by asset id. Read-only, and the only one of
// the three running-bot routes that is safe to call speculatively.
func GetDcrdexMMAvailableBalancesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	host := q.Get("host")
	baseID, err1 := strconv.ParseUint(q.Get("baseID"), 10, 32)
	quoteID, err2 := strconv.ParseUint(q.Get("quoteID"), 10, 32)
	if host == "" || err1 != nil || err2 != nil {
		http.Error(w, "host, baseID and quoteID are required", http.StatusBadRequest)
		return
	}
	client, ok := dexUnlockedClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cfgPath, err := mmConfigPath(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	bals, err := client.MMAvailableBalances(ctx, cfgPath, host, uint32(baseID), uint32(quoteID))
	if err != nil {
		dexWriteErr(w, err)
		return
	}
	json.NewEncoder(w).Encode(bals)
}

// mmRunningBotUpdate decodes a running-bot request: the market plus the signed
// per-asset atom deltas both update routes can carry. Config carries a bisonw
// mm.BotConfig verbatim on the config route and is unused on the inventory one.
type mmRunningBotUpdate struct {
	Host     string           `json:"host"`
	BaseID   uint32           `json:"baseID"`
	QuoteID  uint32           `json:"quoteID"`
	DexDiffs map[uint32]int64 `json:"dexDiffs"`
	CexDiffs map[uint32]int64 `json:"cexDiffs"`
	Config   json.RawMessage  `json:"config"`
}

func decodeRunningBotUpdate(w http.ResponseWriter, r *http.Request) (*mmRunningBotUpdate, bool) {
	var req mmRunningBotUpdate
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || req.Host == "" {
		http.Error(w, "host is required", http.StatusBadRequest)
		return nil, false
	}
	return &req, true
}

// stageMMConfig writes one bot config where bisonw can read it back and returns
// that path plus a cleanup. The RPC route takes a file rather than a config,
// and the webserver route that writes bisonw's own mm_cfg.json refuses while
// the bot is running, so the config goes through the shared control directory
// instead of through the daemon's file. Only the named bot is written: the
// route looks up its market and ignores the rest.
func stageMMConfig(cfg json.RawMessage) (path string, cleanup func(), err error) {
	if err := os.MkdirAll(config.DcrdexMMUpdateDir(), 0o755); err != nil {
		return "", nil, err
	}
	f, err := os.CreateTemp(config.DcrdexMMUpdateDir(), "dcrdex-mm-*.json")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.Remove(f.Name()) }
	// bisonw reads this through a read-only mount, so it has to be world
	// readable; the file holds bot settings only, never keys or credentials.
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		cleanup()
		return "", nil, err
	}
	body, err := json.Marshal(struct {
		BotConfigs []json.RawMessage `json:"botConfigs"`
	}{BotConfigs: []json.RawMessage{cfg}})
	if err == nil {
		_, err = f.Write(body)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return f.Name(), cleanup, nil
}

// UpdateDcrdexMMRunningBotCfgHandler applies a config to a bot that is already
// running, so a change takes effect without the stop/start cycle that would
// cancel its book.
//
// The change is live only. bisonw's UpdateRunningBotCfg takes a saveUpdate flag
// its body never reads, and the webserver route that does persist refuses while
// the bot is running, so the stored config is unchanged until the bot is
// stopped and saved again.
func UpdateDcrdexMMRunningBotCfgHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	req, ok := decodeRunningBotUpdate(w, r)
	if !ok {
		return
	}
	if len(req.Config) == 0 {
		http.Error(w, "config is required", http.StatusBadRequest)
		return
	}
	client, ok := dexUnlockedClient(w)
	if !ok {
		return
	}
	cfgPath, cleanup, err := stageMMConfig(req.Config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := client.UpdateRunningBotCfg(ctx, cfgPath, req.Host, req.BaseID, req.QuoteID,
		req.DexDiffs, req.CexDiffs); err != nil {
		dexWriteErr(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// UpdateDcrdexMMRunningBotInventoryHandler moves funds between a running bot's
// allocation and the wallet, leaving its config alone. This spends real funds;
// the frontend gates it behind an explicit confirmation.
func UpdateDcrdexMMRunningBotInventoryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	req, ok := decodeRunningBotUpdate(w, r)
	if !ok {
		return
	}
	if len(req.DexDiffs) == 0 && len(req.CexDiffs) == 0 {
		http.Error(w, "at least one balance change is required", http.StatusBadRequest)
		return
	}
	client, ok := dexUnlockedClient(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := client.UpdateRunningBotInventory(ctx, req.Host, req.BaseID, req.QuoteID,
		req.DexDiffs, req.CexDiffs); err != nil {
		dexWriteErr(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
