// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// The Bison Relay gaming section's confinement policy (Bison Relay > Gaming).
// Games are untrusted plugins running beside a wallet, dcrlnd and a BR
// identity, so they never hold wallet credentials: they reach one account,
// under caps, through this policy. Storage speaks atoms; the frontend speaks
// DCR, converted here the same way the BR-MCP handlers do.

// gamingSettingsView is the DCR-denominated frontend shape.
type gamingSettingsView struct {
	Enabled             bool     `json:"enabled"`
	Account             string   `json:"account"`
	Mode                string   `json:"mode"`
	PerTableCapDcr      float64  `json:"perTableCapDcr"`
	PerDayCapDcr        float64  `json:"perDayCapDcr"`
	MaxOpenTables       int      `json:"maxOpenTables"`
	ApprovalTimeoutSecs int      `json:"approvalTimeoutSecs"`
	InstalledGames      []string `json:"installedGames"`
}

func gamingToView(s types.GamingSettings) gamingSettingsView {
	if s.InstalledGames == nil {
		s.InstalledGames = []string{}
	}
	return gamingSettingsView{
		Enabled:             s.Enabled,
		Account:             s.Account,
		Mode:                s.Mode,
		PerTableCapDcr:      dcrutil.Amount(s.PerTableCapAtoms).ToCoin(),
		PerDayCapDcr:        dcrutil.Amount(s.PerDayCapAtoms).ToCoin(),
		MaxOpenTables:       s.MaxOpenTables,
		ApprovalTimeoutSecs: s.ApprovalTimeoutSecs,
		InstalledGames:      s.InstalledGames,
	}
}

func gamingFromView(v gamingSettingsView) (types.GamingSettings, error) {
	perTable, err := dcrutil.NewAmount(v.PerTableCapDcr)
	if err != nil {
		return types.GamingSettings{}, err
	}
	perDay, err := dcrutil.NewAmount(v.PerDayCapDcr)
	if err != nil {
		return types.GamingSettings{}, err
	}
	return types.GamingSettings{
		Enabled:             v.Enabled,
		Account:             v.Account,
		Mode:                v.Mode,
		PerTableCapAtoms:    int64(perTable),
		PerDayCapAtoms:      int64(perDay),
		MaxOpenTables:       v.MaxOpenTables,
		ApprovalTimeoutSecs: v.ApprovalTimeoutSecs,
		InstalledGames:      v.InstalledGames,
	}, nil
}

func gamingJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// BisonrelayGamingSettingsHandler round-trips the gaming confinement policy.
func BisonrelayGamingSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		gamingJSON(w, gamingToView(services.ReadGamingSettings()))
	case http.MethodPost:
		var in gamingSettingsView
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		next, err := gamingFromView(in)
		if err != nil {
			http.Error(w, "bad amount: "+err.Error(), http.StatusBadRequest)
			return
		}
		saved, err := services.WriteGamingSettings(next)
		if err != nil {
			http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		gamingJSON(w, gamingToView(saved))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// BisonrelayGamingGamesHandler lists the games this build can route, marked
// with whether the user installed them.
func BisonrelayGamingGamesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	gamingJSON(w, map[string]any{"games": services.GamingCatalogue()})
}
