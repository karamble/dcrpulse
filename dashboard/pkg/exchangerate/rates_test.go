// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package exchangerate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// canned answers every request by host with a fixed body.
type canned map[string]string

func (c canned) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(c[r.URL.Host])), Header: http.Header{}}, nil
}

func krakenTicker(pairs map[string]string) string {
	var b strings.Builder
	b.WriteString(`{"error":[],"result":{`)
	first := true
	for k, v := range pairs {
		if !first {
			b.WriteString(",")
		}
		first = false
		b.WriteString(`"` + k + `":{"c":["` + v + `","1.0"]}`)
	}
	b.WriteString(`}}`)
	return b.String()
}

func TestKrakenUSDRefusesPricesThatAreNotFiniteAndPositive(t *testing.T) {
	for _, bad := range []string{"NaN", "Inf", "+Infinity", "1e400", "-5", "0"} {
		c := New(canned{"api.kraken.com": krakenTicker(map[string]string{"DCRUSD": bad})})
		if p, err := c.KrakenUSD(context.Background(), "dcr"); err == nil {
			t.Fatalf("%q accepted as %v", bad, p)
		}
	}
	c := New(canned{"api.kraken.com": krakenTicker(map[string]string{"DCRUSD": "18.25"})})
	if p, err := c.KrakenUSD(context.Background(), "dcr"); err != nil || p != 18.25 {
		t.Fatalf("18.25 read as %v, %v", p, err)
	}
}

func TestUSDKeepsOnlyUsableKrakenPrices(t *testing.T) {
	c := New(canned{
		"min-api.cryptocompare.com": `{"Response":"Error"}`,
		"api.kraken.com": krakenTicker(map[string]string{
			"XXBTZUSD": "1e300", // finite, kept
			"DCRUSD":   "Inf",
			"XETHXXBT": "1e300", // times BTC/USD overflows
			"XLTCZUSD": "80.5",
			"BCHUSD":   "1e400",
		}),
	})
	got, err := c.USD(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]float64{"btc": 1e300, "ltc": 80.5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rates %v, want %v", got, want)
	}
	if _, err := json.Marshal(got); err != nil {
		t.Fatalf("rates do not encode: %v", err)
	}
}
