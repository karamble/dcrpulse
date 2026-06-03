// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"dcrpulse/internal/config"
	"dcrpulse/internal/types"
)

const (
	themeSchemaVersion = 1
	maxCustomThemes    = 50
)

// GetThemesHandler returns the persisted theme store: the active theme
// selection plus any user-created themes. When nothing has been saved yet
// it returns a default pointing at the built-in "pulse" theme.
func GetThemesHandler(w http.ResponseWriter, r *http.Request) {
	store := types.ThemeStore{
		Schema:        themeSchemaVersion,
		ActiveThemeID: "pulse",
		CustomThemes:  []json.RawMessage{},
	}
	if gc, err := config.LoadGlobalCfg(); err == nil {
		var saved types.ThemeStore
		if ok, err := gc.Get(config.KeyThemeStore, &saved); ok && err == nil {
			if saved.Schema != 0 {
				store.Schema = saved.Schema
			}
			if saved.ActiveThemeID != "" {
				store.ActiveThemeID = saved.ActiveThemeID
			}
			if saved.CustomThemes != nil {
				store.CustomThemes = saved.CustomThemes
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(store)
}

// SaveThemesHandler persists the theme store. Custom themes are stored
// verbatim as opaque JSON; validation of the theme schema lives in the
// frontend. The body size is bounded by the global JSON-body limit.
func SaveThemesHandler(w http.ResponseWriter, r *http.Request) {
	var req types.ThemeStore
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Schema != themeSchemaVersion {
		http.Error(w, "unsupported theme schema", http.StatusBadRequest)
		return
	}
	if len(req.CustomThemes) > maxCustomThemes {
		http.Error(w, "too many custom themes", http.StatusBadRequest)
		return
	}
	if req.CustomThemes == nil {
		req.CustomThemes = []json.RawMessage{}
	}

	gc, err := config.LoadGlobalCfg()
	if err != nil {
		log.Printf("themes save: load global cfg: %v", err)
		http.Error(w, "failed to load themes", http.StatusInternalServerError)
		return
	}
	if err := gc.Set(config.KeyThemeStore, req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := gc.Save(); err != nil {
		log.Printf("themes save: save global cfg: %v", err)
		http.Error(w, "failed to save themes", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
