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
	"strings"
	"sync"
)

// WebClient talks to bisonw's webserver HTTP API. The cookie jar is the whole
// session: bisonw resolves the app password server-side, we never hold it.
type WebClient struct {
	baseURL string
	http    *http.Client

	// loginMu queues logins: each costs the daemon two argon2 runs and
	// registers a token it only releases on logout.
	loginMu sync.Mutex

	mu       sync.Mutex
	loggedIn bool
}

// errUnauthed signals an expired or missing webserver session (the daemon's
// rejectUnauthed 401); callSession maps it to ErrDexLocked.
var errUnauthed = errors.New("bisonw web: not authenticated")

// ErrDexLocked reports that no webserver session is established; only an
// explicit user unlock recovers it (no password is held to retry with).
var ErrDexLocked = errors.New("bisonw web: session locked")

// webMsgError keeps an ok:false reply's daemon message inspectable.
type webMsgError struct{ msg string }

func (e *webMsgError) Error() string { return "bisonw web: " + e.msg }

// staleSessionMsg matches the resolvePass failures bisonw reports as ok:false
// rather than 401; both mean the session is unusable.
func staleSessionMsg(msg string) bool {
	return msg == "app pass cannot be empty" ||
		strings.HasPrefix(msg, "cached encrypted password not found")
}

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

// Login establishes the webserver session; bisonw caches the password
// encrypted under the sessionkey cookie and resolves it for later calls.
func (c *WebClient) Login(ctx context.Context, appPass string) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if err := c.request(ctx, http.MethodPost, "/api/login", map[string]string{"pass": appPass}, nil); err != nil {
		return fmt.Errorf("bisonw web: login: %w", err)
	}
	c.mu.Lock()
	c.loggedIn = true
	c.mu.Unlock()
	return nil
}

// InitApp initializes bisonw (optionally restoring a mnemonic seed) via
// /api/init, which also logs in. The reply's mnemonic stays undecoded.
func (c *WebClient) InitApp(ctx context.Context, appPass, seed string) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	body := map[string]string{"pass": appPass}
	if seed != "" {
		body["seed"] = seed
	}
	if err := c.request(ctx, http.MethodPost, "/api/init", body, nil); err != nil {
		return fmt.Errorf("bisonw web: init: %w", err)
	}
	c.mu.Lock()
	c.loggedIn = true
	c.mu.Unlock()
	return nil
}

// Logout locks bisonw and revokes every webserver session and its cached
// password. Refused while any order is active; the session survives that.
func (c *WebClient) Logout(ctx context.Context) error {
	if err := c.callSession(ctx, http.MethodPost, "/api/logout", struct{}{}, nil); err != nil {
		return err
	}
	c.mu.Lock()
	c.loggedIn = false
	c.mu.Unlock()
	return nil
}

// LoggedIn reports the locally tracked session state without a network call.
func (c *WebClient) LoggedIn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loggedIn
}

// SessionValid probes /api/user (200 even unauthed): a non-null user means a
// live session and an unlocked daemon. The local flag is reconciled to match.
func (c *WebClient) SessionValid(ctx context.Context) (bool, error) {
	var res struct {
		webAck
		User json.RawMessage `json:"user"`
	}
	if err := c.request(ctx, http.MethodGet, "/api/user", nil, &res); err != nil {
		return false, err
	}
	valid := len(res.User) > 0 && string(res.User) != "null"
	c.mu.Lock()
	c.loggedIn = valid
	c.mu.Unlock()
	return valid, nil
}

