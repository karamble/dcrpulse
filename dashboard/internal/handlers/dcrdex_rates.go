// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"dcrpulse/internal/services"
)

// The rate-source steps the routes take; variables so tests can stand in.
var (
	dexRateSourceChoice  = services.DexRateSourceChoice
	saveDexRateSources   = services.SaveDexRateSources
	applyDexRateSources  = services.ApplyDexRateSources
	exchangeRatesEnabled = func() bool { return services.ExchangeRatesEnabled() }
)

type dexRateSource struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type dexRateSourcesReply struct {
	ExchangeRates bool            `json:"exchangeRates"`
	Sources       []dexRateSource `json:"sources"`
}

// GetDcrdexRateSourcesHandler reports the active wallet's DCRDEX price-source
// choice and whether exchange rates are on at all.
func GetDcrdexRateSourcesHandler(w http.ResponseWriter, r *http.Request) {
	choice, err := dexRateSourceChoice(r.Context())
	if err != nil {
		http.Error(w, "failed to load rate sources", http.StatusInternalServerError)
		return
	}
	reply := dexRateSourcesReply{ExchangeRates: exchangeRatesEnabled(), Sources: []dexRateSource{}}
	for _, name := range services.DexRateSources {
		reply.Sources = append(reply.Sources, dexRateSource{Name: name, Enabled: choice[name]})
	}
	writeJSON(w, reply)
}

// SaveDcrdexRateSourcesHandler stores the named sources' choice and hands it to
// DCRDEX; applied is false while DCRDEX is locked or not running.
func SaveDcrdexRateSourcesHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sources map[string]bool `json:"sources"`
	}
	if !decodeRequest(w, r, &req) {
		return
	}
	if len(req.Sources) == 0 {
		http.Error(w, "sources is required", http.StatusBadRequest)
		return
	}
	if err := saveDexRateSources(r.Context(), req.Sources); err != nil {
		if errors.Is(err, services.ErrUnknownDexRateSource) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dexcLog.Errorf("save rate sources: %v", err)
		http.Error(w, "failed to save rate sources", http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	applied := true
	if err := applyDexRateSources(ctx); err != nil {
		dexcLog.Warnf("Rate sources not applied to DCRDEX: %v", err)
		applied = false
	}
	writeJSON(w, map[string]bool{"applied": applied})
}
