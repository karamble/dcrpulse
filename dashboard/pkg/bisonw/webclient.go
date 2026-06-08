// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"sync"
)

// WebClient talks to bisonw's webserver HTTP API (the same one the official
// DCRDEX web UI uses), as opposed to the RPC server that Client speaks to. It is
// used for the market-maker routes, which the RPC server exposes only through a
// config file; the webserver instead persists bot and CEX configuration in the
// daemon's encrypted database.
//
// The webserver authenticates with a cookie session (dexauth + sessionkey)
// established by POSTing the app password to /api/login. WebClient holds that
// session in a cookie jar and re-logs in transparently when it expires.
type WebClient struct {
	baseURL string
	http    *http.Client

	mu       sync.Mutex
	loggedIn bool
}

// errUnauthed signals an expired or missing webserver session, triggering one
// transparent re-login and retry.
var errUnauthed = errors.New("bisonw web: not authenticated")

// webAck mirrors the webserver's standard JSON response: ok plus an error
// message, with the route-specific result carried in sibling fields.
type webAck struct {
	OK  bool   `json:"ok"`
	Msg string `json:"msg"`
}

// NewWebClient constructs a WebClient for bisonw's webserver, trusting cfg's
// pinned TLS cert (web.cert). User/Pass are unused (the webserver authenticates
// by session cookie, not HTTP Basic).
func NewWebClient(cfg Config) (*WebClient, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("bisonw: Addr is required")
	}
	tlsConfig, err := tlsConfigFor(cfg)
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &WebClient{
		baseURL: "https://" + cfg.Addr,
		http: &http.Client{
			Jar:       jar,
			Transport: &http.Transport{TLSClientConfig: tlsConfig},
		},
	}, nil
}

// login establishes a session by POSTing the app password to /api/login. The
// resulting dexauth/sessionkey cookies are stored in the jar.
func (c *WebClient) login(ctx context.Context, appPass string) error {
	if err := c.request(ctx, http.MethodPost, "/api/login", map[string]string{"pass": appPass}, nil); err != nil {
		return fmt.Errorf("bisonw web: login: %w", err)
	}
	c.mu.Lock()
	c.loggedIn = true
	c.mu.Unlock()
	return nil
}

// call ensures a session exists, runs the request, and re-logs in once if the
// session has expired. appPass is needed for the (re-)login.
func (c *WebClient) call(ctx context.Context, method, path, appPass string, body, result any) error {
	c.mu.Lock()
	loggedIn := c.loggedIn
	c.mu.Unlock()
	if !loggedIn {
		if err := c.login(ctx, appPass); err != nil {
			return err
		}
	}
	err := c.request(ctx, method, path, body, result)
	if errors.Is(err, errUnauthed) {
		c.mu.Lock()
		c.loggedIn = false
		c.mu.Unlock()
		if lerr := c.login(ctx, appPass); lerr != nil {
			return lerr
		}
		err = c.request(ctx, method, path, body, result)
	}
	return err
}

