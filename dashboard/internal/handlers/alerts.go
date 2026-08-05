// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"dcrpulse/internal/alerts"

	"github.com/gorilla/mux"
)

// AlertsSummaryHandler serves the pill payload. Deliberately its own endpoint
// rather than a field on /api/wallet/status: that handler fails with plain-text
// errors while dcrd is down or in IBD, which is exactly when critical alerts
// fire.
func AlertsSummaryHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, alerts.Summary())
}

// ListAlertsHandler returns every entry, newest first. The ring is capped at
// a few hundred entries, so filtering stays client-side.
func ListAlertsHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, struct {
		Entries []alerts.Entry `json:"entries"`
	}{Entries: alerts.List()})
}

// MarkAlertReadHandler marks a single entry read.
func MarkAlertReadHandler(w http.ResponseWriter, r *http.Request) {
	if err := alerts.MarkRead(mux.Vars(r)["id"]); err != nil {
		if errors.Is(err, alerts.ErrNotFound) {
			http.Error(w, "alert not found", http.StatusNotFound)
			return
		}
		log.Printf("alerts: mark read: %v", err)
		http.Error(w, "failed to update alert", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MarkAllAlertsReadHandler marks every entry read.
func MarkAllAlertsReadHandler(w http.ResponseWriter, r *http.Request) {
	alerts.MarkAllRead()
	w.WriteHeader(http.StatusNoContent)
}

// GetAlertsSettingsHandler returns the per-category opt-outs, defaults-filled.
func GetAlertsSettingsHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, alerts.Settings())
}

// SaveAlertsSettingsHandler persists the per-category opt-outs. Disabled
// categories generate nothing at the source; disabling also resolves that
// category's active conditions.
func SaveAlertsSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var req alerts.SettingsData
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := alerts.UpdateSettings(req); err != nil {
		if errors.Is(err, alerts.ErrBadSettings) {
			http.Error(w, "invalid alerts settings", http.StatusBadRequest)
			return
		}
		log.Printf("alerts: save settings: %v", err)
		http.Error(w, "failed to save alerts settings", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
