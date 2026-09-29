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
	"strings"
	"testing"
	"time"
)

func TestDecodeRequest(t *testing.T) {
	var v struct {
		N int `json:"n"`
	}
	rec := httptest.NewRecorder()
	if decodeRequest(rec, httptest.NewRequest("POST", "/", strings.NewReader(`{"n":`)), &v) {
		t.Fatal("a truncated body decoded")
	}
	if rec.Code != http.StatusBadRequest || rec.Body.String() != "invalid request body\n" {
		t.Errorf("answer = %d %q, want 400 invalid request body", rec.Code, rec.Body.String())
	}
	if !decodeRequest(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{"n":3}`)), &v) || v.N != 3 {
		t.Errorf("a good body did not decode: %+v", v)
	}
}

// stubGate stands in for the DEX gates: it either hands out a client or answers
// the way dexWebSession does while locked.
func stubGate(open bool) func(http.ResponseWriter) (string, bool) {
	return func(w http.ResponseWriter) (string, bool) {
		if !open {
			http.Error(w, "DCRDEX is locked", http.StatusConflict)
			return "", false
		}
		return "client", true
	}
}

func TestDexCall(t *testing.T) {
	serve := func(open bool, call func(ctx context.Context, c string) (any, error)) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		dexCall(rec, httptest.NewRequest("POST", "/", nil), stubGate(open), 7*time.Second, call)
		return rec
	}

	t.Run("a refused gate never reaches the daemon", func(t *testing.T) {
		rec := serve(false, func(context.Context, string) (any, error) {
			t.Fatal("the call ran past a refused gate")
			return nil, nil
		})
		if rec.Code != http.StatusConflict {
			t.Errorf("status = %d, want 409", rec.Code)
		}
	})
	t.Run("the call runs under the handler's timeout", func(t *testing.T) {
		serve(true, func(ctx context.Context, c string) (any, error) {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 7*time.Second || time.Until(deadline) < 6*time.Second {
				t.Errorf("deadline = %v (set %v), want about 7s", time.Until(deadline), ok)
			}
			if c != "client" {
				t.Errorf("client = %q, want the gate's", c)
			}
			return nil, nil
		})
	})
	t.Run("a daemon error is mapped by dexWriteErr", func(t *testing.T) {
		rec := serve(true, func(context.Context, string) (any, error) { return nil, errors.New("order not found") })
		if rec.Code != http.StatusBadGateway || rec.Body.String() != "order not found\n" {
			t.Errorf("answer = %d %q, want 502 order not found", rec.Code, rec.Body.String())
		}
	})
	t.Run("a raw reply is forwarded byte for byte", func(t *testing.T) {
		const raw = `{ "b": 2,  "a": [1, 2] }`
		rec := serve(true, func(context.Context, string) (any, error) { return json.RawMessage(raw), nil })
		if rec.Body.String() != raw || rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("answer = %q (%s), want the bytes as sent, as JSON", rec.Body.String(), rec.Header().Get("Content-Type"))
		}
	})
	t.Run("any other result is encoded", func(t *testing.T) {
		rec := serve(true, func(context.Context, string) (any, error) { return map[string]string{"coin": "c1"}, nil })
		if rec.Body.String() != "{\"coin\":\"c1\"}\n" || rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("answer = %q (%s)", rec.Body.String(), rec.Header().Get("Content-Type"))
		}
	})
}

func TestDexDo(t *testing.T) {
	rec := httptest.NewRecorder()
	dexDo(rec, httptest.NewRequest("POST", "/", nil), stubGate(true), time.Second,
		func(context.Context, string) error { return nil })
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"ok\":true}\n" {
		t.Errorf("answer = %d %q, want 200 {\"ok\":true}", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	dexDo(rec, httptest.NewRequest("POST", "/", nil), stubGate(true), time.Second,
		func(context.Context, string) error { return errors.New("wallet not found") })
	if rec.Code != http.StatusBadGateway {
		t.Errorf("a failed action answered %d, want 502", rec.Code)
	}
}

// The converted DEX routes keep their gates. The test process has no DEX
// session and no configured client, so each route answers from its gate: 409
// for the locked-session gates, 503 for the RPC-client one. A malformed body
// is still refused first.
func TestDexRoutesKeepTheirGates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		url     string
		body    string
		want    int
	}{
		{"deposit address", NewDexDepositAddressHandler, "/?assetID=42", "", http.StatusConflict},
		{"address used", DexAddressUsedHandler, "/?addr=Ds1", "", http.StatusConflict},
		{"bond fee buffer", GetDcrdexBondsFeeBufferHandler, "/?assetID=42", "", http.StatusConflict},
		{"single order", GetDcrdexSingleOrderHandler, "/", `{"id":"ab"}`, http.StatusConflict},
		{"place", PlaceDcrdexOrderHandler, "/", `{"host":"h","qty":1}`, http.StatusConflict},
		{"actions", GetDcrdexActionsHandler, "/", "", http.StatusConflict},
		{"pre-accelerate", PreDcrdexAccelerateHandler, "/", `{"orderID":"ab"}`, http.StatusConflict},
		{"estimate", DcrdexAccelerationEstimateHandler, "/", `{"orderID":"ab","newRate":1}`, http.StatusConflict},
		{"accelerate", AccelerateDcrdexOrderHandler, "/", `{"orderID":"ab","newRate":1,"appPass":"p"}`, http.StatusConflict},
		{"pre-order", PreDcrdexOrderHandler, "/", `{"host":"h","qty":1}`, http.StatusConflict},
		{"max buy", MaxDcrdexBuyHandler, "/", `{"host":"h","rate":1}`, http.StatusConflict},
		{"max sell", MaxDcrdexSellHandler, "/", `{"host":"h"}`, http.StatusConflict},
		{"create asset wallet", CreateDcrdexAssetWalletHandler, "/", `{"walletType":"spv"}`, http.StatusConflict},
		{"send", SendDcrdexWalletHandler, "/", `{"value":1,"address":"a","appPass":"p"}`, http.StatusConflict},
		{"estimate fee", EstimateDcrdexSendFeeHandler, "/", `{"value":1,"address":"a"}`, http.StatusConflict},
		{"open wallet", OpenDcrdexWalletHandler, "/", `{"assetID":42}`, http.StatusConflict},
		{"discover", DiscoverDcrdexAccountHandler, "/", `{"host":"h"}`, http.StatusConflict},
		{"mm status", GetDcrdexMMStatusHandler, "/", "", http.StatusConflict},
		{"mm report", GetDcrdexMMMarketReportHandler, "/?host=h&baseID=42&quoteID=0", "", http.StatusConflict},
		{"mm run logs", GetDcrdexMMRunLogsHandler, "/?host=h&baseID=42&quoteID=0&startTime=1", "", http.StatusConflict},
		{"mm archived runs", GetDcrdexMMArchivedRunsHandler, "/", "", http.StatusConflict},
		{"mm bot config", UpdateDcrdexMMBotConfigHandler, "/", `{"x":1}`, http.StatusConflict},
		{"mm stop", StopDcrdexMMBotHandler, "/", `{"host":"h"}`, http.StatusConflict},
		{"mm start", StartDcrdexMMBotHandler, "/", `{"x":1}`, http.StatusConflict},
		{"mm inventory", UpdateDcrdexMMRunningBotInventoryHandler, "/", `{"host":"h","dexDiffs":{"42":1}}`, http.StatusConflict},
		{"exchanges", GetDcrdexExchangesHandler, "/", "", http.StatusServiceUnavailable},
		{"my orders", GetDcrdexMyOrdersHandler, "/", "", http.StatusServiceUnavailable},
		{"peers", GetDcrdexWalletPeersHandler, "/?assetID=42", "", http.StatusServiceUnavailable},
		{"notifications", GetDcrdexNotificationsHandler, "/", "", http.StatusServiceUnavailable},
		{"export seed", ExportDcrdexSeedHandler, "/", `{"appPass":"p"}`, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.handler(rec, httptest.NewRequest("POST", tc.url, strings.NewReader(tc.body)))
			if rec.Code != tc.want {
				t.Fatalf("answer = %d %q, want %d from the gate", rec.Code, rec.Body.String(), tc.want)
			}
			if tc.want == http.StatusConflict && rec.Body.String() != "DCRDEX is locked\n" {
				t.Errorf("body = %q, want the lock message", rec.Body.String())
			}
		})
	}

	rec := httptest.NewRecorder()
	PlaceDcrdexOrderHandler(rec, httptest.NewRequest("POST", "/", strings.NewReader(`{"host":`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a malformed order answered %d, want 400 before the gate", rec.Code)
	}
}

func TestDexBondAssets(t *testing.T) {
	got := dexBondAssets(map[string]dexBondOffer{
		"dcr": {ID: 42, Confs: 2, Amt: 150_000_000},
		"btc": {ID: 0, Confs: 1, Amt: 100_000},
	})
	if len(got) != 2 || got[0].Symbol != "BTC" || got[1].Symbol != "DCR" {
		t.Fatalf("assets = %+v, want BTC then DCR", got)
	}
	if got[1].AmtAtoms != 150_000_000 || got[1].Amt != 1.5 || got[1].Confs != 2 || got[1].AssetID != 42 {
		t.Errorf("DCR entry = %+v, want 1.5 DCR from 150000000 atoms", got[1])
	}
}

func TestFormatSemver(t *testing.T) {
	if got := formatSemver(0, 10, 4294967295); got != "0.10.4294967295" {
		t.Errorf("formatSemver = %q", got)
	}
}