// request performs one HTTP call, decodes the {ok,msg} envelope, and (when ok
// and result is non-nil) unmarshals the full body into result so route-specific
// fields are available to the caller.
func (c *WebClient) request(ctx context.Context, method, path string, body, result any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("bisonw web: marshal body for %s: %w", path, err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("bisonw web: %s: %w", path, err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("bisonw web: %s: read response: %w", path, err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return errUnauthed
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bisonw web: %s: http %d: %s", path, resp.StatusCode, bytes.TrimSpace(respBytes))
	}
	var ack webAck
	if err := json.Unmarshal(respBytes, &ack); err != nil {
		return fmt.Errorf("bisonw web: %s: decode response: %w", path, err)
	}
	if !ack.OK {
		// rejectUnauthed also answers 200 with ok:false in some builds; treat an
		// explicit auth message as a session expiry so the caller re-logs in.
		if ack.Msg == "" {
			return fmt.Errorf("bisonw web: %s: request failed", path)
		}
		return fmt.Errorf("bisonw web: %s", ack.Msg)
	}
	if result != nil {
		if err := json.Unmarshal(respBytes, result); err != nil {
			return fmt.Errorf("bisonw web: %s: decode result: %w", path, err)
		}
	}
	return nil
}

// NewDepositAddress fetches a fresh deposit address for the asset's wallet. This
// route exists only on the webserver, not the RPC server. The dcrwallet backend
// returns its next unused external address (the index is managed by dcrwallet).
func (c *WebClient) NewDepositAddress(ctx context.Context, appPass string, assetID uint32) (string, error) {
	var res struct {
		webAck
		Address string `json:"address"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/depositaddress", appPass, map[string]any{"assetID": assetID}, &res); err != nil {
		return "", err
	}
	return res.Address, nil
}

// EstimateSendTxFee estimates the network fee to send value (atoms of assetID) to
// addr and reports whether addr is a valid address for the asset. Webserver-only
// route (/api/txfee). The returned fee is in the fee asset's atoms (the parent
// chain coin for a token, otherwise the asset itself).
func (c *WebClient) EstimateSendTxFee(ctx context.Context, appPass string, assetID uint32, addr string, value uint64, subtract, maxWithdraw bool) (txFee uint64, validAddress bool, err error) {
	var res struct {
		webAck
		TxFee        uint64 `json:"txfee"`
		ValidAddress bool   `json:"validaddress"`
	}
	body := map[string]any{
		"assetID":     assetID,
		"addr":        addr,
		"value":       value,
		"subtract":    subtract,
		"maxWithdraw": maxWithdraw,
	}
	if err := c.call(ctx, http.MethodPost, "/api/txfee", appPass, body, &res); err != nil {
		return 0, false, err
	}
	return res.TxFee, res.ValidAddress, nil
}

// PreOrder returns bisonw's pre-order estimate for a prospective order: the swap
// and redeem network-fee estimates (best/worst case) and the order options
// available per asset. The args mirror a core.TradeForm. Webserver-only route
// (/api/preorder); the RPC server has no equivalent. The raw `estimate` object
// is returned for the caller to forward.
func (c *WebClient) PreOrder(ctx context.Context, appPass, host string, isLimit, sell bool, baseID, quoteID uint32, qty, rate uint64, tifNow bool, options map[string]string) (json.RawMessage, error) {
	body := map[string]any{
		"host":    host,
		"isLimit": isLimit,
		"sell":    sell,
		"base":    baseID,
		"quote":   quoteID,
		"qty":     qty,
		"rate":    rate,
		"tifnow":  tifNow,
		"options": options,
	}
	var res struct {
		webAck
		Estimate json.RawMessage `json:"estimate"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/preorder", appPass, body, &res); err != nil {
		return nil, err
	}
	return res.Estimate, nil
}

// MaxBuy returns the largest buy order (lots + fee estimate) fundable at rate on
// the host's base/quote market. Webserver-only route (/api/maxbuy).
func (c *WebClient) MaxBuy(ctx context.Context, appPass, host string, baseID, quoteID uint32, rate uint64) (json.RawMessage, error) {
	body := map[string]any{"host": host, "base": baseID, "quote": quoteID, "rate": rate}
	var res struct {
		webAck
		MaxBuy json.RawMessage `json:"maxBuy"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/maxbuy", appPass, body, &res); err != nil {
		return nil, err
	}
	return res.MaxBuy, nil
}

// MaxSell returns the largest sell order (lots + fee estimate) fundable on the
// host's base/quote market. Webserver-only route (/api/maxsell).
func (c *WebClient) MaxSell(ctx context.Context, appPass, host string, baseID, quoteID uint32) (json.RawMessage, error) {
	body := map[string]any{"host": host, "base": baseID, "quote": quoteID}
	var res struct {
		webAck
		MaxSell json.RawMessage `json:"maxSell"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/maxsell", appPass, body, &res); err != nil {
		return nil, err
	}
	return res.MaxSell, nil
}

// Orders returns the user's full order history matching filter (a core.OrderFilter
// JSON object: n, offset, hosts, assets, statuses, market). Unlike the RPC
// myorders route (active + recently-tracked only), this webserver route reads the
// full orders database, so it includes canceled/executed/revoked orders. The raw
// `orders` array is returned for the caller to normalize.
func (c *WebClient) Orders(ctx context.Context, appPass string, filter map[string]any) (json.RawMessage, error) {
	var res struct {
		webAck
		Orders json.RawMessage `json:"orders"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/orders", appPass, filter, &res); err != nil {
		return nil, err
	}
	return res.Orders, nil
}

// Order returns a single order by its hex id, including live swap-coin
// confirmation counts for active orders (the RPC myorders route and the orders
// archive both omit confs). Webserver-only route (/api/order); the body is the
// order id encoded as a JSON hex string (dex.Bytes).
func (c *WebClient) Order(ctx context.Context, appPass, orderID string) (json.RawMessage, error) {
	var res struct {
		webAck
		Order json.RawMessage `json:"order"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/order", appPass, orderID, &res); err != nil {
		return nil, err
	}
	return res.Order, nil
}

// AddressUsed reports whether the asset's wallet has ever received funds at addr,
// used to warn against deposit-address reuse. Webserver-only route.
func (c *WebClient) AddressUsed(ctx context.Context, appPass string, assetID uint32, addr string) (bool, error) {
	var res struct {
		webAck
		Used bool `json:"used"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/addressused", appPass, map[string]any{"assetID": assetID, "addr": addr}, &res); err != nil {
		return false, err
	}
	return res.Used, nil
}

// MMStatus returns the market-making status (bots + CEX state) as the raw
// `status` object from /api/marketmakingstatus.
func (c *WebClient) MMStatus(ctx context.Context, appPass string) (json.RawMessage, error) {
	var res struct {
		webAck
		Status json.RawMessage `json:"status"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/marketmakingstatus", appPass, nil, &res); err != nil {
		return nil, err
	}
	return res.Status, nil
}

// UpdateBotConfig persists (and validates) a bot config. cfg is a full
// mm.BotConfig JSON object built by the caller.
func (c *WebClient) UpdateBotConfig(ctx context.Context, appPass string, cfg json.RawMessage) error {
	return c.call(ctx, http.MethodPost, "/api/updatebotconfig", appPass, cfg, nil)
}

// RemoveBotConfig deletes a stored bot config.
func (c *WebClient) RemoveBotConfig(ctx context.Context, appPass, host string, baseID, quoteID uint32) error {
	body := map[string]any{"host": host, "baseID": baseID, "quoteID": quoteID}
	return c.call(ctx, http.MethodPost, "/api/removebotconfig", appPass, body, nil)
}

// UpdateCEXConfig stores (and validates) CEX API credentials. cfg is an
// mm.CEXConfig JSON object {name, apiKey, apiSecret}.
func (c *WebClient) UpdateCEXConfig(ctx context.Context, appPass string, cfg json.RawMessage) error {
	return c.call(ctx, http.MethodPost, "/api/updatecexconfig", appPass, cfg, nil)
}

// StartBot starts a configured bot. startCfg is an mm.StartConfig JSON object
// (MarketWithHost plus optional alloc/autoRebalance); the app password is sent
// alongside so the daemon can unlock the wallets.
func (c *WebClient) StartBot(ctx context.Context, appPass string, startCfg json.RawMessage) error {
	body := map[string]any{"config": startCfg, "appPW": appPass}
	return c.call(ctx, http.MethodPost, "/api/startmarketmakingbot", appPass, body, nil)
}

// StopBot stops a running bot on the given market.
func (c *WebClient) StopBot(ctx context.Context, appPass, host string, baseID, quoteID uint32) error {
	body := map[string]any{"market": map[string]any{"host": host, "baseID": baseID, "quoteID": quoteID}}
	return c.call(ctx, http.MethodPost, "/api/stopmarketmakingbot", appPass, body, nil)
}

// MarketReport returns the market report (oracle prices and fiat rates) used by
// the bot configuration UI, as the raw `report` object from /api/marketreport.
func (c *WebClient) MarketReport(ctx context.Context, appPass, host string, baseID, quoteID uint32) (json.RawMessage, error) {
	body := map[string]any{"host": host, "baseID": baseID, "quoteID": quoteID}
	var res struct {
		webAck
		Report json.RawMessage `json:"report"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/marketreport", appPass, body, &res); err != nil {
		return nil, err
	}
	return res.Report, nil
}
