// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import "strings"

// discovered holds live identifiers harvested from earlier read calls so that
// parameter-taking read tools can be exercised with real values instead of
// being skipped. Fields are filled by absorb as producer tools succeed.
type discovered struct {
	address     string
	txid        string
	blockHash   string
	blockHeight int64
	uid         string
	gcid        string
	postUID     string
	postID      string
	vspHost     string
	vspPubkey   string
	propToken   string // Politeia proposal token (governance_proposals)
	dexHost     string // a configured DEX server host (dex_exchanges)
	tsDigest    string // a sha256 hex digest of a timestamp record (timestamp_records)
}

type toolKind int

const (
	kindRead toolKind = iota
	kindSpend
)

// spec describes one tool and how the harness exercises it.
type spec struct {
	domain string
	name   string
	kind   toolKind

	// read builds arguments for a read tool. ok=false means a required live
	// identifier is missing, so the tool is reported SKIPPED rather than called.
	read func(*discovered) (args map[string]any, ok bool)

	// spend builds placeholder arguments for a spend tool (always called in the
	// gating phase), and denial is the substring the result MUST contain when no
	// grant is present.
	spend  func(*discovered) map[string]any
	denial string
	note   string
}

// placeholder values for spend gating: they only need to satisfy each tool's
// input schema and reach the grant check, which refuses before anything moves.
const (
	phAddr = "DsTestPlaceholderAddressXXXXXXXXXXX"
	phHex  = "0000000000000000000000000000000000000000000000000000000000000000"
)

// optInvoice, when set via -invoice, lets ln_pay reach its grant check instead
// of failing at invoice decode.
var optInvoice string

func noArgs(*discovered) (map[string]any, bool) { return map[string]any{}, true }

func rd(domain, name string) spec {
	return spec{domain: domain, name: name, kind: kindRead, read: noArgs}
}

func rdArgs(domain, name string, f func(*discovered) (map[string]any, bool)) spec {
	return spec{domain: domain, name: name, kind: kindRead, read: f}
}

func sp(domain, name, denial, note string, f func(*discovered) map[string]any) spec {
	return spec{domain: domain, name: name, kind: kindSpend, spend: f, denial: denial, note: note}
}

