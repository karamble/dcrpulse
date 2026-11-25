// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
)

// ErrExchangeRatesOff is returned instead of a price while exchange rates are
// turned off.
var ErrExchangeRatesOff = errors.New("exchange rates are turned off in Settings")

// ErrUnknownDexRateSource refuses a rate source the user cannot choose.
var ErrUnknownDexRateSource = errors.New("unknown DCRDEX rate source")

// DexRateSources are the bisonw fiat rate sources a wallet can choose from,
// named exactly as DCRDEX's client/core/exchangeratefetcher.go names them.
var DexRateSources = []string{"Coinpaprika", "dcrdata"}

// dexRetiredRateSource no longer has a free API, so bisonw is always told to
// leave it alone; DCRDEX v1.1 no longer knows the name.
const dexRetiredRateSource = "Messari"

// ExchangeRatesEnabled reports whether the exchange-rates external-request
// toggle is on. A variable so tests can turn it off.
var ExchangeRatesEnabled = func() bool {
	return ExternalRequestAllowed(config.ExternalRequestExchangeRates)
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

// The active wallet's stored rate-source choices; variables so tests can stand in.
var (
	readDexRateSources = func(ctx context.Context) (map[string]bool, error) {
		wc, err := activeWalletCfg(ctx)
		if err != nil {
			return nil, err
		}
		return wc.DexRateSources()
	}
	writeDexRateSources = func(ctx context.Context, update map[string]bool) error {
		wc, err := activeWalletCfg(ctx)
		if err != nil {
			return err
		}
		for source, on := range update {
			if err := wc.SetDexRateSource(source, on); err != nil {
				return err
			}
		}
		return wc.Save()
	}
)

func activeWalletCfg(ctx context.Context) (*config.WalletCfg, error) {
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return nil, err
	}
	return config.LoadWalletCfg(network, CurrentWalletName())
}

// DexRateSourceChoice reports, for every choosable source, whether the active
// wallet's DCRDEX may ask it.
func DexRateSourceChoice(ctx context.Context) (map[string]bool, error) {
	stored, err := readDexRateSources(ctx)
	if err != nil {
		return nil, err
	}
	choice := make(map[string]bool, len(DexRateSources))
	for _, source := range DexRateSources {
		on, ok := stored[source]
		choice[source] = !ok || on
	}
	return choice, nil
}

// SaveDexRateSources stores the active wallet's choice for the named sources
// and keeps the others.
func SaveDexRateSources(ctx context.Context, update map[string]bool) error {
	for source := range update {
		if !slices.Contains(DexRateSources, source) {
			return fmt.Errorf("%w: %q", ErrUnknownDexRateSource, source)
		}
	}
	return writeDexRateSources(ctx, update)
}

// ApplyDexRateSources tells the active wallet's DCRDEX which rate sources to
// ask: the chosen ones while exchange rates are on, none while they are off.
func ApplyDexRateSources(ctx context.Context) error {
	return applyDexRateSources(ctx, ExchangeRatesEnabled())
}

func applyDexRateSources(ctx context.Context, on bool) error {
	var choice map[string]bool
	if on {
		var err error
		if choice, err = DexRateSourceChoice(ctx); err != nil {
			return fmt.Errorf("read rate sources: %w", err)
		}
	}
	for _, source := range DexRateSources {
		if err := setDexRateSource(ctx, source, !(on && choice[source])); err != nil {
			return fmt.Errorf("%s: %w", source, err)
		}
	}
	err := setDexRateSource(ctx, dexRetiredRateSource, true)
	if err != nil && !strings.Contains(err.Error(), "unknown fiat rate source") {
		return fmt.Errorf("%s: %w", dexRetiredRateSource, err)
	}
	return nil
}

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
	if err := applyDexRateSources(ctx, on); err != nil {
		settLog.Warnf("Exchange rates not applied to DCRDEX: %v", err)
		missed = append(missed, "dcrdex")
	}
	return missed
}
