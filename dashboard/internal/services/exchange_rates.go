// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
)

// ErrExchangeRatesOff is returned instead of a price while exchange rates are
// turned off.
var ErrExchangeRatesOff = errors.New("exchange rates are turned off in Settings")

// dexRateSources are bisonw's fiat rate sources, named exactly as DCRDEX's
// client/core/exchangeratefetcher.go names them.
var dexRateSources = []string{"Messari", "Coinpaprika", "dcrdata"}

// ExchangeRatesEnabled reports whether the exchange-rates external-request
// toggle is on. Defaults to true when no global config is present yet. A
// variable so tests can turn it off.
var ExchangeRatesEnabled = func() bool {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return true
	}
	allowed, _ := gc.AllowedExternalRequests()
	v, ok := allowed[config.ExternalRequestExchangeRates]
	if !ok {
		return true
	}
	return v
}

// The daemons the setting is applied to; variables so tests can stand in.
var (
	setBrclientdExchangeRates = func(ctx context.Context, on bool) error {
		return rpc.BrclientdSetBRBehavior(ctx, map[string]bool{"exchangeRates": on})
	}
	setDexRateSource = func(ctx context.Context, source string, disable bool) error {
		web, err := rpc.DcrdexWebClient()
		if err != nil {
			return err
		}
		return web.ToggleRateSource(ctx, source, disable)
	}
)

// ApplyExchangeRates tells brclientd and bisonw whether to fetch exchange
// rates, and names the ones it could not tell: brclientd when it is not
// running, dcrdex while it is locked or not running.
func ApplyExchangeRates(ctx context.Context) []string {
	on := ExchangeRatesEnabled()
	var missed []string
	if err := setBrclientdExchangeRates(ctx, on); err != nil {
		settLog.Warnf("Exchange rates not applied to Bison Relay: %v", err)
		missed = append(missed, "brclientd")
	}
	for _, source := range dexRateSources {
		if err := setDexRateSource(ctx, source, !on); err != nil {
			settLog.Warnf("Exchange rates not applied to DCRDEX (%s): %v", source, err)
			missed = append(missed, "dcrdex")
			break
		}
	}
	return missed
}