// catalog is the full ordered list of tools the harness exercises. Read-tool
// producers are ordered before their consumers so identifier chaining works.
var catalog = []spec{
	// node
	rd("node", "node_status"),
	rd("node", "node_dashboard"),
	rd("node", "node_blockchain_info"),
	rd("node", "node_network"),
	rd("node", "node_peers"),
	rd("node", "node_supply"),
	rd("node", "node_staking_overview"),
	rd("node", "node_mempool"),

	// wallet
	rd("wallet", "wallet_dashboard"),
	rd("wallet", "wallet_accounts"),
	rd("wallet", "wallet_status"),
	rd("wallet", "wallet_addresses"),
	rdArgs("wallet", "wallet_new_address", func(*discovered) (map[string]any, bool) {
		return map[string]any{"account": 0}, true
	}),
	rdArgs("wallet", "wallet_transactions", func(*discovered) (map[string]any, bool) {
		return map[string]any{"count": 5}, true
	}),
	rd("wallet", "wallet_sync_progress"),
	rdArgs("wallet", "wallet_construct_transaction", func(d *discovered) (map[string]any, bool) {
		if d.address == "" {
			return nil, false
		}
		return map[string]any{"account": 0, "address": d.address, "amountAtoms": 100000}, true
	}),
	rdArgs("wallet", "wallet_decode_signed_transaction", func(*discovered) (map[string]any, bool) { return nil, false }),
	rdArgs("wallet", "wallet_validate_address", func(d *discovered) (map[string]any, bool) {
		if d.address == "" {
			return nil, false
		}
		return map[string]any{"address": d.address}, true
	}),

	// staking
	rd("staking", "staking_tickets"),
	rd("staking", "staking_info"),
	rd("staking", "staking_vsps"),
	rd("staking", "staking_used_vsps"),
	rd("staking", "staking_autobuyer_settings"),
	rd("staking", "staking_purchase_status"),
	rd("staking", "staking_autobuyer_status"),
	rdArgs("staking", "staking_vsp_info", func(d *discovered) (map[string]any, bool) {
		if d.vspHost == "" {
			return nil, false
		}
		return map[string]any{"host": d.vspHost}, true
	}),

	// governance
	rd("governance", "governance_agendas"),
	rd("governance", "governance_treasury_policies"),
	rd("governance", "governance_tspend_policies"),
	rd("governance", "governance_proposals"),
	rd("governance", "governance_refresh_proposals"),
	rd("governance", "governance_vote_trickle_status"),
	rdArgs("governance", "governance_vote_trickle_events", func(*discovered) (map[string]any, bool) {
		return map[string]any{"count": 20}, true
	}),
	// governance consumers (need a proposal token from governance_proposals)
	rdArgs("governance", "governance_proposal_detail", func(d *discovered) (map[string]any, bool) {
		if d.propToken == "" {
			return nil, false
		}
		return map[string]any{"token": d.propToken}, true
	}),
	rdArgs("governance", "governance_proposal_vote_eligibility", func(d *discovered) (map[string]any, bool) {
		if d.propToken == "" {
			return nil, false
		}
		return map[string]any{"token": d.propToken}, true
	}),
	rdArgs("governance", "governance_refresh_proposal_detail", func(d *discovered) (map[string]any, bool) {
		if d.propToken == "" {
			return nil, false
		}
		return map[string]any{"token": d.propToken}, true
	}),

	// treasury
	rd("treasury", "treasury_info"),
	rd("treasury", "treasury_mempool_tspends"),
	rd("treasury", "treasury_balance_history"),
	rd("treasury", "treasury_scan_progress"),
	rd("treasury", "treasury_scan_results"),
	rdArgs("treasury", "treasury_vote_progress", func(d *discovered) (map[string]any, bool) {
		if d.txid == "" {
			return nil, false
		}
		return map[string]any{"txHash": d.txid}, true
	}),

	// lightning
	rd("lightning", "lightning_info"),
	rd("lightning", "lightning_balance"),
	rd("lightning", "lightning_channels"),
	rd("lightning", "lightning_activity"),
	rd("lightning", "lightning_payments"),
	rd("lightning", "lightning_invoices"),
	rd("lightning", "ln_peer_presets"),
	rd("lightning", "ln_liquidity_defaults"),
	rd("lightning", "ln_autopilot_status"),
	rd("lightning", "ln_network"),
	rd("lightning", "ln_watchtowers"),
	rdArgs("lightning", "ln_graph_search", func(*discovered) (map[string]any, bool) {
		return map[string]any{}, true
	}),
	rdArgs("lightning", "ln_liquidity_estimate", func(*discovered) (map[string]any, bool) {
		return map[string]any{"chanSizeDcr": 0.01}, true
	}),
	rdArgs("lightning", "ln_decode_invoice", func(*discovered) (map[string]any, bool) {
		if optInvoice == "" {
			return nil, false
		}
		return map[string]any{"payReq": optInvoice}, true
	}),
	rdArgs("lightning", "ln_graph_node", func(*discovered) (map[string]any, bool) {
		return nil, false
	}),
	rdArgs("lightning", "ln_graph_routes", func(*discovered) (map[string]any, bool) {
		return nil, false
	}),

	// privacy
	rd("privacy", "privacy_status"),

	// explorer (consumers of node/wallet identifiers)
	rdArgs("explorer", "explorer_address", func(d *discovered) (map[string]any, bool) {
		if d.address == "" {
			return nil, false
		}
		return map[string]any{"address": d.address}, true
	}),
	rdArgs("explorer", "explorer_transaction", func(d *discovered) (map[string]any, bool) {
		if d.txid == "" {
			return nil, false
		}
		return map[string]any{"txHash": d.txid}, true
	}),
	rdArgs("explorer", "explorer_block_by_height", func(d *discovered) (map[string]any, bool) {
		h := d.blockHeight
		if h <= 0 {
			h = 1
		}
		return map[string]any{"height": h}, true
	}),
	rdArgs("explorer", "explorer_block_by_hash", func(d *discovered) (map[string]any, bool) {
		if d.blockHash == "" {
			return nil, false
		}
		return map[string]any{"hash": d.blockHash}, true
	}),
	rdArgs("explorer", "explorer_recent_blocks", func(*discovered) (map[string]any, bool) {
		return map[string]any{"page": 1, "pageSize": 5}, true
	}),
	rd("explorer", "explorer_mempool"),
	rdArgs("explorer", "explorer_search", func(d *discovered) (map[string]any, bool) {
		if d.address == "" {
			return nil, false
		}
		return map[string]any{"query": d.address}, true
	}),

	// timestamp
	rd("timestamp", "timestamp_records"),
	rd("timestamp", "timestamp_status"),
	rd("timestamp", "timestamp_export"),
	// timestamp consumers (need a digest from timestamp_records)
	rdArgs("timestamp", "timestamp_verify", func(d *discovered) (map[string]any, bool) {
		if d.tsDigest == "" {
			return nil, false
		}
		return map[string]any{"digest": d.tsDigest}, true
	}),
	rdArgs("timestamp", "timestamp_validate", func(d *discovered) (map[string]any, bool) {
		if d.tsDigest == "" {
			return nil, false
		}
		return map[string]any{"digest": d.tsDigest}, true
	}),
	rdArgs("timestamp", "timestamp_proof", func(d *discovered) (map[string]any, bool) {
		if d.tsDigest == "" {
			return nil, false
		}
		return map[string]any{"digest": d.tsDigest}, true
	}),

	// tor
	rd("tor", "tor_status"),
	rd("tor", "tor_control"),
	rd("tor", "tor_settings"),

	// dex (dex_exchanges produces the host that host-consumers reuse)
	rd("dex", "dex_version"),
	rd("dex", "dex_exchanges"),
	rd("dex", "dex_wallets"),
	rd("dex", "dex_orders"),
	rd("dex", "dex_notifications"),
	rd("dex", "dex_assets"),
	rd("dex", "dex_rates"),
	rd("dex", "dex_orders_history"),
	rd("dex", "dex_mm_status"),
	rd("dex", "dex_mm_archived_runs"),
	rdArgs("dex", "dex_wallet", func(*discovered) (map[string]any, bool) {
		return map[string]any{"assetId": 42}, true
	}),
	rdArgs("dex", "dex_wallet_txs", func(*discovered) (map[string]any, bool) {
		return map[string]any{"assetId": 42}, true
	}),
	rdArgs("dex", "dex_deposit_address", func(*discovered) (map[string]any, bool) {
		return map[string]any{"assetId": 42}, true
	}),
	rdArgs("dex", "dex_config", func(d *discovered) (map[string]any, bool) {
		if d.dexHost == "" {
			return nil, false
		}
		return map[string]any{"host": d.dexHost}, true
	}),
	rdArgs("dex", "dex_account", func(d *discovered) (map[string]any, bool) {
		if d.dexHost == "" {
			return nil, false
		}
		return map[string]any{"host": d.dexHost}, true
	}),
	rdArgs("dex", "dex_preorder", func(d *discovered) (map[string]any, bool) {
		if d.dexHost == "" {
			return nil, false
		}
		return map[string]any{"host": d.dexHost, "base": 42, "quote": 0, "sell": false, "isLimit": true, "qty": 1, "rate": 1}, true
	}),
	rdArgs("dex", "dex_max_buy", func(d *discovered) (map[string]any, bool) {
		if d.dexHost == "" {
			return nil, false
		}
		return map[string]any{"host": d.dexHost, "base": 42, "quote": 0, "rate": 1}, true
	}),
	rdArgs("dex", "dex_max_sell", func(d *discovered) (map[string]any, bool) {
		if d.dexHost == "" {
			return nil, false
		}
		return map[string]any{"host": d.dexHost, "base": 42, "quote": 0}, true
	}),
	rdArgs("dex", "dex_mm_market_report", func(d *discovered) (map[string]any, bool) {
		if d.dexHost == "" {
			return nil, false
		}
		return map[string]any{"host": d.dexHost, "baseId": 42, "quoteId": 0}, true
	}),
	rdArgs("dex", "dex_orderbook", func(d *discovered) (map[string]any, bool) {
		if d.dexHost == "" {
			return nil, false
		}
		return map[string]any{"host": d.dexHost, "baseId": 42, "quoteId": 0}, true
	}),
	rdArgs("dex", "dex_trades", func(d *discovered) (map[string]any, bool) {
		if d.dexHost == "" {
			return nil, false
		}
		return map[string]any{"host": d.dexHost, "baseId": 42, "quoteId": 0}, true
	}),
	rdArgs("dex", "dex_candles", func(d *discovered) (map[string]any, bool) {
		if d.dexHost == "" {
			return nil, false
		}
		return map[string]any{"host": d.dexHost, "baseId": 42, "quoteId": 0, "dur": "24h"}, true
	}),
	rd("dex", "dex_market_summary"),
	rdArgs("dex", "dex_mm_run_logs", func(d *discovered) (map[string]any, bool) {
		if d.dexHost == "" {
			return nil, false
		}
		return map[string]any{"host": d.dexHost, "baseId": 42, "quoteId": 0, "startTime": 0}, true
	}),
	rdArgs("dex", "dex_order", func(*discovered) (map[string]any, bool) {
		return nil, false
	}),
	rdArgs("dex", "dex_wallet_tx", func(*discovered) (map[string]any, bool) {
		return nil, false
	}),
	rdArgs("dex", "dex_address_used", func(*discovered) (map[string]any, bool) {
		return nil, false
	}),
	rdArgs("dex", "dex_estimate_send_fee", func(*discovered) (map[string]any, bool) {
		return nil, false
	}),

	// bisonrelay read producers
	rd("bisonrelay", "br_status"),
	rd("bisonrelay", "br_identity"),
	rd("bisonrelay", "br_connection"),
	rd("bisonrelay", "br_contacts"),
	rd("bisonrelay", "br_blocked_contacts"),
	rd("bisonrelay", "br_contact_groups"),
	rd("bisonrelay", "br_notifications"),
	rd("bisonrelay", "br_posts"),
	rd("bisonrelay", "br_groupchats"),
	rd("bisonrelay", "br_shared_files"),
	rd("bisonrelay", "br_stats"),
	rd("bisonrelay", "br_store"),
	rd("bisonrelay", "br_store_products"),
	rd("bisonrelay", "br_pages"),
	rd("bisonrelay", "br_downloads"),
	rd("bisonrelay", "br_store_files"),
	rdArgs("bisonrelay", "br_store_file_get", func(*discovered) (map[string]any, bool) { return nil, false }),
	rd("bisonrelay", "br_rates"),
	// bisonrelay read consumers (need a contact/post/groupchat identifier)
	rdArgs("bisonrelay", "br_pm_history", func(d *discovered) (map[string]any, bool) {
		if d.uid == "" {
			return nil, false
		}
		return map[string]any{"uid": d.uid}, true
	}),
	rdArgs("bisonrelay", "br_post", func(d *discovered) (map[string]any, bool) {
		if d.postUID == "" || d.postID == "" {
			return nil, false
		}
		return map[string]any{"uid": d.postUID, "pid": d.postID}, true
	}),
	rdArgs("bisonrelay", "br_post_comments", func(d *discovered) (map[string]any, bool) {
		if d.postUID == "" || d.postID == "" {
			return nil, false
		}
		return map[string]any{"uid": d.postUID, "pid": d.postID}, true
	}),
	rdArgs("bisonrelay", "br_groupchat", func(d *discovered) (map[string]any, bool) {
		if d.gcid == "" {
			return nil, false
		}
		return map[string]any{"gcid": d.gcid}, true
	}),
	rdArgs("bisonrelay", "br_groupchat_history", func(d *discovered) (map[string]any, bool) {
		if d.gcid == "" {
			return nil, false
		}
		return map[string]any{"gcid": d.gcid}, true
	}),
	// Remote page/storefront reads: schema-listed but not invoked live -- a fetch
	// against an arbitrary contact who hosts nothing would 404 or block for the
	// 30s page-fetch timeout. Exercise these against a known store by hand.
	rdArgs("bisonrelay", "br_page_fetch", func(*discovered) (map[string]any, bool) { return nil, false }),
	rdArgs("bisonrelay", "br_shop_cart", func(*discovered) (map[string]any, bool) { return nil, false }),
	rdArgs("bisonrelay", "br_shop_orders", func(*discovered) (map[string]any, bool) { return nil, false }),
	rdArgs("bisonrelay", "br_shop_order", func(*discovered) (map[string]any, bool) { return nil, false }),

	// spend / write (gated; placeholder calls must be refused with no grant)
	sp("wallet", "wallet_send", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"account": 0, "address": phAddr, "amountDcr": 0.001}
	}),
	sp("wallet", "wallet_broadcast_signed_transaction", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"signedTxHex": phHex}
	}),
	sp("staking", "staking_purchase", "no spend grant", "", func(d *discovered) map[string]any {
		host, key := d.vspHost, d.vspPubkey
		if host == "" {
			host = "https://vsp.example.org"
		}
		if key == "" {
			key = phHex
		}
		return map[string]any{"account": 0, "numTickets": 1, "vspHost": host, "vspPubkey": key}
	}),
	sp("governance", "governance_set_vote_choice", "does not allow governance voting", "", func(*discovered) map[string]any {
		return map[string]any{"agendaId": "mcptest", "choiceId": "yes"}
	}),
	sp("governance", "governance_cast_proposal_vote", "does not allow governance voting", "", func(*discovered) map[string]any {
		return map[string]any{"token": "mcptest", "voteOption": "yes"}
	}),
	sp("governance", "governance_set_treasury_policy", "does not allow governance voting", "", func(*discovered) map[string]any {
		return map[string]any{"key": phHex, "policy": "abstain"}
	}),
	sp("governance", "governance_set_tspend_policy", "does not allow governance voting", "", func(*discovered) map[string]any {
		return map[string]any{"hash": phHex, "policy": "abstain"}
	}),
	sp("governance", "governance_vote_trickle_start", "does not allow governance voting", "", func(*discovered) map[string]any {
		return map[string]any{"token": "mcptest", "voteOption": "yes", "durationSeconds": 60}
	}),
	sp("governance", "governance_vote_trickle_stop", "does not allow governance voting", "", func(*discovered) map[string]any {
		return map[string]any{"token": "mcptest"}
	}),
	sp("lightning", "ln_pay", "does not allow Lightning payments",
		"decode runs before the grant check; without -invoice this fails at decode (still a safe no-op)",
		func(*discovered) map[string]any {
			req := optInvoice
			if req == "" {
				req = "lnbc1mcptestplaceholderinvoice"
			}
			return map[string]any{"payReq": req}
		}),
	sp("lightning", "ln_add_invoice", "does not allow Lightning payments", "", func(*discovered) map[string]any {
		return map[string]any{"amountDcr": 0.0001, "memo": "mcptest"}
	}),
	sp("lightning", "ln_open_channel", "does not allow Lightning payments", "", func(*discovered) map[string]any {
		return map[string]any{"peerUri": "02" + phHex[:64] + "@127.0.0.1:9735", "localDcr": 0.01}
	}),
	sp("dex", "dex_place_order", "does not allow DEX trading", "", func(*discovered) map[string]any {
		return map[string]any{"host": "dex.decred.org:7232", "base": 42, "quote": 0, "sell": false, "isLimit": true, "qty": 1, "rate": 1}
	}),
	sp("dex", "dex_cancel_order", "does not allow DEX trading", "", func(*discovered) map[string]any {
		return map[string]any{"orderId": phHex}
	}),
	sp("bisonrelay", "br_store_save_product", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"sku": "mcptest", "title": "mcptest", "price": 0.01}
	}),
	sp("bisonrelay", "br_store_delete_product", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"sku": "mcptest"}
	}),
	sp("bisonrelay", "br_send_message", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex, "message": "mcptest"}
	}),
	sp("bisonrelay", "br_send_groupchat_message", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"gcid": phHex, "message": "mcptest"}
	}),
	sp("bisonrelay", "br_tip_user", "does not allow Lightning payments", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex, "amountDcr": 0.0001}
	}),
	sp("bisonrelay", "br_unshare_file", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"fid": phHex}
	}),
	sp("bisonrelay", "br_page_submit", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex, "path": []string{"addToCart"}, "data": map[string]any{"sku": "x", "qty": 1}}
	}),
	sp("bisonrelay", "br_shop_add_to_cart", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex, "sku": "mcptest", "quantity": 1}
	}),
	sp("bisonrelay", "br_shop_clear_cart", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex}
	}),
	sp("bisonrelay", "br_shop_place_order", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex}
	}),
	sp("bisonrelay", "br_shop_order_comment", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex, "id": 1, "comment": "mcptest"}
	}),
	sp("bisonrelay", "br_content_get", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex, "fid": phHex}
	}),
	sp("bisonrelay", "br_download_cancel", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"fid": phHex}
	}),
	sp("bisonrelay", "br_download_delete", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"fid": phHex}
	}),
	sp("bisonrelay", "br_notification_delete", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"id": 1}
	}),
	sp("bisonrelay", "br_notifications_clear", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{}
	}),
	sp("bisonrelay", "br_store_file_delete", "does not allow Bison Relay write actions", "", func(*discovered) map[string]any {
		return map[string]any{"path": "mcptest-placeholder.txt"}
	}),

	// staking writes (gated)
	sp("staking", "staking_autobuyer_save_settings", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"account": 0, "vspHost": "https://vsp.example.org", "vspPubkey": phHex, "balanceToMaintain": 0.001}
	}),
	sp("staking", "staking_autobuyer_stop", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{}
	}),
	sp("staking", "staking_sync_failed_vsp_tickets", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"account": 0, "vspHost": "https://vsp.example.org", "vspPubkey": phHex}
	}),
	sp("staking", "staking_process_unmanaged_vsp_tickets", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"account": 0, "vspHost": "https://vsp.example.org", "vspPubkey": phHex}
	}),

	// privacy writes (gated)
	sp("privacy", "privacy_mixer_start", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{}
	}),
	sp("privacy", "privacy_mixer_stop", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{}
	}),

	// tor writes (gated)
	sp("tor", "tor_set_settings", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"enabled": false, "isolation": false, "dcrdOnion": false, "circuitLimit": 8}
	}),
	sp("tor", "tor_new_identity", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{}
	}),

	// timestamp writes (gated; grant is checked before any digest validation)
	sp("timestamp", "timestamp_create", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"digest": phHex, "filename": "mcptest"}
	}),
	sp("timestamp", "timestamp_retry", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"digest": phHex}
	}),
	sp("timestamp", "timestamp_delete", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"digest": phHex}
	}),
	sp("timestamp", "timestamp_update", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"digest": phHex, "title": "mcptest"}
	}),
	sp("timestamp", "timestamp_refresh", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{}
	}),

	// lightning writes (gated; some validate args before the grant check)
	sp("lightning", "ln_close_channel", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"channelPoint": phHex + ":0"}
	}),
	sp("lightning", "ln_cancel_invoice", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"paymentHash": phHex}
	}),
	sp("lightning", "ln_watchtower_add", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"pubKey": phHex, "address": "127.0.0.1:9911"}
	}),
	sp("lightning", "ln_watchtower_remove", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"pubKey": phHex}
	}),
	sp("lightning", "ln_autopilot_set", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"active": false}
	}),
	sp("lightning", "ln_liquidity_request", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"chanSizeDcr": 0.01, "approvedFeeDcr": 0.001}
	}),

	// dex writes (gated; dex_send/dex_post_bond validate amount before the grant check)
	sp("dex", "dex_set_bond_options", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"host": "dex.decred.org:7232"}
	}),
	sp("dex", "dex_wallet_open", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"assetId": 42}
	}),
	sp("dex", "dex_wallet_close", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"assetId": 42}
	}),
	sp("dex", "dex_wallet_toggle", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"assetId": 42, "disable": false}
	}),
	sp("dex", "dex_wallet_rescan", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"assetId": 42}
	}),
	sp("dex", "dex_add_peer", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"assetId": 42, "address": "127.0.0.1:9108"}
	}),
	sp("dex", "dex_remove_peer", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"assetId": 42, "address": "127.0.0.1:9108"}
	}),
	sp("dex", "dex_discover_account", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"host": "dex.decred.org:7232"}
	}),
	sp("dex", "dex_mm_update_config", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"config": "{}"}
	}),
	sp("dex", "dex_mm_remove_config", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"host": "dex.decred.org:7232", "baseId": 42, "quoteId": 0}
	}),
	sp("dex", "dex_mm_update_cex_config", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"config": "{}"}
	}),
	sp("dex", "dex_mm_stop", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"host": "dex.decred.org:7232", "baseId": 42, "quoteId": 0}
	}),
	sp("dex", "dex_send", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"assetId": 42, "value": 0.001, "address": phAddr}
	}),
	sp("dex", "dex_post_bond", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"host": "dex.decred.org:7232", "bond": 1}
	}),

	// bisonrelay writes (scopeBR)
	sp("bisonrelay", "br_post_create", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"post": "mcptest"}
	}),
	sp("bisonrelay", "br_post_comment", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex, "pid": phHex, "comment": "mcptest"}
	}),
	sp("bisonrelay", "br_post_heart", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex, "pid": phHex, "heart": true}
	}),
	sp("bisonrelay", "br_post_relay", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex, "pid": phHex}
	}),
	sp("bisonrelay", "br_page_save", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"name": "mcptest", "content": "mcptest"}
	}),
	sp("bisonrelay", "br_page_delete", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"name": "mcptest"}
	}),
	sp("bisonrelay", "br_file_send", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex, "filename": "mcptest", "dataB64": "eA=="}
	}),
	sp("bisonrelay", "br_file_add", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"filename": "mcptest", "dataB64": "eA=="}
	}),
	sp("bisonrelay", "br_store_order_status", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex, "id": 1, "status": "shipped"}
	}),
	sp("bisonrelay", "br_store_order_comment", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"uid": phHex, "id": 1, "comment": "mcptest"}
	}),
	sp("bisonrelay", "br_store_file_upload", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"filename": "mcptest", "dataB64": "eA=="}
	}),
	sp("bisonrelay", "br_store_template_save", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"name": "mcptest", "content": "mcptest"}
	}),
	sp("bisonrelay", "br_store_template_delete", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"name": "mcptest"}
	}),
	sp("bisonrelay", "br_gc_create", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"name": "mcptest"}
	}),
	sp("bisonrelay", "br_gc_invite", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"gcid": phHex, "uid": phHex}
	}),
	sp("bisonrelay", "br_gc_invites_accept", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"iid": 1}
	}),
	sp("bisonrelay", "br_gc_part", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"gcid": phHex}
	}),
	sp("bisonrelay", "br_rtdt_create", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"description": "mcptest"}
	}),
	sp("bisonrelay", "br_rtdt_create_instant", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"uids": []string{phHex}}
	}),
	sp("bisonrelay", "br_rtdt_invite", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"rv": phHex, "uids": []string{phHex}}
	}),
	sp("bisonrelay", "br_rtdt_accept", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"rv": phHex, "inviter": phHex}
	}),
	sp("bisonrelay", "br_rtdt_join", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"rv": phHex}
	}),
	sp("bisonrelay", "br_rtdt_leave", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"rv": phHex}
	}),
	sp("bisonrelay", "br_rtdt_dissolve", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"rv": phHex}
	}),
	sp("bisonrelay", "br_rtdt_chat", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"rv": phHex, "message": "mcptest"}
	}),

	// bisonrelay group-admin writes (scopeBRAdmin)
	sp("bisonrelay", "br_gc_kick", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"gcid": phHex, "uid": phHex}
	}),
	sp("bisonrelay", "br_gc_kill", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"gcid": phHex}
	}),
	sp("bisonrelay", "br_gc_block", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"gcid": phHex, "uid": phHex}
	}),
	sp("bisonrelay", "br_gc_unblock", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"gcid": phHex, "uid": phHex}
	}),
	sp("bisonrelay", "br_gc_admins", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"gcid": phHex, "extraAdmins": []string{phHex}}
	}),
	sp("bisonrelay", "br_gc_owner", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"gcid": phHex, "newOwner": phHex}
	}),
	sp("bisonrelay", "br_rtdt_kick", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"rv": phHex, "peerId": 1}
	}),
	sp("bisonrelay", "br_rtdt_remove", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"rv": phHex, "uid": phHex}
	}),
}

