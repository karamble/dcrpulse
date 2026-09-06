// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"dcrpulse/internal/dexassets"
	"dcrpulse/internal/rpc"
	"dcrpulse/pkg/bisonw"

	"github.com/decred/dcrd/dcrutil/v4"
)

type dexOrdersInput struct {
	Host string `json:"host,omitempty" jsonschema:"optional DEX host filter; empty returns all"`
}

type dexNotificationsInput struct {
	Count int `json:"count,omitempty" jsonschema:"max notifications to return (default 50)"`
}

type dexPlaceOrderInput struct {
	Host    string `json:"host" jsonschema:"DEX server host"`
	Base    uint32 `json:"base" jsonschema:"base asset id"`
	Quote   uint32 `json:"quote" jsonschema:"quote asset id"`
	Sell    bool   `json:"sell" jsonschema:"true to sell the base asset, false to buy it"`
	IsLimit bool   `json:"isLimit" jsonschema:"true for a limit order, false for market"`
	Qty     uint64 `json:"qty" jsonschema:"quantity in atomic units of the base asset"`
	Rate    uint64 `json:"rate,omitempty" jsonschema:"rate in atomic message-rate units (limit orders)"`
	TifNow  bool   `json:"tifNow,omitempty" jsonschema:"immediate (taker) if true, standing (maker) if false"`
}

type dexCancelInput struct {
	OrderID string `json:"orderId" jsonschema:"hex order id to cancel"`
}

// dexOrderDCROutlay reports the DCR that leaves the wallet when an order fills,
// or 0 when neither side of the market is DCR. Selling gives up the base asset;
// buying gives up the quote, whose quantity is derived from the rate for a limit
// order and is already quote-denominated for a market buy.
func dexOrderDCROutlay(in dexPlaceOrderInput) (int64, error) {
	switch {
	case in.Sell && in.Base == bisonw.AssetDCR:
		return dexAtomsToInt64(in.Qty)
	case !in.Sell && in.Quote == bisonw.AssetDCR:
		if !in.IsLimit {
			return dexAtomsToInt64(in.Qty)
		}
		if in.Rate == 0 {
			return 0, fmt.Errorf("rate is required for a limit order")
		}
		if in.Rate > math.MaxUint64/in.Qty {
			return 0, fmt.Errorf("order value is out of range")
		}
		return dexAtomsToInt64(in.Qty * in.Rate / dexRateEncodingFactor)
	}
	return 0, nil
}

func dexAtomsToInt64(v uint64) (int64, error) {
	if v > math.MaxInt64 {
		return 0, fmt.Errorf("amount is out of range")
	}
	return int64(v), nil
}

// dexSpendAction describes a dex.spend move in the operator's approval message.
func dexSpendAction(amountAtoms int64) string {
	if amountAtoms > 0 {
		return fmt.Sprintf("make a DEX spend of %s", dcrAmountStr(amountAtoms))
	}
	return "make a DEX spend"
}

type dexHostInput struct {
	Host string `json:"host" jsonschema:"DEX server host"`
}

type dexConfigInput struct {
	Host string `json:"host" jsonschema:"DEX server host to fetch the public configuration for"`
}

type dexPreOrderInput struct {
	Host    string            `json:"host" jsonschema:"DEX server host"`
	Base    uint32            `json:"base" jsonschema:"base asset id"`
	Quote   uint32            `json:"quote" jsonschema:"quote asset id"`
	Sell    bool              `json:"sell" jsonschema:"true to sell the base asset, false to buy it"`
	IsLimit bool              `json:"isLimit" jsonschema:"true for a limit order, false for market"`
	Qty     uint64            `json:"qty" jsonschema:"quantity in atomic units of the base asset"`
	Rate    uint64            `json:"rate,omitempty" jsonschema:"rate in atomic message-rate units (limit orders)"`
	TifNow  bool              `json:"tifNow,omitempty" jsonschema:"immediate (taker) if true, standing (maker) if false"`
	Options map[string]string `json:"options,omitempty" jsonschema:"optional per-asset order options"`
}

type dexMaxBuyInput struct {
	Host  string `json:"host" jsonschema:"DEX server host"`
	Base  uint32 `json:"base" jsonschema:"base asset id"`
	Quote uint32 `json:"quote" jsonschema:"quote asset id"`
	Rate  uint64 `json:"rate" jsonschema:"rate in atomic message-rate units"`
}

type dexMaxSellInput struct {
	Host  string `json:"host" jsonschema:"DEX server host"`
	Base  uint32 `json:"base" jsonschema:"base asset id"`
	Quote uint32 `json:"quote" jsonschema:"quote asset id"`
}

type dexOrderInput struct {
	ID string `json:"id" jsonschema:"hex order id"`
}

type dexOrdersHistoryInput struct {
	Host    string `json:"host,omitempty" jsonschema:"optional DEX host filter"`
	N       int    `json:"n,omitempty" jsonschema:"max orders to return (default 50)"`
	Offset  string `json:"offset,omitempty" jsonschema:"hex order id to page from (older orders)"`
	Status  string `json:"status,omitempty" jsonschema:"optional status filter: epoch, booked, executed, canceled, revoked"`
	BaseID  uint32 `json:"baseId,omitempty" jsonschema:"optional base asset id market filter (with quoteId)"`
	QuoteID uint32 `json:"quoteId,omitempty" jsonschema:"optional quote asset id market filter (with baseId)"`
}

type dexAssetInput struct {
	AssetID uint32 `json:"assetId" jsonschema:"asset id"`
}