// callSession runs one request on the cookie session. It never logs in: no
// session, a 401, or a stale password cache all surface as ErrDexLocked.
func (c *WebClient) callSession(ctx context.Context, method, path string, body, result any) error {
	c.mu.Lock()
	loggedIn := c.loggedIn
	c.mu.Unlock()
	if !loggedIn {
		return ErrDexLocked
	}
	err := c.request(ctx, method, path, body, result)
	var wm *webMsgError
	if errors.Is(err, errUnauthed) || (errors.As(err, &wm) && staleSessionMsg(wm.msg)) {
		c.mu.Lock()
		c.loggedIn = false
		c.mu.Unlock()
		return ErrDexLocked
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
		if ack.Msg == "" {
			return fmt.Errorf("bisonw web: %s: request failed", path)
		}
		return &webMsgError{msg: ack.Msg}
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
func (c *WebClient) NewDepositAddress(ctx context.Context, assetID uint32) (string, error) {
	var res struct {
		webAck
		Address string `json:"address"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/depositaddress", map[string]any{"assetID": assetID}, &res); err != nil {
		return "", err
	}
	return res.Address, nil
}

// BondsFeeBuffer returns the fee buffer (in the asset's atoms) bisonw recommends
// reserving on top of the bond amount to cover the bond transaction fees for the
// given bond asset. Webserver-only route (/api/bondsfeebuffer).
func (c *WebClient) BondsFeeBuffer(ctx context.Context, assetID uint32) (uint64, error) {
	var res struct {
		webAck
		FeeBuffer uint64 `json:"feeBuffer"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/bondsfeebuffer", map[string]any{"assetID": assetID}, &res); err != nil {
		return 0, err
	}
	return res.FeeBuffer, nil
}

// EstimateSendTxFee estimates the network fee to send value (atoms of assetID) to
// addr and reports whether addr is a valid address for the asset. Webserver-only
// route (/api/txfee). The returned fee is in the fee asset's atoms (the parent
// chain coin for a token, otherwise the asset itself).
func (c *WebClient) EstimateSendTxFee(ctx context.Context, assetID uint32, addr string, value uint64, subtract, maxWithdraw bool) (txFee uint64, validAddress bool, err error) {
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
	if err := c.callSession(ctx, http.MethodPost, "/api/txfee", body, &res); err != nil {
		return 0, false, err
	}
	return res.TxFee, res.ValidAddress, nil
}

// PreOrder returns bisonw's pre-order estimate for a prospective order: the swap
// and redeem network-fee estimates (best/worst case) and the order options
// available per asset. The args mirror a core.TradeForm. Webserver-only route
// (/api/preorder); the RPC server has no equivalent. The raw `estimate` object
// is returned for the caller to forward.
func (c *WebClient) PreOrder(ctx context.Context, host string, isLimit, sell bool, baseID, quoteID uint32, qty, rate uint64, tifNow bool, options map[string]string) (json.RawMessage, error) {
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
	if err := c.callSession(ctx, http.MethodPost, "/api/preorder", body, &res); err != nil {
		return nil, err
	}
	return res.Estimate, nil
}

// MaxBuy returns the largest buy order (lots + fee estimate) fundable at rate on
// the host's base/quote market. Webserver-only route (/api/maxbuy).
func (c *WebClient) MaxBuy(ctx context.Context, host string, baseID, quoteID uint32, rate uint64) (json.RawMessage, error) {
	body := map[string]any{"host": host, "base": baseID, "quote": quoteID, "rate": rate}
	var res struct {
		webAck
		MaxBuy json.RawMessage `json:"maxBuy"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/maxbuy", body, &res); err != nil {
		return nil, err
	}
	return res.MaxBuy, nil
}

// MaxSell returns the largest sell order (lots + fee estimate) fundable on the
// host's base/quote market. Webserver-only route (/api/maxsell).
func (c *WebClient) MaxSell(ctx context.Context, host string, baseID, quoteID uint32) (json.RawMessage, error) {
	body := map[string]any{"host": host, "base": baseID, "quote": quoteID}
	var res struct {
		webAck
		MaxSell json.RawMessage `json:"maxSell"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/maxsell", body, &res); err != nil {
		return nil, err
	}
	return res.MaxSell, nil
}

// Orders returns the user's full order history matching filter (a core.OrderFilter
// JSON object: n, offset, hosts, assets, statuses, market). Unlike the RPC
// myorders route (active + recently-tracked only), this webserver route reads the
// full orders database, so it includes canceled/executed/revoked orders. The raw
// `orders` array is returned for the caller to normalize.
func (c *WebClient) Orders(ctx context.Context, filter map[string]any) (json.RawMessage, error) {
	var res struct {
		webAck
		Orders json.RawMessage `json:"orders"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/orders", filter, &res); err != nil {
		return nil, err
	}
	return res.Orders, nil
}

// Order returns a single order by its hex id, including live swap-coin
// confirmation counts for active orders (the RPC myorders route and the orders
// archive both omit confs). Webserver-only route (/api/order); the body is the
// order id encoded as a JSON hex string (dex.Bytes).
func (c *WebClient) Order(ctx context.Context, orderID string) (json.RawMessage, error) {
	var res struct {
		webAck
		Order json.RawMessage `json:"order"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/order", orderID, &res); err != nil {
		return nil, err
	}
	return res.Order, nil
}

// AddressUsed reports whether the asset's wallet has ever received funds at addr,
// used to warn against deposit-address reuse. Webserver-only route.
func (c *WebClient) AddressUsed(ctx context.Context, assetID uint32, addr string) (bool, error) {
	var res struct {
		webAck
		Used bool `json:"used"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/addressused", map[string]any{"assetID": assetID, "addr": addr}, &res); err != nil {
		return false, err
	}
	return res.Used, nil
}

// WalletSettings returns bisonw's stored configuration for the asset's wallet.
// Webserver-only route. The map holds credentials in the clear.
func (c *WebClient) WalletSettings(ctx context.Context, assetID uint32) (map[string]string, error) {
	var res struct {
		webAck
		Map map[string]string `json:"map"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/walletsettings", map[string]any{"assetID": assetID}, &res); err != nil {
		return nil, err
	}
	return res.Map, nil
}

// ReconfigureWallet replaces the asset wallet's stored configuration and
// reconnects it. Webserver-only route; cfg replaces the settings wholesale.
// An empty newWalletPW leaves the stored wallet password alone.
func (c *WebClient) ReconfigureWallet(ctx context.Context, assetID uint32, walletType string, cfg map[string]string, newWalletPW string) error {
	body := map[string]any{
		"assetID":    assetID,
		"walletType": walletType,
		"config":     cfg,
	}
	// The key must be absent, not null: bisonw fails to unmarshal a null here.
	if newWalletPW != "" {
		body["newWalletPW"] = newWalletPW
	}
	return c.callSession(ctx, http.MethodPost, "/api/reconfigurewallet", body, nil)
}

// MMStatus returns the market-making status (bots + CEX state) as the raw
// `status` object from /api/marketmakingstatus.
func (c *WebClient) MMStatus(ctx context.Context) (json.RawMessage, error) {
	var res struct {
		webAck
		Status json.RawMessage `json:"status"`
	}
	if err := c.callSession(ctx, http.MethodGet, "/api/marketmakingstatus", nil, &res); err != nil {
		return nil, err
	}
	return res.Status, nil
}

// ArchivedRuns returns the market-maker run history as the raw `runs` array
// from /api/archivedmmruns (each entry is {startTime, market, profit}).
func (c *WebClient) ArchivedRuns(ctx context.Context) (json.RawMessage, error) {
	var res struct {
		webAck
		Runs json.RawMessage `json:"runs"`
	}
	if err := c.callSession(ctx, http.MethodGet, "/api/archivedmmruns", nil, &res); err != nil {
		return nil, err
	}
	return res.Runs, nil
}

// UpdateBotConfig persists (and validates) a bot config. cfg is a full
// mm.BotConfig JSON object built by the caller.
func (c *WebClient) UpdateBotConfig(ctx context.Context, cfg json.RawMessage) error {
	return c.callSession(ctx, http.MethodPost, "/api/updatebotconfig", cfg, nil)
}

// RemoveBotConfig deletes a stored bot config.
func (c *WebClient) RemoveBotConfig(ctx context.Context, host string, baseID, quoteID uint32) error {
	body := map[string]any{"host": host, "baseID": baseID, "quoteID": quoteID}
	return c.callSession(ctx, http.MethodPost, "/api/removebotconfig", body, nil)
}

// UpdateCEXConfig stores (and validates) CEX API credentials. cfg is an
// mm.CEXConfig JSON object {name, apiKey, apiSecret}.
func (c *WebClient) UpdateCEXConfig(ctx context.Context, cfg json.RawMessage) error {
	return c.callSession(ctx, http.MethodPost, "/api/updatecexconfig", cfg, nil)
}

// StartBot starts a configured bot. startCfg is an mm.StartConfig JSON object
// (MarketWithHost plus optional alloc/autoRebalance); the wallet-unlock
// password resolves from the daemon's session cache.
func (c *WebClient) StartBot(ctx context.Context, startCfg json.RawMessage) error {
	body := map[string]any{"config": startCfg}
	return c.callSession(ctx, http.MethodPost, "/api/startmarketmakingbot", body, nil)
}

// StopBot stops a running bot on the given market.
func (c *WebClient) StopBot(ctx context.Context, host string, baseID, quoteID uint32) error {
	body := map[string]any{"market": map[string]any{"host": host, "baseID": baseID, "quoteID": quoteID}}
	return c.callSession(ctx, http.MethodPost, "/api/stopmarketmakingbot", body, nil)
}

// MarketReport returns the market report (oracle prices and fiat rates) used by
// the bot configuration UI, as the raw `report` object from /api/marketreport.
func (c *WebClient) MarketReport(ctx context.Context, host string, baseID, quoteID uint32) (json.RawMessage, error) {
	body := map[string]any{"host": host, "baseID": baseID, "quoteID": quoteID}
	var res struct {
		webAck
		Report json.RawMessage `json:"report"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/marketreport", body, &res); err != nil {
		return nil, err
	}
	return res.Report, nil
}

// RunLogs returns a market-maker run's event log (the bot's DEX/CEX orders,
// deposits, and withdrawals) plus the run overview, as the raw {overview, logs,
// updatedLogs} from the webserver-only /api/mmrunlogs route. n caps the number
// of events; refID pages older events (the id of the oldest event already held).
func (c *WebClient) RunLogs(ctx context.Context, host string, baseID, quoteID uint32, startTime int64, n uint64, refID *uint64) (json.RawMessage, error) {
	body := map[string]any{
		"startTime": startTime,
		"market":    map[string]any{"host": host, "baseID": baseID, "quoteID": quoteID},
		"n":         n,
	}
	if refID != nil {
		body["refID"] = *refID
	}
	var res struct {
		webAck
		Overview    json.RawMessage `json:"overview"`
		Logs        json.RawMessage `json:"logs"`
		UpdatedLogs json.RawMessage `json:"updatedLogs"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/mmrunlogs", body, &res); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]json.RawMessage{
		"overview":    res.Overview,
		"logs":        res.Logs,
		"updatedLogs": res.UpdatedLogs,
	})
}

// Trade places an order via the synchronous /api/trade, returning the raw
// finalized `order` object. The empty pw resolves from the session cache.
func (c *WebClient) Trade(ctx context.Context, host string, isLimit, sell bool, baseID, quoteID uint32, qty, rate uint64, tifNow bool, options map[string]string) (json.RawMessage, error) {
	body := map[string]any{
		"pw": "",
		"order": map[string]any{
			"host":    host,
			"isLimit": isLimit,
			"sell":    sell,
			"base":    baseID,
			"quote":   quoteID,
			"qty":     qty,
			"rate":    rate,
			"tifnow":  tifNow,
			"options": options,
		},
	}
	var res struct {
		webAck
		Order json.RawMessage `json:"order"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/trade", body, &res); err != nil {
		return nil, err
	}
	return res.Order, nil
}

// Send sends value (atoms) to an address, returning the coin id. /api/send
// refuses the session password cache, so the app password rides each call.
func (c *WebClient) Send(ctx context.Context, appPass string, assetID uint32, value uint64, address string, subtract bool) (string, error) {
	body := map[string]any{
		"assetID":  assetID,
		"value":    value,
		"address":  address,
		"subtract": subtract,
		"pw":       appPass,
	}
	var res struct {
		webAck
		Coin string `json:"coin"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/send", body, &res); err != nil {
		return "", err
	}
	return res.Coin, nil
}

// PostBond posts a fidelity bond. The reply carries no bond details; progress
// arrives on the notification feed. maintainTier applies to new accounts only.
func (c *WebClient) PostBond(ctx context.Context, host, cert string, bond uint64, assetID uint32, maintainTier *bool) error {
	body := map[string]any{
		"addr":  host,
		"pass":  "",
		"bond":  bond,
		"asset": assetID,
	}
	if cert != "" {
		body["cert"] = cert
	}
	if maintainTier != nil {
		body["maintain"] = *maintainTier
	}
	return c.callSession(ctx, http.MethodPost, "/api/postbond", body, nil)
}

// OpenWallet unlocks the asset's wallet; the empty pass field is the
// app-password slot, resolved from the session cache.
func (c *WebClient) OpenWallet(ctx context.Context, assetID uint32) error {
	return c.callSession(ctx, http.MethodPost, "/api/openwallet", map[string]any{"assetID": assetID, "pass": ""}, nil)
}

// NewWallet creates and unlocks an asset wallet. The form is flat: assetID,
// walletType and config sit beside pass (wallet) and appPass (session slot).
func (c *WebClient) NewWallet(ctx context.Context, assetID uint32, walletType string, cfg map[string]string, walletPass string) error {
	body := map[string]any{
		"assetID":    assetID,
		"walletType": walletType,
		"config":     cfg,
		"pass":       walletPass,
		"appPass":    "",
	}
	return c.callSession(ctx, http.MethodPost, "/api/newwallet", body, nil)
}

// DiscoverAccount discovers or restores an account on a DEX server, returning
// true when it already exists and is paid.
func (c *WebClient) DiscoverAccount(ctx context.Context, addr, cert string) (bool, error) {
	body := map[string]any{"addr": addr, "pass": ""}
	if cert != "" {
		body["cert"] = cert
	}
	var res struct {
		webAck
		Paid bool `json:"paid"`
	}
	if err := c.callSession(ctx, http.MethodPost, "/api/discoveracct", body, &res); err != nil {
		return false, err
	}
	return res.Paid, nil
}
