// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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

// bisonw serializes the stored CEX API key and secret into the market-making
// status. These pin that they never leave the client, and that everything else
// in the payload survives, including fields this code does not know about.
func TestRedactCEXSecrets(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		wantErr    bool
		wantErrMsg string
		wantAbsent []string
		wantSubstr []string
	}{{
		name:       "credentials are stripped and the name kept",
		status:     `{"cexes":{"Binance":{"config":{"name":"Binance","apiKey":"KEY","apiSecret":"SEC"},"connected":true}}}`,
		wantAbsent: []string{"KEY", "SEC", "apiKey", "apiSecret"},
		wantSubstr: []string{`"name":"Binance"`, `"connected":true`},
	}, {
		name:       "unknown sibling fields on the entry survive",
		status:     `{"cexes":{"X":{"config":{"name":"X","apiKey":"K"},"futureField":{"a":1},"balances":{"42":{"available":7}}}}}`,
		wantAbsent: []string{"apiKey", `"K"`},
		wantSubstr: []string{`"futureField":{"a":1}`, `"available":7`},
	}, {
		name:       "unknown fields inside config are dropped with the secrets",
		status:     `{"cexes":{"X":{"config":{"name":"X","apiKey":"K","futureSecret":"S"}}}}`,
		wantAbsent: []string{"apiKey", "futureSecret", `"S"`, `"K"`},
		wantSubstr: []string{`"name":"X"`},
	}, {
		name:       "null config is left alone",
		status:     `{"cexes":{"X":{"config":null,"connected":false}}}`,
		wantSubstr: []string{`"config":null`, `"connected":false`},
	}, {
		name:       "absent cexes is refused, since a rename must not pass bytes through",
		status:     `{"bots":[{"host":"dex.decred.org"}]}`,
		wantErr:    true,
		wantErrMsg: "no cexes member",
	}, {
		name:       "an entry with no config member is refused",
		status:     `{"cexes":{"X":{"connected":true}}}`,
		wantErr:    true,
		wantErrMsg: "has no config member",
	}, {
		name:       "null cexes is not an error",
		status:     `{"cexes":null,"bots":[]}`,
		wantSubstr: []string{`"cexes":null`},
	}, {
		name:       "every entry is redacted, not just the first",
		status:     `{"cexes":{"A":{"config":{"name":"A","apiSecret":"SA"}},"B":{"config":{"name":"B","apiSecret":"SB"}}}}`,
		wantAbsent: []string{"SA", "SB", "apiSecret"},
		wantSubstr: []string{`"name":"A"`, `"name":"B"`},
	}, {
		name:       "config stays a present object, which the UI reads as configured",
		status:     `{"cexes":{"X":{"config":{"name":"X","apiKey":"K","apiSecret":"S"}}}}`,
		wantAbsent: []string{"apiKey", "apiSecret"},
		wantSubstr: []string{`"config":{`},
	}, {
		name:    "malformed payload is an error, not a passthrough",
		status:  `{"cexes":{"X":{"config":{"name"`,
		wantErr: true,
	}, {
		name:       "top-level bots are untouched",
		status:     `{"bots":[{"host":"h","cexName":"Binance","running":true}],"cexes":{}}`,
		wantSubstr: []string{`"cexName":"Binance"`, `"running":true`},
	}, {
		name:       "empty cexes is fine",
		status:     `{"cexes":{},"bots":[]}`,
		wantSubstr: []string{`"cexes":{}`},
	}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := redactCEXSecrets(json.RawMessage(tc.status))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got %s", got)
				}
				if got != nil {
					t.Fatalf("an error must return no payload, got %s", got)
				}
				if tc.wantErrMsg != "" && !strings.Contains(err.Error(), tc.wantErrMsg) {
					t.Fatalf("want an error naming %q, got %v", tc.wantErrMsg, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("redactCEXSecrets: %v", err)
			}
			for _, s := range tc.wantAbsent {
				if strings.Contains(string(got), s) {
					t.Errorf("%q survived redaction: %s", s, got)
				}
			}
			for _, s := range tc.wantSubstr {
				if !strings.Contains(string(got), s) {
					t.Errorf("%q was lost: %s", s, got)
				}
			}
		})
	}
}

// TestMMStatusRedacts is the property rather than the implementation: whatever
// the redactor looks like, the secret must not reach a caller of MMStatus.
func TestMMStatusRedacts(t *testing.T) {
	const secret = "SUPER-SECRET-EXCHANGE-KEY"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true,"status":{"cexes":{"Binance":{"config":`+
			`{"name":"Binance","apiKey":"%s","apiSecret":"%s"},"connected":true}}}}`, secret, secret)
	}))
	defer srv.Close()

	c := &WebClient{baseURL: srv.URL, http: srv.Client(), loggedIn: true}
	got, err := c.MMStatus(context.Background())
	if err != nil {
		t.Fatalf("MMStatus: %v", err)
	}
	if strings.Contains(string(got), secret) {
		t.Fatalf("MMStatus returned the CEX credential: %s", got)
	}
	if !strings.Contains(string(got), `"name":"Binance"`) {
		t.Fatalf("MMStatus lost the exchange name: %s", got)
	}
}

// TestRedactCEXSecretsPreservesEverythingElse compares parsed structures rather
// than substrings: only cexes.*.config may differ from the input.
func TestRedactCEXSecretsPreservesEverythingElse(t *testing.T) {
	const in = `{"bots":[{"host":"h","config":{"baseID":42,"quoteID":0},"futureBotField":[1,2]}],` +
		`"cexes":{"Binance":{"config":{"name":"Binance","apiKey":"K","apiSecret":"S"},` +
		`"connected":true,"markets":{"dcr_btc":{"baseMinWithdraw":123456789012345}},` +
		`"futureField":{"nested":{"deep":true}}}},"futureTopLevel":"keep me"}`
	got, err := redactCEXSecrets(json.RawMessage(in))
	if err != nil {
		t.Fatalf("redactCEXSecrets: %v", err)
	}
	var want, have map[string]any
	if err := json.Unmarshal([]byte(in), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &have); err != nil {
		t.Fatal(err)
	}
	// The only sanctioned difference.
	want["cexes"].(map[string]any)["Binance"].(map[string]any)["config"] =
		map[string]any{"name": "Binance"}
	if !reflect.DeepEqual(want, have) {
		t.Fatalf("payload changed beyond the config redaction:\n want %v\n have %v", want, have)
	}
}
