// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
	"dcrpulse/internal/utils"
)

// GetSettingsHandler returns the per-wallet + global settings envelope.
// Missing keys yield safe defaults so the UI always has something to
// render.
func GetSettingsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	network, _ := services.CurrentNetwork(ctx)
	walletName := services.CurrentWalletName()

	walletOut := types.WalletSettings{GapLimit: 20}
	globalOut := types.GlobalSettings{
		ExternalRequests: types.ExternalRequestSettings{
			VSPListing:    new(services.ExternalRequestAllowed(config.ExternalRequestVSPListing)),
			Politeia:      new(services.ExternalRequestAllowed(config.ExternalRequestPoliteia)),
			Brseeder:      new(services.ExternalRequestAllowed(config.ExternalRequestBrseeder)),
			ExchangeRates: new(services.ExternalRequestAllowed(config.ExternalRequestExchangeRates)),
		},
		DecredPulseBotURL: services.DefaultDecredPulseBotURL,
	}

	if network != "" {
		if wc, err := config.LoadWalletCfg(network, walletName); err == nil {
			var gap int
			if ok, _ := wc.Get(config.KeyGapLimit, &gap); ok && gap > 0 {
				walletOut.GapLimit = gap
			}
			var currency string
			if ok, _ := wc.Get("currency_display", &currency); ok {
				walletOut.CurrencyDisplay = currency
			}
		}
	}

	if gc, err := config.LoadGlobalCfg(); err == nil {
		var botURL string
		if ok, _ := gc.Get(config.KeyDecredPulseBotURL, &botURL); ok && strings.TrimSpace(botURL) != "" {
			globalOut.DecredPulseBotURL = strings.TrimRight(strings.TrimSpace(botURL), "/")
		}
	}

	writeJSON(w, types.SettingsEnvelope{
		Wallet: &walletOut,
		Global: &globalOut,
	})
}