type dexWalletTxsInput struct {
	AssetID uint32 `json:"assetId" jsonschema:"asset id"`
	N       int    `json:"n,omitempty" jsonschema:"max transactions to return"`
	RefID   string `json:"refId,omitempty" jsonschema:"tx-id cursor to page from"`
	Past    bool   `json:"past,omitempty" jsonschema:"page towards older transactions"`
}

type dexWalletTxInput struct {
	AssetID uint32 `json:"assetId" jsonschema:"asset id"`
	TxID    string `json:"txId" jsonschema:"transaction id"`
}

type dexAddressUsedInput struct {
	AssetID uint32 `json:"assetId" jsonschema:"asset id"`
	Address string `json:"address" jsonschema:"address to check"`
}

type dexEstimateSendFeeInput struct {
	AssetID  uint32  `json:"assetId" jsonschema:"asset id to send"`
	Value    float64 `json:"value" jsonschema:"amount in conventional units"`
	Address  string  `json:"address" jsonschema:"destination address"`
	Subtract bool    `json:"subtract,omitempty" jsonschema:"subtract the fee from the sent amount"`
}

type dexMarketInput struct {
	Host    string `json:"host" jsonschema:"DEX server host"`
	BaseID  uint32 `json:"baseId" jsonschema:"base asset id"`
	QuoteID uint32 `json:"quoteId" jsonschema:"quote asset id"`
}

type dexTradesInput struct {
	Host    string `json:"host" jsonschema:"DEX server host"`
	BaseID  uint32 `json:"baseId" jsonschema:"base asset id"`
	QuoteID uint32 `json:"quoteId" jsonschema:"quote asset id"`
	Limit   int    `json:"limit,omitempty" jsonschema:"max trades to return, newest first (default ~100)"`
}

type dexMarketSummaryInput struct {
	Host    string `json:"host,omitempty" jsonschema:"optional DEX host filter; empty summarizes all markets"`
	BaseID  uint32 `json:"baseId,omitempty" jsonschema:"optional base asset id (with quoteId) to summarize one market"`
	QuoteID uint32 `json:"quoteId,omitempty" jsonschema:"optional quote asset id (with baseId) to summarize one market"`
}

type dexCandlesInput struct {
	Host    string `json:"host" jsonschema:"DEX server host"`
	BaseID  uint32 `json:"baseId" jsonschema:"base asset id"`
	QuoteID uint32 `json:"quoteId" jsonschema:"quote asset id"`
	Dur     string `json:"dur,omitempty" jsonschema:"candle bin duration, one of the market's candleDurs (e.g. 24h, 1h, 5m); defaults to 24h"`
}

type dexMMRunLogsInput struct {
	Host      string `json:"host" jsonschema:"DEX server host"`
	BaseID    uint32 `json:"baseId" jsonschema:"base asset id"`
	QuoteID   uint32 `json:"quoteId" jsonschema:"quote asset id"`
	StartTime int64  `json:"startTime" jsonschema:"the bot run's start time (unix seconds)"`
	N         uint64 `json:"n,omitempty" jsonschema:"max events to return (default 50)"`
	RefID     uint64 `json:"refId,omitempty" jsonschema:"oldest event id already held, to page older events"`
}

type dexSetBondOptionsInput struct {
	Host         string `json:"host" jsonschema:"DEX server host"`
	TargetTier   *int   `json:"targetTier,omitempty" jsonschema:"auto-renew target tier; 0 disables auto-renewal; omit to leave unchanged"`
	MaxBondedDcr *int   `json:"maxBondedDcr,omitempty" jsonschema:"max bonded amount in atoms; omit to leave unchanged"`
	BondAssetID  *int   `json:"bondAssetId,omitempty" jsonschema:"bond asset id; omit to leave unchanged"`
	PenaltyComps *int   `json:"penaltyComps,omitempty" jsonschema:"penalty compensation count; omit to leave unchanged"`
}

type dexWalletAssetInput struct {
	AssetID uint32 `json:"assetId" jsonschema:"asset id"`
}

type dexToggleWalletInput struct {
	AssetID uint32 `json:"assetId" jsonschema:"asset id"`
	Disable bool   `json:"disable" jsonschema:"true to disable the wallet, false to enable it"`
}

type dexRescanWalletInput struct {
	AssetID uint32 `json:"assetId" jsonschema:"asset id"`
	Force   bool   `json:"force,omitempty" jsonschema:"force the rescan even when the wallet has active orders"`
}

type dexWalletPeerInput struct {
	AssetID uint32 `json:"assetId" jsonschema:"asset id"`
	Address string `json:"address" jsonschema:"peer address"`
}

type dexMMConfigInput struct {
	Config string `json:"config" jsonschema:"bot configuration as a JSON object (mm.BotConfig)"`
}

type dexMMCexConfigInput struct {
	Config string `json:"config" jsonschema:"CEX configuration as a JSON object (mm.CEXConfig: name, apiKey, apiSecret)"`
}

type dexMMRemoveConfigInput struct {
	Host    string `json:"host" jsonschema:"DEX server host"`
	BaseID  uint32 `json:"baseId" jsonschema:"base asset id"`
	QuoteID uint32 `json:"quoteId" jsonschema:"quote asset id"`
}

type dexMMStopInput struct {
	Host    string `json:"host" jsonschema:"DEX server host"`
	BaseID  uint32 `json:"baseId" jsonschema:"base asset id"`
	QuoteID uint32 `json:"quoteId" jsonschema:"quote asset id"`
}

