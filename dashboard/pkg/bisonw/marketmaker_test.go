// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The running-bot routes take positional string arguments and put host in a
// different slot on each of the two update routes. bisonw checks the argument
// count and nothing else, so a value in the wrong slot is accepted and means
// something else. These pin the wire order against literals.
func captureRPCArgs(t *testing.T, result string, call func(*Client) error) (route string, args []string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req wireMessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		var params rawParams
		if err := json.Unmarshal(req.Payload, &params); err != nil {
			t.Errorf("decode params: %v", err)
			return
		}
		route, args = req.Route, params.Args
		payload, _ := json.Marshal(responsePayload{Result: json.RawMessage(result)})
		resp, _ := json.Marshal(wireMessage{Type: msgTypeResponse, ID: req.ID, Payload: payload})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(resp)
	}))
	t.Cleanup(srv.Close)

	if err := call(&Client{url: srv.URL, http: srv.Client()}); err != nil {
		t.Fatalf("call: %v", err)
	}
	return route, args
}

func assertArgs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sent %d args %q; want %d %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg %d is %q; want %q", i, got[i], want[i])
		}
	}
}

func TestMMAvailableBalancesArgOrder(t *testing.T) {
	route, args := captureRPCArgs(t, `{"dexBalances":{"42":7},"cexBalances":null}`, func(c *Client) error {
		_, err := c.MMAvailableBalances(context.Background(), "/dex/.dexc/mainnet/mm_cfg.json",
			"dex.decred.org:7232", 42, 0)
		return err
	})
	if route != "mmavailablebalances" {
		t.Errorf("route is %q", route)
	}
	assertArgs(t, args, []string{"/dex/.dexc/mainnet/mm_cfg.json", "dex.decred.org:7232", "42", "0"})
}

func TestMMAvailableBalancesDecodesAtomsByAssetID(t *testing.T) {
	var bals *MMBalances
	captureRPCArgs(t, `{"dexBalances":{"42":250000000,"0":1300000},"cexBalances":{}}`, func(c *Client) error {
		var err error
		bals, err = c.MMAvailableBalances(context.Background(), "/cfg", "host", 42, 0)
		return err
	})
	if bals.DEX[42] != 250000000 {
		t.Errorf("dcr balance is %d; want 250000000", bals.DEX[42])
	}
	if bals.DEX[0] != 1300000 {
		t.Errorf("btc balance is %d; want 1300000", bals.DEX[0])
	}
	if len(bals.CEX) != 0 {
		t.Errorf("cex balances are %v; want empty", bals.CEX)
	}
}

// The config route takes the file path first and host second.
func TestUpdateRunningBotCfgArgOrder(t *testing.T) {
	route, args := captureRPCArgs(t, `"updated running bot"`, func(c *Client) error {
		return c.UpdateRunningBotCfg(context.Background(), "/dex/.dexc/mainnet/mm_cfg.json",
			"dex.decred.org:7232", 42, 0, nil, nil)
	})
	if route != "updaterunningbotcfg" {
		t.Errorf("route is %q", route)
	}
	assertArgs(t, args, []string{"/dex/.dexc/mainnet/mm_cfg.json", "dex.decred.org:7232", "42", "0", "[]", "[]"})
}

// The inventory route takes no file path, so host is first.
func TestUpdateRunningBotInventoryArgOrder(t *testing.T) {
	route, args := captureRPCArgs(t, `"updated running bot"`, func(c *Client) error {
		return c.UpdateRunningBotInventory(context.Background(), "dex.decred.org:7232", 42, 0,
			map[uint32]int64{42: -1000000, 0: 5000000}, nil)
	})
	if route != "updaterunningbotinv" {
		t.Errorf("route is %q", route)
	}
	assertArgs(t, args, []string{"dex.decred.org:7232", "42", "0", "[[0,5000000],[42,-1000000]]", "[]"})
}
