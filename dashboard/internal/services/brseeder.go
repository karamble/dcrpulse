// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/types"
)

// BrseederEnabled reports whether the user has the brseeder external-
// request toggle on. Defaults to true when no global config is present.
func BrseederEnabled() bool {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return true
	}
	allowed, _ := gc.AllowedExternalRequests()
	if allowed == nil {
		return true
	}
	v, ok := allowed[config.ExternalRequestBrseeder]
	if !ok {
		return true
	}
	return v
}

// Bison Relay publishes a JSON list of its brserver+LND endpoints at
// https://bisonrelay.org/api/live. The schema is documented in
// github.com/companyzero/bisonrelay/rpc/seeder.go and the response is
// stable in shape. We use the `lnd` field as a curated peer preset for
// the open-channel form. Mirrors `brseeder/client/seederclient.go`.
const (
	brseederURL          = "https://bisonrelay.org/api/live"
	brseederRefreshEvery = 5 * time.Minute
	brseederTimeout      = 8 * time.Second
)

// brseederFallback is the verbatim mainnet onboarding peer from
// Bison Relay's client_onboard.go:210. Used when the brseeder query
// fails on first try so the open-channel form is never empty.
var brseederFallback = []types.PeerPreset{{
	Label:      "hub0.bisonrelay.org",
	URI:        "03bd03386d7b2efe80ae46d6c8cfcfdfcf9c9297a465ac0d48c110d11ae58ed509@hub0.bisonrelay.org:9735",
	IsFallback: true,
}}

type seederResponse struct {
	ServerGroups []struct {
		BrServer string `json:"brserver"`
		LND      string `json:"lnd"`
		IsMaster bool   `json:"isMaster"`
		Online   bool   `json:"online"`
	} `json:"serverGroups"`
}

var (
	brseederMu        sync.RWMutex
	brseederCache     []types.PeerPreset
	brseederStartOnce sync.Once
)

// StartBrseederRefresh kicks off a background goroutine that keeps the
// peer-preset cache fresh. Safe to call multiple times; subsequent calls
// no-op. Called from main.go at startup.
func StartBrseederRefresh() {
	brseederStartOnce.Do(func() {
		go brseederLoop()
	})
}

func brseederLoop() {
	refreshBrseederOnce()
	ticker := time.NewTicker(brseederRefreshEvery)
	defer ticker.Stop()
	for range ticker.C {
		refreshBrseederOnce()
	}
}

func refreshBrseederOnce() {
	if !BrseederEnabled() {
		// User has opted out of the outbound bisonrelay.org query.
		// Clear any previously-cached entries so the presets list
		// degrades to the hardcoded hub0 fallback only.
		brseederMu.Lock()
		brseederCache = nil
		brseederMu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), brseederTimeout)
	defer cancel()

	presets, err := fetchBrseeder(ctx)
	if err != nil {
		brseederMu.RLock()
		empty := len(brseederCache) == 0
		brseederMu.RUnlock()
		if empty {
			brseederMu.Lock()
			brseederCache = append([]types.PeerPreset{}, brseederFallback...)
			brseederMu.Unlock()
			log.Printf("brseeder unreachable on first fetch, using hardcoded fallback: %v", err)
		} else {
			log.Printf("brseeder refresh failed (keeping previous cache): %v", err)
		}
		return
	}

	brseederMu.Lock()
	brseederCache = presets
	brseederMu.Unlock()
}

func fetchBrseeder(ctx context.Context) ([]types.PeerPreset, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, brseederURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := externalHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brseeder status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	var raw seederResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode brseeder: %w", err)
	}

	out := make([]types.PeerPreset, 0, len(raw.ServerGroups))
	for _, sg := range raw.ServerGroups {
		if !sg.IsMaster || !sg.Online || sg.LND == "" {
			continue
		}
		// Drop the ":443" suffix on the label — that's the brserver TLS
		// port, irrelevant for an LN-peer label.
		label := sg.BrServer
		for i := len(label) - 1; i >= 0; i-- {
			if label[i] == ':' {
				label = label[:i]
				break
			}
		}
		out = append(out, types.PeerPreset{Label: label, URI: sg.LND})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("brseeder returned 0 usable entries")
	}
	return out, nil
}

// LightningPeerPresets returns the current cached set of peer presets
// from Bison Relay's brseeder, with the hardcoded hub0 fallback appended
// at the end (deduplicated by URI). If the user has disabled brseeder
// under Settings → Privacy, returns an empty list so the open-channel
// form shows no presets — the user can still type a manual peer URI.
func LightningPeerPresets(_ context.Context) []types.PeerPreset {
	if !BrseederEnabled() {
		return []types.PeerPreset{}
	}
	brseederMu.RLock()
	cached := append([]types.PeerPreset{}, brseederCache...)
	brseederMu.RUnlock()

	// Ensure the fallback is always reachable as a last-resort option.
	seen := map[string]struct{}{}
	for _, p := range cached {
		seen[p.URI] = struct{}{}
	}
	for _, fb := range brseederFallback {
		if _, ok := seen[fb.URI]; ok {
			continue
		}
		cached = append(cached, fb)
	}
	return cached
}