type dexPostBondInput struct {
	Host         string `json:"host" jsonschema:"DEX server host"`
	Bond         uint64 `json:"bond" jsonschema:"bond amount in DCR atoms"`
	MaintainTier *bool  `json:"maintainTier,omitempty" jsonschema:"maintain the resulting tier with auto-renewal"`
}

// dexLocked is the actionable error returned when a DEX read or write needs a
// bisonw webserver session but the DEX is locked.
// cexConfigName pulls the exchange name out of a CEX config blob. The name is
// what the credentials are bound to upstream, where storing them replaces any
// existing entry for the same exchange, so the operator has to be told which one
// they are approving and the audit row has to say which one changed. A blob that
// names no exchange is refused rather than approved blind.
func cexConfigName(raw string) (string, error) {
	var cfg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return "", fmt.Errorf("config is not a JSON object: %w", err)
	}
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return "", fmt.Errorf("config names no exchange")
	}
	return name, nil
}

func dexLocked() error {
	return fmt.Errorf("DEX is locked; ask the user to unlock it in the dashboard")
}

// dexConvToAtoms converts a conventional amount of an asset to its atomic units,
// rounding to the nearest atom (Decred's atoms-per-coin as the fallback factor).
func dexConvToAtoms(amount float64, assetID uint32) uint64 {
	cf := dexassets.ConvFactor(assetID)
	if cf == 0 {
		cf = uint64(dcrutil.AtomsPerCoin)
	}
	return uint64(math.Round(amount * float64(cf)))
}

// dexRateEncodingFactor is DCRDEX's message-rate encoding factor
// (decred.org/dcrdex/dex/calc.RateEncodingFactor). A conventional exchange rate
// is msgRate / dexRateEncodingFactor * baseFactor / quoteFactor.
const dexRateEncodingFactor = 1e8

// dexMatchSummary mirrors orderbook.MatchSummary, the raw recent-match entry the
// order book snapshot carries (rate as a message-rate, qty in base atoms, stamp
// in unix milliseconds).
type dexMatchSummary struct {
	Rate  uint64 `json:"rate"`
	Qty   uint64 `json:"qty"`
	Stamp uint64 `json:"stamp"`
	Sell  bool   `json:"sell"`
}

// dexTrade is a recent match enriched with conventional rate and quantity
// alongside the raw atomic values.
type dexTrade struct {
	Rate      float64 `json:"rate"`
	MsgRate   uint64  `json:"msgRate"`
	Qty       float64 `json:"qty"`
	QtyAtomic uint64  `json:"qtyAtomic"`
	Sell      bool    `json:"sell"`
	Stamp     uint64  `json:"stamp"`
}

