// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"fmt"
	"math"
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

type dexSendInput struct {
	AssetID uint32  `json:"assetId" jsonschema:"asset id to send"`
	Value   float64 `json:"value" jsonschema:"amount in conventional units"`
	Address string  `json:"address" jsonschema:"destination address"`
}

type dexPostBondInput struct {
	Host         string `json:"host" jsonschema:"DEX server host"`
	Bond         uint64 `json:"bond" jsonschema:"bond amount in DCR atoms"`
	MaintainTier *bool  `json:"maintainTier,omitempty" jsonschema:"maintain the resulting tier with auto-renewal"`
}

// dexLocked is the actionable error returned when a DEX read or write needs the
// in-memory app password but the DEX session is locked.
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
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.PreOrder(ctx, appPass, in.Host, in.IsLimit, in.Sell, in.Base, in.Quote, in.Qty, in.Rate, in.TifNow, in.Options)
		}),
	readTool("dex", "dex_max_buy",
		"Get the largest buy order fundable at the given rate on a market, with fee estimates. Requires the DEX unlocked.",
		func(ctx context.Context, in dexMaxBuyInput) (any, error) {
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.MaxBuy(ctx, appPass, in.Host, in.Base, in.Quote, in.Rate)
		}),
	readTool("dex", "dex_max_sell",
		"Get the largest sell order fundable on a market, with fee estimates. Requires the DEX unlocked.",
		func(ctx context.Context, in dexMaxSellInput) (any, error) {
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.MaxSell(ctx, appPass, in.Host, in.Base, in.Quote)
		}),
	readTool("dex", "dex_order",
		"Get a single order by its hex id, including live swap-coin confirmation counts. Requires the DEX unlocked.",
		func(ctx context.Context, in dexOrderInput) (any, error) {
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.Order(ctx, appPass, in.ID)
		}),
	readTool("dex", "dex_orders_history",
		"Get the user's full order history, including canceled/executed/revoked orders, with optional status/market filters and pagination. Requires the DEX unlocked.",
		func(ctx context.Context, in dexOrdersHistoryInput) (any, error) {
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
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
			return c.Orders(ctx, appPass, filter)
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
	readTool("dex", "dex_deposit_address",
		"Get a fresh deposit address for a DEX wallet by asset id. Requires the DEX unlocked.",
		func(ctx context.Context, in dexAssetInput) (any, error) {
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			addr, err := c.NewDepositAddress(ctx, appPass, in.AssetID)
			if err != nil {
				return nil, err
			}
			return map[string]string{"address": addr}, nil
		}),
	readTool("dex", "dex_address_used",
		"Check whether a DEX wallet address has already received funds, to avoid address reuse. Requires the DEX unlocked.",
		func(ctx context.Context, in dexAddressUsedInput) (any, error) {
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			used, err := c.AddressUsed(ctx, appPass, in.AssetID, in.Address)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"used": used}, nil
		}),
	readTool("dex", "dex_estimate_send_fee",
		"Estimate the network fee to send an amount of an asset to an address, and validate the address. Requires the DEX unlocked.",
		func(ctx context.Context, in dexEstimateSendFeeInput) (any, error) {
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			atoms := dexConvToAtoms(in.Value, in.AssetID)
			txFee, validAddr, err := c.EstimateSendTxFee(ctx, appPass, in.AssetID, in.Address, atoms, in.Subtract, false)
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
		"Get the market-making status (bots and CEX state). Requires the DEX unlocked.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.MMStatus(ctx, appPass)
		}),
	readTool("dex", "dex_mm_market_report",
		"Get the market report (oracle prices and fiat rates) for a market. Requires the DEX unlocked.",
		func(ctx context.Context, in dexMarketInput) (any, error) {
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.MarketReport(ctx, appPass, in.Host, in.BaseID, in.QuoteID)
		}),
	readTool("dex", "dex_mm_run_logs",
		"Get a market-maker run's event log (DEX/CEX orders, deposits, withdrawals) and overview. Requires the DEX unlocked.",
		func(ctx context.Context, in dexMMRunLogsInput) (any, error) {
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
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
			return c.RunLogs(ctx, appPass, in.Host, in.BaseID, in.QuoteID, in.StartTime, n, refID)
		}),
	readTool("dex", "dex_mm_archived_runs",
		"Get the market-maker run history (past runs, newest first). Requires the DEX unlocked.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				return nil, dexLocked()
			}
			c, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			return c.ArchivedRuns(ctx, appPass)
		}),
	agentTool("dex", "dex_place_order",
		"Place a DCRDEX trade order. Requires a spend grant with DEX trading enabled and the DEX unlocked. Quantity and rate are in atomic units.",
		func(ctx context.Context, a *agent, in dexPlaceOrderInput) (any, error) {
			detail := fmt.Sprintf("base=%d quote=%d qty=%d", in.Base, in.Quote, in.Qty)
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_place_order", 0, 0, in.Host, "denied", err.Error())
				return nil, err
			}
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				err := dexLocked()
				recordSpend(a, "dex_place_order", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			raw, err := client.Trade(ctx, bisonw.TradeParams{
				AppPass: appPass,
				Host:    in.Host,
				IsLimit: in.IsLimit,
				Sell:    in.Sell,
				Base:    in.Base,
				Quote:   in.Quote,
				Qty:     in.Qty,
				Rate:    in.Rate,
				TifNow:  in.TifNow,
			})
			if err != nil {
				recordSpend(a, "dex_place_order", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_place_order", 0, 0, in.Host, "ok", detail)
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
		"Update a DEX account's auto-bond options (target tier, max bonded, bond asset, penalty comps). Omit a field to leave it unchanged; targetTier 0 disables auto-renewal. Requires a spend grant with DEX trading enabled and the DEX unlocked.",
		func(ctx context.Context, a *agent, in dexSetBondOptionsInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_set_bond_options", 0, 0, in.Host, "denied", err.Error())
				return nil, err
			}
			if _, ok := rpc.DcrdexAppPass(); !ok {
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
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				err := dexLocked()
				recordSpend(a, "dex_wallet_open", 0, 0, target, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			if err := client.OpenWallet(ctx, appPass, in.AssetID); err != nil {
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
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				err := dexLocked()
				recordSpend(a, "dex_discover_account", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				return nil, err
			}
			paid, err := client.DiscoverAccount(ctx, appPass, in.Host, "")
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
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				err := dexLocked()
				recordSpend(a, "dex_mm_update_config", 0, 0, "", "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			if err := client.UpdateBotConfig(ctx, appPass, []byte(in.Config)); err != nil {
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
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				err := dexLocked()
				recordSpend(a, "dex_mm_remove_config", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			if err := client.RemoveBotConfig(ctx, appPass, in.Host, in.BaseID, in.QuoteID); err != nil {
				recordSpend(a, "dex_mm_remove_config", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_mm_remove_config", 0, 0, in.Host, "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_mm_update_cex_config",
		"Store (and validate) CEX API credentials for the market maker. Requires a spend grant with DEX trading enabled and the DEX unlocked.",
		func(ctx context.Context, a *agent, in dexMMCexConfigInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_mm_update_cex_config", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				err := dexLocked()
				recordSpend(a, "dex_mm_update_cex_config", 0, 0, "", "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			if err := client.UpdateCEXConfig(ctx, appPass, []byte(in.Config)); err != nil {
				recordSpend(a, "dex_mm_update_cex_config", 0, 0, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_mm_update_cex_config", 0, 0, "", "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_mm_stop",
		"Stop a running market-maker bot on a market. Requires a spend grant with DEX trading enabled and the DEX unlocked.",
		func(ctx context.Context, a *agent, in dexMMStopInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeDex, time.Now()); err != nil {
				recordSpend(a, "dex_mm_stop", 0, 0, in.Host, "denied", err.Error())
				return nil, err
			}
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				err := dexLocked()
				recordSpend(a, "dex_mm_stop", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexWebClient()
			if err != nil {
				return nil, err
			}
			if err := client.StopBot(ctx, appPass, in.Host, in.BaseID, in.QuoteID); err != nil {
				recordSpend(a, "dex_mm_stop", 0, 0, in.Host, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_mm_stop", 0, 0, in.Host, "ok", "")
			return map[string]bool{"ok": true}, nil
		}),
	agentTool("dex", "dex_send",
		"Send an amount of an asset from a DEX wallet to an address. Requires a spend grant with DEX send/post-bond enabled and the DEX unlocked. For Decred the amount counts against the grant's DCR daily cap; non-DCR assets cannot be DCR-capped.",
		func(ctx context.Context, a *agent, in dexSendInput) (any, error) {
			target := in.Address
			// Only Decred (asset 42) draws on the DCR spend budget; other assets are
			// scope-gated only (the DCR cap cannot bound a non-DCR amount).
			var capAtoms int64
			var amountDCR float64
			if in.AssetID == bisonw.AssetDCR {
				amt, err := dcrutil.NewAmount(in.Value)
				if err != nil || int64(amt) <= 0 {
					return nil, fmt.Errorf("value must be positive")
				}
				capAtoms = int64(amt)
				amountDCR = amt.ToCoin()
			} else if in.Value <= 0 {
				return nil, fmt.Errorf("value must be positive")
			}
			if err := grants.authorizeSpendScoped(a.id, scopeDexSpend, capAtoms, time.Now()); err != nil {
				if tripwire(a.id, err) {
					recordSpend(a, "dex_send", 0, amountDCR, target, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "dex_send", 0, amountDCR, target, "denied", err.Error())
				}
				return nil, err
			}
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				if capAtoms > 0 {
					grants.refund(a.id, capAtoms)
				}
				err := dexLocked()
				recordSpend(a, "dex_send", 0, amountDCR, target, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				if capAtoms > 0 {
					grants.refund(a.id, capAtoms)
				}
				return nil, err
			}
			sendAtoms := dexConvToAtoms(in.Value, in.AssetID)
			coin, err := client.Send(ctx, appPass, in.AssetID, sendAtoms, in.Address)
			if err != nil {
				if capAtoms > 0 {
					grants.refund(a.id, capAtoms)
				}
				recordSpend(a, "dex_send", 0, amountDCR, target, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_send", 0, amountDCR, target, "ok", coin)
			return map[string]string{"coin": coin}, nil
		}),
	agentTool("dex", "dex_post_bond",
		"Post a fidelity bond (in DCR) to register or maintain a DEX account. Requires a spend grant with DEX send/post-bond enabled and the DEX unlocked. The bond counts against the grant's DCR daily cap.",
		func(ctx context.Context, a *agent, in dexPostBondInput) (any, error) {
			if in.Bond == 0 {
				return nil, fmt.Errorf("bond must be positive")
			}
			capAtoms := int64(in.Bond)
			amountDCR := dcrutil.Amount(capAtoms).ToCoin()
			if err := grants.authorizeSpendScoped(a.id, scopeDexSpend, capAtoms, time.Now()); err != nil {
				if tripwire(a.id, err) {
					recordSpend(a, "dex_post_bond", 0, amountDCR, in.Host, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "dex_post_bond", 0, amountDCR, in.Host, "denied", err.Error())
				}
				return nil, err
			}
			appPass, ok := rpc.DcrdexAppPass()
			if !ok {
				grants.refund(a.id, capAtoms)
				err := dexLocked()
				recordSpend(a, "dex_post_bond", 0, amountDCR, in.Host, "error", err.Error())
				return nil, err
			}
			client, err := rpc.DcrdexClient()
			if err != nil {
				grants.refund(a.id, capAtoms)
				return nil, err
			}
			raw, err := client.PostBond(ctx, bisonw.PostBondParams{
				AppPass:      appPass,
				Host:         in.Host,
				Bond:         in.Bond,
				MaintainTier: in.MaintainTier,
			})
			if err != nil {
				grants.refund(a.id, capAtoms)
				recordSpend(a, "dex_post_bond", 0, amountDCR, in.Host, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "dex_post_bond", 0, amountDCR, in.Host, "ok", "")
			return raw, nil
		}),
}
