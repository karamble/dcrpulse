// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dcrpulse/internal/config"
	"dcrpulse/internal/types"
)

// WALLET-15: a client that omits a switch, such as a tab from before the
// exchange-rates switch existed, must not turn it off by saving.
func TestSaveKeepsExternalRequestSwitchesItOmits(t *testing.T) {
	var env types.SettingsEnvelope
	if err := json.Unmarshal([]byte(`{"global":{"externalRequests":{"vspListing":true,"politeia":false}}}`), &env); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"exchange_rates": true, "brseeder": false, "politeia": true}
	mergeExternalRequests(allowed, env.Global.ExternalRequests)
	want := map[string]bool{"exchange_rates": true, "brseeder": false, "politeia": false, "stakepool_listing": true}
	if !reflect.DeepEqual(allowed, want) {
		t.Fatalf("allowed = %v, want %v", allowed, want)
	}
}

// botSaveSetup stores a bot URL and Politeia off in a temporary global config,
// and records every bot health check, each of which fails like a bot that is
// down.
func botSaveSetup(t *testing.T, storedURL string) *[]string {
	t.Helper()
	prevPath, prevCheck := globalCfgPath, checkBotHealth
	path := filepath.Join(t.TempDir(), "config.json")
	globalCfgPath = func() string { return path }
	var probed []string
	checkBotHealth = func(_ context.Context, url string) error {
		probed = append(probed, url)
		return errors.New("could not reach a Decred Pulse bot: HTTP 502")
	}
	t.Cleanup(func() { globalCfgPath, checkBotHealth = prevPath, prevCheck })
	gc, err := config.LoadGlobalCfgAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := gc.Set(config.KeyDecredPulseBotURL, storedURL); err != nil {
		t.Fatal(err)
	}
	if err := gc.SetAllowedExternalRequests(map[string]bool{config.ExternalRequestPoliteia: false}); err != nil {
		t.Fatal(err)
	}
	if err := gc.Save(); err != nil {
		t.Fatal(err)
	}
	return &probed
}

func saveSettings(body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	SaveSettingsHandler(w, httptest.NewRequest(http.MethodPost, "/api/wallet/settings", strings.NewReader(body)))
	return w
}

// stored reads back the saved bot URL and Politeia switch.
func stored(t *testing.T) (string, bool) {
	t.Helper()
	gc, err := config.LoadGlobalCfgAt(globalCfgPath())
	if err != nil {
		t.Fatal(err)
	}
	var url string
	if _, err := gc.Get(config.KeyDecredPulseBotURL, &url); err != nil {
		t.Fatal(err)
	}
	allowed, err := gc.AllowedExternalRequests()
	if err != nil {
		t.Fatal(err)
	}
	return url, allowed[config.ExternalRequestPoliteia]
}

// A privacy switch is saved without contacting the bot, so a bot that is down
// cannot block it.
func TestSwitchSaveLeavesTheBotAlone(t *testing.T) {
	probed := botSaveSetup(t, "https://bot.example")
	w := saveSettings(`{"global":{"externalRequests":{"politeia":true}}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if len(*probed) != 0 {
		t.Fatalf("switch save contacted the bot at %v", *probed)
	}
	url, politeia := stored(t)
	if !politeia || url != "https://bot.example" {
		t.Fatalf("stored politeia=%v url=%q, want true and the old URL", politeia, url)
	}
}

// Saving a bot URL checks it first, and a bot that does not answer leaves
// everything as it was.
func TestBotURLSaveChecksTheBot(t *testing.T) {
	probed := botSaveSetup(t, "https://bot.example")
	w := saveSettings(`{"global":{"decredPulseBotUrl":"https://other.example/"}}`)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", w.Code)
	}
	if !reflect.DeepEqual(*probed, []string{"https://other.example"}) {
		t.Fatalf("probed %v, want the new URL once", *probed)
	}
	if url, _ := stored(t); url != "https://bot.example" {
		t.Fatalf("stored url %q after a failed check", url)
	}
}