// dexRecentMatches extracts the recent-match cache from a core.MarketOrderBook
// snapshot and enriches each entry with conventional rate and quantity, newest
// first. It returns an empty slice when the snapshot carries no matches.
func dexRecentMatches(book json.RawMessage, baseID, quoteID uint32) []dexTrade {
	var mob struct {
		Book struct {
			RecentMatches []dexMatchSummary `json:"recentMatches"`
		} `json:"book"`
	}
	out := []dexTrade{}
	if len(book) == 0 || json.Unmarshal(book, &mob) != nil {
		return out
	}
	baseCF := float64(dexassets.ConvFactor(baseID))
	quoteCF := float64(dexassets.ConvFactor(quoteID))
	if baseCF == 0 {
		baseCF = float64(dcrutil.AtomsPerCoin)
	}
	if quoteCF == 0 {
		quoteCF = float64(dcrutil.AtomsPerCoin)
	}
	for _, m := range mob.Book.RecentMatches {
		out = append(out, dexTrade{
			Rate:      float64(m.Rate) / dexRateEncodingFactor * baseCF / quoteCF,
			MsgRate:   m.Rate,
			Qty:       float64(m.Qty) / baseCF,
			QtyAtomic: m.Qty,
			Sell:      m.Sell,
			Stamp:     m.Stamp,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stamp > out[j].Stamp })
	return out
}

// dexSpot mirrors the spot fields a market carries in the Exchanges snapshot: the
// last match message-rate plus 24h stats (rates as message-rates, volume in base
// atoms).
type dexSpot struct {
	Rate     uint64  `json:"rate"`
	Change24 float64 `json:"change24"`
	High24   uint64  `json:"high24"`
	Low24    uint64  `json:"low24"`
	Vol24    uint64  `json:"vol24"`
	Stamp    uint64  `json:"stamp"`
}

// dexMarketSummary is a one-shot market snapshot: last trade rate (conventional, and
// USD when the quote can be priced) plus 24h stats, built from the Exchanges spot
// data and the USD rate feed with no per-market order book calls.
type dexMarketSummary struct {
	Host        string   `json:"host"`
	Market      string   `json:"market"`
	BaseID      uint32   `json:"baseId"`
	BaseSymbol  string   `json:"baseSymbol"`
	QuoteID     uint32   `json:"quoteId"`
	QuoteSymbol string   `json:"quoteSymbol"`
	LastRate    float64  `json:"lastRate"`
	LastRateUSD *float64 `json:"lastRateUsd,omitempty"`
	Change24    float64  `json:"change24"`
	High24      float64  `json:"high24"`
	Low24       float64  `json:"low24"`
	Vol24Base   float64  `json:"vol24Base"`
	Stamp       uint64   `json:"stamp"`
}

// dexQuoteUSD resolves a market quote asset's USD price from the DCR/BTC rate feed,
// treating USD-pegged stablecoins as $1. Returns false when the quote can't be priced.
func dexQuoteUSD(symbol string, dcrUSD, btcUSD float64) (float64, bool) {
	switch {
	case symbol == "btc":
		return btcUSD, btcUSD > 0
	case symbol == "dcr":
		return dcrUSD, dcrUSD > 0
	case strings.HasPrefix(symbol, "usdc"), strings.HasPrefix(symbol, "usdt"), strings.HasPrefix(symbol, "dai"):
		return 1.0, true
	}
	return 0, false
}

// dexMarketSummaries builds per-market summaries from an Exchanges snapshot and the
// USD rate feed, optionally narrowed to one host and/or one base/quote market.
func dexMarketSummaries(exch, rates json.RawMessage, host string, baseID, quoteID uint32) []dexMarketSummary {
	var exMap map[string]struct {
		Markets map[string]struct {
			BaseID      uint32   `json:"baseid"`
			BaseSymbol  string   `json:"basesymbol"`
			QuoteID     uint32   `json:"quoteid"`
			QuoteSymbol string   `json:"quotesymbol"`
			Spot        *dexSpot `json:"spot"`
		} `json:"markets"`
	}
	out := []dexMarketSummary{}
	if len(exch) == 0 || json.Unmarshal(exch, &exMap) != nil {
		return out
	}
	var r struct {
		DcrUSD float64 `json:"dcr_usd"`
		BtcUSD float64 `json:"btc_usd"`
	}
	_ = json.Unmarshal(rates, &r)
	for h, ex := range exMap {
		if host != "" && h != host {
			continue
		}
		for name, m := range ex.Markets {
			if m.Spot == nil {
				continue
			}
			if (baseID != 0 || quoteID != 0) && (m.BaseID != baseID || m.QuoteID != quoteID) {
				continue
			}
			baseCF := float64(dexassets.ConvFactor(m.BaseID))
			quoteCF := float64(dexassets.ConvFactor(m.QuoteID))
			if baseCF == 0 {
				baseCF = float64(dcrutil.AtomsPerCoin)
			}
			if quoteCF == 0 {
				quoteCF = float64(dcrutil.AtomsPerCoin)
			}
			conv := func(msgRate uint64) float64 {
				return float64(msgRate) / dexRateEncodingFactor * baseCF / quoteCF
			}
			s := dexMarketSummary{
				Host:        h,
				Market:      name,
				BaseID:      m.BaseID,
				BaseSymbol:  m.BaseSymbol,
				QuoteID:     m.QuoteID,
				QuoteSymbol: m.QuoteSymbol,
				LastRate:    conv(m.Spot.Rate),
				Change24:    m.Spot.Change24,
				High24:      conv(m.Spot.High24),
				Low24:       conv(m.Spot.Low24),
				Vol24Base:   float64(m.Spot.Vol24) / baseCF,
				Stamp:       m.Spot.Stamp,
			}
			if qUSD, ok := dexQuoteUSD(m.QuoteSymbol, r.DcrUSD, r.BtcUSD); ok {
				usd := s.LastRate * qUSD
				s.LastRateUSD = &usd
			}
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		return out[i].Market < out[j].Market
	})
	return out
}

// dexTools are the dex domain tools: reads that need no write scope (some need
// the DEX unlocked), grant-gated wallet/account/market-maker write actions
// (scopeDex), and grant-gated fund moves (scopeDexSpend).
var dexTools = []toolDef{
	readTool("dex", "dex_version",
		"Get the bisonw (DCRDEX) version and confirm the DEX client is reachable.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			return c.Version(ctx)
		}),
	readTool("dex", "dex_exchanges",
		"List the configured DEX servers and their markets.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			return c.Exchanges(ctx)
		}),
	readTool("dex", "dex_wallets",
		"List the DEX wallets and their balances and state.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			return c.Wallets(ctx)
		}),
	readTool("dex", "dex_wallet",
		"Get a single DEX wallet's balance and state by asset id.",
		func(ctx context.Context, in dexWalletAssetInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			has, err := c.HasWallet(ctx, in.AssetID)
			if err != nil {
				return nil, err
			}
			if !has {
				return nil, fmt.Errorf("no DEX wallet configured for asset %d", in.AssetID)
			}
			return c.Wallets(ctx)
		}),
	readTool("dex", "dex_orders",
		"List the user's active and recent DEX orders. Optional 'host' to filter by DEX server.",
		func(ctx context.Context, in dexOrdersInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			return c.MyOrders(ctx, in.Host)
		}),
	readTool("dex", "dex_notifications",
		"List recent DEX notifications. Optional count (default 50).",
		func(ctx context.Context, in dexNotificationsInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			n := in.Count
			if n <= 0 {
				n = 50
			}
			return c.Notifications(ctx, n)
		}),
	readTool("dex", "dex_config",
		"Fetch a DEX server's public configuration (markets, bond requirements). Provide the server host.",
		func(ctx context.Context, in dexConfigInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			return c.GetDEXConfig(ctx, in.Host, "")
		}),
	readTool("dex", "dex_account",
		"Get the per-server account state (tier, reputation, bonds) for a registered DEX host.",
		func(ctx context.Context, in dexHostInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			return c.Exchanges(ctx)
		}),
	readTool("dex", "dex_preorder",
		"Get the pre-order estimate (swap/redeem fee estimates and order options) for a prospective order. Requires the DEX unlocked.",
		func(ctx context.Context, in dexPreOrderInput) (any, error) {
			if !rpc.DcrdexUnlocked() {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.PreOrder(ctx, in.Host, in.IsLimit, in.Sell, in.Base, in.Quote, in.Qty, in.Rate, in.TifNow, in.Options)
		}),
	readTool("dex", "dex_max_buy",
		"Get the largest buy order fundable at the given rate on a market, with fee estimates. Requires the DEX unlocked.",
		func(ctx context.Context, in dexMaxBuyInput) (any, error) {
			if !rpc.DcrdexUnlocked() {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.MaxBuy(ctx, in.Host, in.Base, in.Quote, in.Rate)
		}),
	readTool("dex", "dex_max_sell",
		"Get the largest sell order fundable on a market, with fee estimates. Requires the DEX unlocked.",
		func(ctx context.Context, in dexMaxSellInput) (any, error) {
			if !rpc.DcrdexUnlocked() {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.MaxSell(ctx, in.Host, in.Base, in.Quote)
		}),
	readTool("dex", "dex_order",
		"Get a single order by its hex id, including live swap-coin confirmation counts. Requires the DEX unlocked.",
		func(ctx context.Context, in dexOrderInput) (any, error) {
			if !rpc.DcrdexUnlocked() {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.Order(ctx, in.ID)
		}),
	readTool("dex", "dex_orders_history",
		"Get the user's full order history, including canceled/executed/revoked orders, with optional status/market filters and pagination. Requires the DEX unlocked.",
		func(ctx context.Context, in dexOrdersHistoryInput) (any, error) {
			if !rpc.DcrdexUnlocked() {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			n := in.N
			if n <= 0 {
				n = 50
			}
			filter := map[string]any{"n": n}
			if in.Host != "" {
				filter["hosts"] = []string{in.Host}
			}
			if in.Offset != "" {
				filter["offset"] = in.Offset
			}
			if in.Status != "" {
				filter["statuses"] = []string{in.Status}
			}
			if in.BaseID != 0 || in.QuoteID != 0 {
				filter["market"] = map[string]any{"baseID": in.BaseID, "quoteID": in.QuoteID}
			}
			return c.Orders(ctx, filter)
		}),
	readTool("dex", "dex_assets",
		"Get the DCRDEX supported-asset catalog (wallet definitions and config-option schemas).",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return dexassets.Raw(), nil
		}),
	readTool("dex", "dex_rates",
		"Get current USD prices for DEX assets, from Kraken with a Bison Relay fallback for DCR and BTC.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			return rpc.BrclientdRates(ctx)
		}),
	readTool("dex", "dex_orderbook",
		"Get the live order book for a market: buy and sell depth, current-epoch orders, and recent matches. Each order carries both conventional (rate, qty) and atomic (msgRate, qtyAtomic) values. Public market data; the DEX need not be unlocked.",
		func(ctx context.Context, in dexMarketInput) (any, error) {
			raw, err := rpc.DcrdexOrderBook(ctx, in.Host, in.BaseID, in.QuoteID)
			if err != nil {
				return nil, err
			}
			return raw, nil
		}),
	readTool("dex", "dex_trades",
		"Get a market's recent public trade history (up to ~100 most recent matches), newest first. Each trade reports the conventional rate and base quantity, the atomic message-rate, the side (true=sell), and the match timestamp (unix milliseconds). Pass limit to cap the number returned. Public market data; the DEX need not be unlocked.",
		func(ctx context.Context, in dexTradesInput) (any, error) {
			raw, err := rpc.DcrdexOrderBook(ctx, in.Host, in.BaseID, in.QuoteID)
			if err != nil {
				return nil, err
			}
			trades := dexRecentMatches(raw, in.BaseID, in.QuoteID)
			if in.Limit > 0 && len(trades) > in.Limit {
				trades = trades[:in.Limit]
			}
			return trades, nil
		}),
	readTool("dex", "dex_candles",
		"Get a market's candlestick (OHLC) history for a bin duration (default 24h, or one of the market's candleDurs such as 1h or 5m). Each candle reports start/end timestamps, start/end/high/low message-rates, and the match and quote volumes. Public market data; the DEX need not be unlocked.",
		func(ctx context.Context, in dexCandlesInput) (any, error) {
			dur := in.Dur
			if dur == "" {
				dur = "24h"
			}
			raw, err := rpc.DcrdexCandles(ctx, in.Host, in.BaseID, in.QuoteID, dur)
			if err != nil {
				return nil, err
			}
			return raw, nil
		}),
	readTool("dex", "dex_market_summary",
		"Get a one-shot market summary: last trade rate (conventional, plus USD when the quote can be priced), 24h high/low/change, and 24h base volume. Built from the exchange spot data and the USD rate feed in a single call. With no host it summarizes every market; pass host (and optionally baseId+quoteId) to narrow to one. Public market data; the DEX need not be unlocked.",
		func(ctx context.Context, in dexMarketSummaryInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			exch, err := c.Exchanges(ctx)
			if err != nil {
				return nil, err
			}
			rates, _ := rpc.BrclientdRates(ctx)
			return dexMarketSummaries(exch, rates, in.Host, in.BaseID, in.QuoteID), nil
		}),
	readTool("dex", "dex_deposit_address",
		"Get a fresh deposit address for a DEX wallet by asset id. Requires the DEX unlocked.",
		func(ctx context.Context, in dexAssetInput) (any, error) {
			if !rpc.DcrdexUnlocked() {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			addr, err := c.NewDepositAddress(ctx, in.AssetID)
			if err != nil {
				return nil, err
			}
			return map[string]string{"address": addr}, nil
		}),
	readTool("dex", "dex_address_used",
		"Check whether a DEX wallet address has already received funds, to avoid address reuse. Requires the DEX unlocked.",
		func(ctx context.Context, in dexAddressUsedInput) (any, error) {
			if !rpc.DcrdexUnlocked() {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			used, err := c.AddressUsed(ctx, in.AssetID, in.Address)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"used": used}, nil
		}),
	readTool("dex", "dex_estimate_send_fee",
		"Estimate the network fee to send an amount of an asset to an address, and validate the address. Requires the DEX unlocked.",
		func(ctx context.Context, in dexEstimateSendFeeInput) (any, error) {
			if !rpc.DcrdexUnlocked() {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			atoms := dexConvToAtoms(in.Value, in.AssetID)
			txFee, validAddr, err := c.EstimateSendTxFee(ctx, in.AssetID, in.Address, atoms, in.Subtract, false)
			if err != nil {
				return nil, err
			}
			return map[string]any{"feeAtoms": txFee, "validAddress": validAddr}, nil
		}),
	readTool("dex", "dex_wallet_txs",
		"Get a DEX wallet's transaction history by asset id, with pagination.",
		func(ctx context.Context, in dexWalletTxsInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			return c.TxHistory(ctx, in.AssetID, in.N, in.RefID, in.Past)
		}),
	readTool("dex", "dex_wallet_tx",
		"Get a single DEX wallet transaction by asset id and transaction id.",
		func(ctx context.Context, in dexWalletTxInput) (any, error) {
			c, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			return c.WalletTx(ctx, in.AssetID, in.TxID)
		}),
	readTool("dex", "dex_mm_status",
		"Get the market-making status (bots and CEX state). CEX API credentials are not returned. Requires the DEX unlocked.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			if !rpc.DcrdexUnlocked() {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.MMStatus(ctx)
		}),
	readTool("dex", "dex_mm_market_report",
		"Get the market report (oracle prices and fiat rates) for a market. Requires the DEX unlocked.",
		func(ctx context.Context, in dexMarketInput) (any, error) {
			if !rpc.DcrdexUnlocked() {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.MarketReport(ctx, in.Host, in.BaseID, in.QuoteID)
		}),
	readTool("dex", "dex_mm_run_logs",
		"Get a market-maker run's event log (DEX/CEX orders, deposits, withdrawals) and overview. Requires the DEX unlocked.",
		func(ctx context.Context, in dexMMRunLogsInput) (any, error) {
			if !rpc.DcrdexUnlocked() {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			n := in.N
			if n == 0 {
				n = 50
			}
			var refID *uint64
			if in.RefID != 0 {
				refID = &in.RefID
			}
			return c.RunLogs(ctx, in.Host, in.BaseID, in.QuoteID, in.StartTime, n, refID)
		}),
	readTool("dex", "dex_mm_archived_runs",
		"Get the market-maker run history (past runs, newest first). Requires the DEX unlocked.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			if !rpc.DcrdexUnlocked() {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.ArchivedRuns(ctx)
		}),
	agentTool("dex", "dex_place_order",
		"Place a DCRDEX trade order. Requires a spend grant with DEX trading enabled and the DEX unlocked. Quantity and rate are in atomic units.",
		func(ctx context.Context, a *agent, in dexPlaceOrderInput) (any, error) {
			detail := fmt.Sprintf("base=%d quote=%d qty=%d", in.Base, in.Quote, in.Qty)
			if in.Qty == 0 {
				return nil, fmt.Errorf("qty must be positive")
			}
			// An order commits funds, so reserve the DCR side against the grant's
			// caps. A market with no DCR side is scope-gated only, as for a
			// non-DCR dex_send: the DCR cap cannot bound a non-DCR amount.
			outlay, err := dexOrderDCROutlay(in)
			if err != nil {
				return nil, err
			}
			amountDCR := dcrutil.Amount(outlay).ToCoin()
			if err := grants.authorizeSpendScoped(ctx, a.id, scopeDex, outlay, dexSpendAction(outlay), time.Now()); err != nil {
				if tripwire(a.id, err) {
					recordSpend(a, "dex_place_order", 0, amountDCR, in.Host, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "dex_place_order", 0, amountDCR, in.Host, "denied", err.Error())
				}
				return nil, err
			}
			if !rpc.DcrdexUnlocked() {
				grants.refund(a.id, outlay)
				err := dexLocked()
				recordSpend(a, "dex_place_order", 0, amountDCR, in.Host, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexWebClient()
			if err != nil {
				grants.refund(a.id, outlay)
				return nil, err
			}
			raw, err := client.Trade(ctx, in.Host, in.IsLimit, in.Sell, in.Base, in.Quote, in.Qty, in.Rate, in.TifNow, nil)
			if err != nil {
				grants.refund(a.id, outlay)
				recordSpend(a, "dex_place_order", 0, amountDCR, in.Host, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_place_order", 0, amountDCR, in.Host, "ok", detail)
			return raw, nil
		}),
	agentTool("dex", "dex_cancel_order",
		"Cancel a standing DCRDEX order. Requires a spend grant with DEX trading enabled.",
		func(ctx context.Context, a *agent, in dexCancelInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_cancel_order", 0, 0, in.OrderID, "denied", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			if err := client.Cancel(ctx, in.OrderID); err != nil {
				recordSpend(a, "dex_cancel_order", 0, 0, in.OrderID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_cancel_order", 0, 0, in.OrderID, "ok", "")
			return map[string]any{"orderId": in.OrderID, "ok": true}, nil
		}),
	agentTool("dex", "dex_set_bond_options",
		"Update a DEX account's auto-bond options (target tier, max bonded, bond asset, penalty comps). Omit a field to leave it unchanged; targetTier 0 disables auto-renewal. Requires a spend grant with DEX send/post-bond enabled and the DEX unlocked, and the user's approval when Bison Relay oversight is on.",
		func(ctx context.Context, a *agent, in dexSetBondOptionsInput) (any, error) {
			// Auto-renewal posts bonds on its own, so this belongs with the
			// explicit bond scope rather than with plain trading. The bonds it
			// posts never pass a cap check, so the operator approves the arming
			// instead; every call gates, including a disarm, because the options
			// interact and a partial rule is harder to reason about.
			action := fmt.Sprintf("update DEX auto-bond options on %s", in.Host)
			if in.TargetTier != nil {
				action = fmt.Sprintf("set the DEX auto-bond target tier on %s to %d", in.Host, *in.TargetTier)
			}
			if err := grants.authorizeActionGated(ctx, a.id, scopeDexSpend, action, time.Now()); err != nil {
				recordSpend(a, "dex_set_bond_options", 0, 0, in.Host, "denied", err.Error())
				return nil, err
			}
			if !rpc.DcrdexUnlocked() {
				err := dexLocked()
				recordSpend(a, "dex_set_bond_options", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			// -1 leaves an option unchanged (see bisonw.SetBondOptions).
			targetTier, maxBonded, bondAsset, penaltyComps := -1, -1, -1, -1
			if in.TargetTier != nil {
				targetTier = *in.TargetTier
			}
			if in.MaxBondedDcr != nil {
				maxBonded = *in.MaxBondedDcr
			}
			if in.BondAssetID != nil {
				bondAsset = *in.BondAssetID
			}
			if in.PenaltyComps != nil {
				penaltyComps = *in.PenaltyComps
			}
			if err := client.SetBondOptions(ctx, in.Host, targetTier, maxBonded, bondAsset, penaltyComps); err != nil {
				recordSpend(a, "dex_set_bond_options", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_set_bond_options", 0, 0, in.Host, "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_wallet_open",
		"Unlock a DEX wallet by asset id. Requires a spend grant with DEX trading enabled and the DEX unlocked.",
		func(ctx context.Context, a *agent, in dexWalletAssetInput) (any, error) {
			target := fmt.Sprintf("asset=%d", in.AssetID)
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_wallet_open", 0, 0, target, "denied", err.Error())
				return nil, err
			}
			if !rpc.DcrdexUnlocked() {
				err := dexLocked()
				recordSpend(a, "dex_wallet_open", 0, 0, target, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			if err := client.OpenWallet(ctx, in.AssetID); err != nil {
				recordSpend(a, "dex_wallet_open", 0, 0, target, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_wallet_open", 0, 0, target, "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_wallet_close",
		"Lock a DEX wallet by asset id. Requires a spend grant with DEX trading enabled.",
		func(ctx context.Context, a *agent, in dexWalletAssetInput) (any, error) {
			target := fmt.Sprintf("asset=%d", in.AssetID)
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_wallet_close", 0, 0, target, "denied", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			if err := client.CloseWallet(ctx, in.AssetID); err != nil {
				recordSpend(a, "dex_wallet_close", 0, 0, target, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_wallet_close", 0, 0, target, "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_wallet_toggle",
		"Enable or disable a DEX wallet by asset id. Requires a spend grant with DEX trading enabled.",
		func(ctx context.Context, a *agent, in dexToggleWalletInput) (any, error) {
			target := fmt.Sprintf("asset=%d disable=%t", in.AssetID, in.Disable)
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_wallet_toggle", 0, 0, target, "denied", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			if err := client.ToggleWalletStatus(ctx, in.AssetID, in.Disable); err != nil {
				recordSpend(a, "dex_wallet_toggle", 0, 0, target, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_wallet_toggle", 0, 0, target, "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_wallet_rescan",
		"Trigger a rescan of a DEX wallet by asset id. Requires a spend grant with DEX trading enabled.",
		func(ctx context.Context, a *agent, in dexRescanWalletInput) (any, error) {
			target := fmt.Sprintf("asset=%d", in.AssetID)
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_wallet_rescan", 0, 0, target, "denied", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			if err := client.RescanWallet(ctx, in.AssetID, in.Force); err != nil {
				recordSpend(a, "dex_wallet_rescan", 0, 0, target, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_wallet_rescan", 0, 0, target, "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_add_peer",
		"Add a persistent peer to a DEX wallet. Requires a spend grant with DEX trading enabled.",
		func(ctx context.Context, a *agent, in dexWalletPeerInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_add_peer", 0, 0, in.Address, "denied", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			if err := client.AddWalletPeer(ctx, in.AssetID, in.Address); err != nil {
				recordSpend(a, "dex_add_peer", 0, 0, in.Address, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_add_peer", 0, 0, in.Address, "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_remove_peer",
		"Remove a persistent peer from a DEX wallet. Requires a spend grant with DEX trading enabled.",
		func(ctx context.Context, a *agent, in dexWalletPeerInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_remove_peer", 0, 0, in.Address, "denied", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			if err := client.RemoveWalletPeer(ctx, in.AssetID, in.Address); err != nil {
				recordSpend(a, "dex_remove_peer", 0, 0, in.Address, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_remove_peer", 0, 0, in.Address, "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_discover_account",
		"Re-discover the account on a DEX server (after a seed restore) and report whether it is already paid. Requires a spend grant with DEX trading enabled and the DEX unlocked.",
		func(ctx context.Context, a *agent, in dexHostInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_discover_account", 0, 0, in.Host, "denied", err.Error())
				return nil, err
			}
			if !rpc.DcrdexUnlocked() {
				err := dexLocked()
				recordSpend(a, "dex_discover_account", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			paid, err := client.DiscoverAccount(ctx, in.Host, "")
			if err != nil {
				recordSpend(a, "dex_discover_account", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_discover_account", 0, 0, in.Host, "ok", fmt.Sprintf("paid=%t", paid))
			return map[string]bool{"paid": paid}, nil
		}),
	agentTool("dex", "dex_mm_update_config",
		"Persist (and validate) a market-maker bot config. Requires a spend grant with DEX trading enabled and the DEX unlocked.",
		func(ctx context.Context, a *agent, in dexMMConfigInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_mm_update_config", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			if !rpc.DcrdexUnlocked() {
				err := dexLocked()
				recordSpend(a, "dex_mm_update_config", 0, 0, "", "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			if err := client.UpdateBotConfig(ctx, []byte(in.Config)); err != nil {
				recordSpend(a, "dex_mm_update_config", 0, 0, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_mm_update_config", 0, 0, "", "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_mm_remove_config",
		"Delete a stored market-maker bot config for a market. Requires a spend grant with DEX trading enabled and the DEX unlocked.",
		func(ctx context.Context, a *agent, in dexMMRemoveConfigInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_mm_remove_config", 0, 0, in.Host, "denied", err.Error())
				return nil, err
			}
			if !rpc.DcrdexUnlocked() {
				err := dexLocked()
				recordSpend(a, "dex_mm_remove_config", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			if err := client.RemoveBotConfig(ctx, in.Host, in.BaseID, in.QuoteID); err != nil {
				recordSpend(a, "dex_mm_remove_config", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_mm_remove_config", 0, 0, in.Host, "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_mm_update_cex_config",
		"Store (and validate) CEX API credentials for the market maker. Requires a spend grant with DEX trading enabled, the DEX unlocked, and the operator's approval: these credentials decide which exchange account a market-maker bot deposits into.",
		func(ctx context.Context, a *agent, in dexMMCexConfigInput) (any, error) {
			// Scope first, so an agent without the grant is refused by the gate
			// rather than by input validation, like ln_pay's pre-decode reject.
			if err := grants.precheckScope(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_mm_update_cex_config", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			// Then the name, before the approval: the prompt and the audit row
			// are worth nothing if neither says which exchange changed.
			name, err := cexConfigName(in.Config)
			if err != nil {
				recordSpend(a, "dex_mm_update_cex_config", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			// Gated rather than a plain scope check. Storing these credentials
			// arms where an autonomous spender sends funds: a bot the operator
			// starts later reads the deposit address from the exchange using
			// whichever key is stored here, and that send never passes a DCR cap.
			// One place carries the exchange name into the trail, so no outcome
			// can end up recording a write without saying what was rewritten.
			audit := func(result, detail string) {
				recordSpend(a, "dex_mm_update_cex_config", 0, 0, name, result, detail)
			}
			action := fmt.Sprintf("store API credentials for the %s exchange, which sets where a market-maker bot deposits funds", name)
			if err := grants.authorizeActionGated(ctx, a.id, scopeDex, action, time.Now()); err != nil {
				audit("denied", err.Error())
				return nil, err
			}
			if !rpc.DcrdexUnlocked() {
				err := dexLocked()
				audit("error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			if err := client.UpdateCEXConfig(ctx, []byte(in.Config)); err != nil {
				audit("error", err.Error())
				return nil, err
			}
			audit("ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_mm_stop",
		"Stop a running market-maker bot on a market. Requires a spend grant with DEX trading enabled and the DEX unlocked.",
		func(ctx context.Context, a *agent, in dexMMStopInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_mm_stop", 0, 0, in.Host, "denied", err.Error())
				return nil, err
			}
			if !rpc.DcrdexUnlocked() {
				err := dexLocked()
				recordSpend(a, "dex_mm_stop", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			if err := client.StopBot(ctx, in.Host, in.BaseID, in.QuoteID); err != nil {
				recordSpend(a, "dex_mm_stop", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_mm_stop", 0, 0, in.Host, "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_post_bond",
		"Post a fidelity bond (in DCR) to register or maintain a DEX account. Requires a spend grant with DEX send/post-bond enabled and the DEX unlocked. The bond counts against the grant's DCR daily cap.",
		func(ctx context.Context, a *agent, in dexPostBondInput) (any, error) {
			// Bound before the int64 conversion: a bond above MaxInt64 would
			// wrap negative and skip the cap reservation entirely.
			if in.Bond == 0 || in.Bond > math.MaxInt64 {
				return nil, fmt.Errorf("bond must be positive and below %d atoms", int64(math.MaxInt64))
			}
			capAtoms := int64(in.Bond)
			amountDCR := dcrutil.Amount(capAtoms).ToCoin()
			if err := grants.authorizeSpendScoped(ctx, a.id, scopeDexSpend, capAtoms, dexSpendAction(capAtoms), time.Now()); err != nil {
				if tripwire(a.id, err) {
					recordSpend(a, "dex_post_bond", 0, amountDCR, in.Host, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "dex_post_bond", 0, amountDCR, in.Host, "denied", err.Error())
				}
				return nil, err
			}
			if !rpc.DcrdexUnlocked() {
				grants.refund(a.id, capAtoms)
				err := dexLocked()
				recordSpend(a, "dex_post_bond", 0, amountDCR, in.Host, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexWebClient()
			if err != nil {
				grants.refund(a.id, capAtoms)
				return nil, err
			}
			if err := client.PostBond(ctx, in.Host, "", in.Bond, bisonw.AssetDCR, in.MaintainTier); err != nil {
				grants.refund(a.id, capAtoms)
				recordSpend(a, "dex_post_bond", 0, amountDCR, in.Host, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_post_bond", 0, amountDCR, in.Host, "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
}
