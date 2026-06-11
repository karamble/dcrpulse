// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package exchangerate provides USD prices for crypto assets, with a TTL cache.
// It mirrors the public price sources DCRDEX's own fiat oracle uses, without
// pulling in that package's dependencies: CryptoCompare is the primary source
// (broad coverage, one call) and Kraken is a fallback (direct USD pair, or BTC
// pair converted via BTC/USD). The package is dependency-free (standard library
// only) so it can be reused.
package exchangerate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cryptoCompareURL = "https://min-api.cryptocompare.com/data/pricemulti"
	krakenTickerURL  = "https://api.kraken.com/0/public/Ticker"
	btcUSDResultKey  = "XXBTZUSD"
)

// asset maps a dcrpulse/DCRDEX asset symbol to its ticker on each source:
// cc is the CryptoCompare fsym; krUSD*/krBTC* are Kraken's request name and
// canonical result key for the USD pair and the BTC-conversion fallback pair.
type asset struct {
	symbol           string
	cc               string
	krUSDReq, krUSDKey string
	krBTCReq, krBTCKey string
}

var assets = []asset{
	{"dcr", "DCR", "DCRUSD", "DCRUSD", "", ""},
	{"btc", "BTC", "XBTUSD", "XXBTZUSD", "", ""},
	{"eth", "ETH", "ETHUSD", "XETHZUSD", "ETHXBT", "XETHXXBT"},
	{"ltc", "LTC", "LTCUSD", "XLTCZUSD", "LTCXBT", "XLTCXXBT"},
	{"bch", "BCH", "BCHUSD", "BCHUSD", "", ""},
	{"dash", "DASH", "DASHUSD", "DASHUSD", "", ""},
	{"doge", "DOGE", "XDGUSD", "XDGUSD", "XDGXBT", "XXDGXXBT"},
	{"zec", "ZEC", "ZECUSD", "XZECZUSD", "", ""},
	{"polygon", "POL", "POLUSD", "POLUSD", "", ""},
	{"usdc", "USDC", "USDCUSD", "USDCUSD", "", ""},
	{"usdt", "USDT", "USDTUSD", "USDTZUSD", "", ""},
	{"dgb", "DGB", "", "", "", ""},
	{"firo", "FIRO", "", "", "", ""},
}

// Cache fetches and caches USD rates.
type Cache struct {
	http *http.Client
	ttl  time.Duration

	mu   sync.Mutex
	at   time.Time
	data map[string]float64
}

// New returns a Cache with a 60s TTL.
func New() *Cache {
	return &Cache{http: &http.Client{Timeout: 15 * time.Second}, ttl: 60 * time.Second}
}

// USD returns a map of asset symbol to USD price, cached for the TTL. It uses
// CryptoCompare first and fills any gaps (or falls back entirely) from Kraken.
// On total failure a previously cached (stale) result is returned if available.
func (c *Cache) USD(ctx context.Context) (map[string]float64, error) {
	c.mu.Lock()
	if c.data != nil && time.Since(c.at) < c.ttl {
		d := c.data
		c.mu.Unlock()
		return d, nil
	}
	c.mu.Unlock()

	out := map[string]float64{}
	if cc, err := c.fetchCryptoCompare(ctx); err == nil {
		for k, v := range cc {
			out[k] = v
		}
	}

	if needKraken(out) {
		if kr, err := c.fetchKraken(ctx); err == nil {
			for k, v := range kr {
				if _, ok := out[k]; !ok {
					out[k] = v
				}
			}
		}
	}

	if len(out) == 0 {
		c.mu.Lock()
		stale := c.data
		c.mu.Unlock()
		if stale != nil {
			return stale, nil
		}
		return nil, fmt.Errorf("exchangerate: no rates available")
	}

	c.mu.Lock()
	c.data = out
	c.at = time.Now()
	c.mu.Unlock()
	return out, nil
}

// needKraken reports whether Kraken should be consulted: nothing yet, or a
// Kraken-listed asset is still missing a rate.
func needKraken(out map[string]float64) bool {
	if len(out) == 0 {
		return true
	}
	for _, a := range assets {
		if a.krUSDKey == "" {
			continue
		}
		if _, ok := out[a.symbol]; !ok {
			return true
		}
	}
	return false
}

func (c *Cache) get(ctx context.Context, rawURL string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

func (c *Cache) fetchCryptoCompare(ctx context.Context) (map[string]float64, error) {
	fsyms := make([]string, 0, len(assets))
	byCC := make(map[string]string, len(assets))
	for _, a := range assets {
		if a.cc == "" {
			continue
		}
		fsyms = append(fsyms, a.cc)
		byCC[a.cc] = a.symbol
	}
	u := cryptoCompareURL + "?" + url.Values{"fsyms": {strings.Join(fsyms, ",")}, "tsyms": {"USD"}}.Encode()
	// CryptoCompare returns {"DCR":{"USD":1.2},...}, or an error object with a
	// string "Response" field that fails to decode into this shape.
	var res map[string]map[string]float64
	if err := c.get(ctx, u, &res); err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(res))
	for cc, quote := range res {
		if sym, ok := byCC[cc]; ok && quote["USD"] > 0 {
			out[sym] = quote["USD"]
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cryptocompare: no rates")
	}
	return out, nil
}

func (c *Cache) fetchKraken(ctx context.Context) (map[string]float64, error) {
	reqSet := map[string]struct{}{"XBTUSD": {}}
	for _, a := range assets {
		if a.krUSDReq != "" {
			reqSet[a.krUSDReq] = struct{}{}
		}
		if a.krBTCReq != "" {
			reqSet[a.krBTCReq] = struct{}{}
		}
	}
	reqs := make([]string, 0, len(reqSet))
	for r := range reqSet {
		reqs = append(reqs, r)
	}
	u := krakenTickerURL + "?" + url.Values{"pair": {strings.Join(reqs, ",")}}.Encode()
	var kr struct {
		Error  []string `json:"error"`
		Result map[string]struct {
			C []string `json:"c"`
		} `json:"result"`
	}
	if err := c.get(ctx, u, &kr); err != nil {
		return nil, err
	}
	if len(kr.Error) > 0 {
		return nil, fmt.Errorf("kraken: %s", strings.Join(kr.Error, "; "))
	}
	last := func(key string) float64 {
		t, ok := kr.Result[key]
		if !ok || len(t.C) == 0 {
			return 0
		}
		f, _ := strconv.ParseFloat(t.C[0], 64)
		return f
	}
	btcUSD := last(btcUSDResultKey)
	out := make(map[string]float64, len(assets))
	for _, a := range assets {
		if v := last(a.krUSDKey); v > 0 {
			out[a.symbol] = v
			continue
		}
		if a.krBTCKey != "" && btcUSD > 0 {
			if b := last(a.krBTCKey); b > 0 {
				out[a.symbol] = b * btcUSD
			}
		}
	}
	return out, nil
}