// SaveSettingsHandler updates either or both subsections. Partial
// envelopes are accepted; unknown Decrediton keys in the underlying
// files are preserved by the WalletCfg/GlobalCfg layers.
func SaveSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var req types.SettingsEnvelope
	if !decodeRequest(w, r, &req) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if req.Wallet != nil {
		network, err := services.CurrentNetwork(ctx)
		if err != nil {
			settLog.Errorf("settings save: network lookup: %v", err)
			http.Error(w, "network not available", http.StatusServiceUnavailable)
			return
		}
		wc, err := config.LoadWalletCfg(network, services.CurrentWalletName())
		if err != nil {
			settLog.Errorf("settings save: load wallet cfg: %v", err)
			http.Error(w, "failed to load settings", http.StatusInternalServerError)
			return
		}
		if req.Wallet.GapLimit > 0 {
			if err := wc.Set(config.KeyGapLimit, req.Wallet.GapLimit); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if req.Wallet.CurrencyDisplay != "" {
			if err := wc.Set("currency_display", req.Wallet.CurrencyDisplay); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if err := wc.Save(); err != nil {
			settLog.Errorf("settings save: save wallet cfg: %v", err)
			http.Error(w, "failed to save settings", http.StatusInternalServerError)
			return
		}
	}

	result := types.SaveSettingsResult{NotApplied: []string{}}
	if req.Global != nil {
		ratesWere := services.ExchangeRatesEnabled()
		gc, err := config.LoadGlobalCfg()
		if err != nil {
			settLog.Errorf("settings save: load global cfg: %v", err)
			http.Error(w, "failed to load global settings", http.StatusInternalServerError)
			return
		}
		allowed, _ := gc.AllowedExternalRequests()
		if allowed == nil {
			allowed = map[string]bool{}
		}
		ext := req.Global.ExternalRequests
		mergeExternalRequests(allowed, ext)
		if err := gc.SetAllowedExternalRequests(allowed); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		botURL := strings.TrimRight(strings.TrimSpace(req.Global.DecredPulseBotURL), "/")
		if botURL != "" && !strings.HasPrefix(botURL, "http://") && !strings.HasPrefix(botURL, "https://") {
			http.Error(w, "Decred Pulse bot URL must start with http:// or https://", http.StatusBadRequest)
			return
		}
		botEnabled := true
		if v, ok := allowed[config.ExternalRequestDecredPulseBot]; ok {
			botEnabled = v
		}
		if botURL != "" && botEnabled {
			probeCtx, probeCancel := context.WithTimeout(r.Context(), 15*time.Second)
			err := services.CheckDecredPulseBotHealth(probeCtx, botURL)
			probeCancel()
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
		}
		if err := gc.Set(config.KeyDecredPulseBotURL, botURL); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := gc.Save(); err != nil {
			settLog.Errorf("settings save: save global cfg: %v", err)
			http.Error(w, "failed to save global settings", http.StatusInternalServerError)
			return
		}
		if ext.ExchangeRates != nil && *ext.ExchangeRates != ratesWere {
			applyCtx, applyCancel := context.WithTimeout(r.Context(), 30*time.Second)
			result.NotApplied = services.ApplyExchangeRates(applyCtx)
			applyCancel()
		}
	}

	writeJSON(w, result)
}

// mergeExternalRequests writes the switches a save names into allowed; an
// omitted switch keeps its stored value.
func mergeExternalRequests(allowed map[string]bool, ext types.ExternalRequestSettings) {
	for key, v := range map[string]*bool{
		config.ExternalRequestVSPListing:    ext.VSPListing,
		config.ExternalRequestPoliteia:      ext.Politeia,
		config.ExternalRequestBrseeder:      ext.Brseeder,
		config.ExternalRequestExchangeRates: ext.ExchangeRates,
	} {
		if v != nil {
			allowed[key] = *v
		}
	}
}

// lnSetUp and armMacaroonReset are the Lightning follow-up of a passphrase
// change, as variables so tests can observe it without a dcrlnd data dir.
var (
	lnSetUp          = services.LightningSetUp
	armMacaroonReset = services.RequestLnMacaroonReset
)

// ChangePassphraseHandler rotates the wallet's private passphrase.
func ChangePassphraseHandler(w http.ResponseWriter, r *http.Request) {
	var req types.ChangePassphraseRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	if req.NewPassphrase == "" {
		http.Error(w, "newPassphrase required", http.StatusBadRequest)
		return
	}
	if len(req.NewPassphrase) < 8 {
		http.Error(w, "new passphrase must be at least 8 characters", http.StatusBadRequest)
		return
	}
	if len(req.OldPassphrase) > 1024 || len(req.NewPassphrase) > 1024 || len(req.DexAppPass) > 1024 {
		http.Error(w, "passphrase too long", http.StatusBadRequest)
		return
	}

	oldPass := []byte(req.OldPassphrase)
	newPass := []byte(req.NewPassphrase)
	defer utils.Zero(oldPass)
	defer utils.Zero(newPass)

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Before anything irreversible: bisonw stores this passphrase too, and
	// handing it the new one needs a DCRDEX session, either the user's own or
	// one opened here with the supplied app password.
	didLogin, err := dexWalletPassphraseGate(ctx, req.DexAppPass)
	if err != nil {
		if errors.Is(err, ErrDexWrongAppPassword) {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if didLogin {
		defer dexPassphraseLogout()
	}

	err = services.ChangePrivatePassphrase(ctx, oldPass, newPass)
	var partial *services.PartialPassphraseChangeError
	switch {
	case err == nil:
	// Must precede the passphrase case: this message contains the word, and
	// reporting it as a wrong passphrase would send the user to retry with
	// the old one when the change has in fact already happened. The wallet
	// passphrase did change, so the steps below still run.
	case errors.As(err, &partial):
		settLog.Errorf("ChangePrivatePassphrase partial: %v", err)
	case isWrongPassphrase(err):
		http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
		return
	default:
		settLog.Errorf("ChangePrivatePassphrase failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// The passphrase has changed, so dcrlnd's macaroon store is now keyed to
	// a stale value. Arm the supervisor's reset before anything else can
	// fail: the next dcrlnd start drops the store and the next unlock
	// rebakes it under the new passphrase.
	if lnSetUp() {
		if err := armMacaroonReset(); err != nil {
			settLog.Errorf("ChangePrivatePassphrase: arm macaroon reset: %v", err)
			http.Error(w, "the wallet passphrase was changed, but the Lightning macaroon reset could not be requested; repeat the change with the new passphrase as both values to retry", http.StatusInternalServerError)
			return
		}
	}

	// The passphrase has already changed at this point, so a failure here is
	// reported rather than retried: bisonw is left holding the previous one.
	dexPass := req.NewPassphrase
	req.OldPassphrase, req.NewPassphrase = "", ""
	if err := syncDexWalletPassphrase(ctx, dexPass); err != nil {
		settLog.Errorf("ChangePrivatePassphrase: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if partial != nil {
		http.Error(w, partial.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetLogsHandler returns the tail of a daemon log file.
// Query params: component=dcrd|dcrwallet, lines=N (default 200, max 5000).
func GetLogsHandler(w http.ResponseWriter, r *http.Request) {
	component := r.URL.Query().Get("component")
	if component == "" {
		component = "dcrwallet"
	}
	linesStr := r.URL.Query().Get("lines")
	lines := 200
	if linesStr != "" {
		if n, err := strconv.Atoi(linesStr); err == nil && n > 0 {
			lines = n
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	out, err := services.TailLog(ctx, services.LogComponent(component), lines)
	if err != nil {
		// Reading our own log must not grow it on every failed poll.
		if services.LogComponent(component) == services.LogComponentDcrpulse {
			settLog.Debugf("TailLog(%s): %v", component, err)
		} else {
			settLog.Errorf("TailLog(%s): %v", component, err)
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"component": component,
		"lines":     out,
	})
}

// DiscoverAddressesHandler triggers a chain scan for previously-used
// addresses under the requested gap limit. Long-running.
func DiscoverAddressesHandler(w http.ResponseWriter, r *http.Request) {
	var req types.DiscoverUsageRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	if req.GapLimit == 0 {
		req.GapLimit = 20
	}
	if req.GapLimit > 10000 {
		http.Error(w, "gapLimit too large (max 10000)", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	if err := services.DiscoverUsage(ctx, req.GapLimit); err != nil {
		settLog.Errorf("DiscoverUsage failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Persist the gap limit as the modal's next prefill only once the scan it
	// described actually ran - a refused attempt must not change the setting.
	persistDiscoveryGap(r.Context(), int(req.GapLimit))

	// Discovery only marks addresses used; the transactions paying them are
	// fetched by a rescan, the same two-step every other discovery flow runs
	// (and Decrediton's discover-usage modal likewise rescans). Detached: the
	// progress surfaces through the existing rescan stream.
	go func() {
		time.Sleep(discoverRescanDelay) // let the wallet load its transaction filter
		discoverRescan(0)
	}()
	w.WriteHeader(http.StatusNoContent)
}

// The persist and rescan hand-offs are indirected so tests can observe their
// presence and ordering without the filter-load delay or a config volume.
var (
	discoverRescanDelay = 5 * time.Second
	discoverRescan      = startRescanViaGrpc
	persistDiscoveryGap = func(ctx context.Context, gap int) {
		network, err := services.CurrentNetwork(ctx)
		if err != nil {
			return
		}
		if wc, err := config.LoadWalletCfg(network, services.CurrentWalletName()); err == nil {
			_ = wc.Set(config.KeyGapLimit, gap)
			_ = wc.Save()
		}
	}
)
