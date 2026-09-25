// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"dcrpulse/internal/services"

	"github.com/decred/dcrd/dcrutil/v4"
)

// GetTreasuryInfoHandler returns current treasury status
func GetTreasuryInfoHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	info, err := services.FetchTreasuryInfo(ctx)
	if err != nil {
		govnLog.Errorf("Error fetching treasury info: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, info)
}

// GetTreasuryBalanceHistoryHandler returns the treasury balance-over-time
// series (the first block of every UTC month plus the tip, cached in-process).
func GetTreasuryBalanceHistoryHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	series, err := services.TreasuryBalanceHistory(ctx)
	if err != nil {
		govnLog.Errorf("Error fetching treasury balance history: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, series)
}

// GetTreasurySpendLimitHandler returns dcrd's DCP-0013 treasury spend limit
// for a TVI block following the tip.
func GetTreasurySpendLimitHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	limit, err := services.TreasurySpendLimit(ctx)
	if err != nil {
		govnLog.Errorf("Error computing treasury spend limit: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, limit)
}

// GetTreasuryOutlookHandler returns the projected treasury block reward for
// the next twelve months.
func GetTreasuryOutlookHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	outlook, err := services.TreasuryOutlook(ctx)
	if err != nil {
		govnLog.Errorf("Error computing treasury outlook: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, outlook)
}

// GetTreasuryRunwayHandler returns how long the treasury balance lasts at the
// monthly spend in monthlySpendAtoms.
func GetTreasuryRunwayHandler(w http.ResponseWriter, r *http.Request) {
	spend, err := strconv.ParseInt(r.URL.Query().Get("monthlySpendAtoms"), 10, 64)
	if err != nil || spend <= 0 || spend > dcrutil.MaxAmount {
		http.Error(w, "monthlySpendAtoms must be a positive amount of atoms", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	runway, err := services.TreasuryRunway(ctx, spend)
	if err != nil {
		govnLog.Errorf("Error computing treasury runway: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, runway)
}

// TriggerTSpendScanHandler triggers a historical blockchain scan for TSpends
func TriggerTSpendScanHandler(w http.ResponseWriter, r *http.Request) {
	// Parse request body to get startHeight (optional)
	var req struct {
		StartHeight int64 `json:"startHeight"`
	}

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid scan request: startHeight must be an int64", http.StatusBadRequest)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid scan request: expected one JSON object", http.StatusBadRequest)
		return
	}
	if req.StartHeight < services.TreasuryActivationHeight {
		req.StartHeight = services.TreasuryActivationHeight
	}

	err := services.TriggerHistoricalScan(r.Context(), req.StartHeight)
	if err != nil {
		govnLog.Errorf("Error triggering TSpend scan: %v", err)
		status := http.StatusInternalServerError
		if errors.Is(err, services.ErrInvalidScanHeight) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Historical TSpend scan started from block %d", req.StartHeight),
	})
}

// GetTSpendScanProgressHandler returns the current scan progress
func GetTSpendScanProgressHandler(w http.ResponseWriter, r *http.Request) {
	progress, err := services.GetScanProgress()
	if err != nil {
		govnLog.Errorf("Error getting scan progress: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, progress)
}

// GetTSpendScanResultsHandler returns the results from the last completed scan
func GetTSpendScanResultsHandler(w http.ResponseWriter, r *http.Request) {
	results := services.GetScanResults()

	writeJSON(w, results)
}
