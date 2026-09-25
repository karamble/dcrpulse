// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"dcrpulse/internal/services"
	"dcrpulse/pkg/exchangerate"
)

type countingTransport struct{ calls atomic.Int32 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, errors.New("no network in tests")
}

func TestRateHandlersAnswerNoPriceWhileExchangeRatesAreOff(t *testing.T) {
	old := services.ExchangeRatesEnabled
	services.ExchangeRatesEnabled = func() bool { return false }
	t.Cleanup(func() { services.ExchangeRatesEnabled = old })
	rt := &countingTransport{}
	oldCache := dexRateCache
	dexRateCache = exchangerate.New(rt)
	t.Cleanup(func() { dexRateCache = oldCache })

	rec := httptest.NewRecorder()
	GetDcrdexRatesHandler(rec, httptest.NewRequest(http.MethodGet, "/api/dcrdex/rates", nil))
	var dex map[string]float64
	if err := json.NewDecoder(rec.Body).Decode(&dex); err != nil || len(dex) != 0 || rt.calls.Load() != 0 {
		t.Fatalf("dex rates %v (%v) after %d requests", dex, err, rt.calls.Load())
	}

	rec = httptest.NewRecorder()
	BisonrelayRatesHandler(rec, httptest.NewRequest(http.MethodGet, "/api/br/rates", nil))
	var br struct {
		DcrUSD float64 `json:"dcr_usd"`
		BtcUSD float64 `json:"btc_usd"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&br); err != nil || rec.Code != http.StatusOK || br.DcrUSD != 0 || br.BtcUSD != 0 {
		t.Fatalf("br rates %d %+v %v", rec.Code, br, err)
	}
}
