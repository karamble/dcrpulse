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
	"time"

	"dcrpulse/internal/services"
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
// series (sampled at ~monthly cadence, cached in-process).
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