// absorb harvests live identifiers from a successful read result into d, scoped
// by which tool produced it so that 64-hex values are not cross-assigned.
func absorb(d *discovered, name string, data any) {
	switch name {
	case "node_status", "node_blockchain_info":
		if d.blockHash == "" {
			d.blockHash = findString(data, isHex64, "besthash", "bestblockhash", "blockhash", "hash")
		}
		if d.blockHeight == 0 {
			if h, ok := findInt(data, "height", "blocks", "blockheight", "bestheight", "headers"); ok {
				d.blockHeight = h
			}
		}
	case "wallet_addresses", "wallet_new_address":
		if d.address == "" {
			if a := findString(data, isAddr, "address", "addr"); a != "" {
				d.address = a
			} else {
				d.address = findString(data, isAddr)
			}
		}
	case "wallet_transactions":
		if d.txid == "" {
			d.txid = findString(data, isHex64, "txid", "txhash", "hash", "tx")
		}
	case "staking_vsps":
		if d.vspHost == "" {
			d.vspHost = findString(data, func(s string) bool { return strings.HasPrefix(s, "http") }, "url", "host", "vspurl", "vsp")
		}
	case "br_contacts", "br_identity":
		if d.uid == "" {
			d.uid = findString(data, isHex64, "uid", "id", "identity", "pubkey")
		}
	case "br_groupchats":
		if d.gcid == "" {
			d.gcid = findString(data, isHex64, "id", "gcid")
		}
	case "br_posts":
		if d.postUID == "" {
			d.postUID = findString(data, isHex64, "from", "author", "authorid", "uid")
		}
		if d.postID == "" {
			d.postID = findString(data, isHex64, "id", "pid", "postid", "hash")
		}
	case "governance_proposals":
		if d.propToken == "" {
			d.propToken = findString(data, isHex64, "token", "censorshiprecord", "censorshipRecord")
		}
	case "dex_exchanges":
		if d.dexHost == "" {
			d.dexHost = findString(data, isHostPort, "host", "url")
		}
	case "timestamp_records":
		if d.tsDigest == "" {
			d.tsDigest = findString(data, isHex64, "digest")
		}
	}
}

// isHostPort matches a "host:port" DEX server address (a dotted host followed by
// a numeric port), used to harvest a live DEX host from dex_exchanges.
func isHostPort(s string) bool {
	i := strings.LastIndexByte(s, ':')
	if i <= 0 || i == len(s)-1 || !strings.Contains(s[:i], ".") {
		return false
	}
	for _, r := range s[i+1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
