// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"testing"
)

// goldenReadOnlyTools lists every tool registered as read-only. No test can tell
// a mis-registered write tool from a genuine read by inspection, so this list is
// the review checkpoint: adding a read tool has to show up here as a deliberate
// diff, and a write tool registered with readTool by mistake fails the test
// below instead of silently escaping the grant gate.
var goldenReadOnlyTools = []string{
	"br_blocked_contacts",
	"br_connection",
	"br_contact_avatar",
	"br_contact_groups",
	"br_contacts",
	"br_downloads",
	"br_embed_get",
	"br_groupchat",
	"br_groupchat_history",
	"br_groupchats",
	"br_identity",
	"br_notifications",
	"br_page_fetch",
	"br_page_get",
	"br_pages",
	"br_pm_history",
	"br_post",
	"br_post_comments",
	"br_posts",
	"br_rates",
	"br_resolve_nick",
	"br_resolve_uid",
	"br_shared_files",
	"br_shop_cart",
	"br_shop_order",
	"br_shop_orders",
	"br_stats",
	"br_status",
	"br_store",
	"br_store_file_get",
	"br_store_files",
	"br_store_products",
	"capabilities",
	"dex_account",
	"dex_address_used",
	"dex_assets",
	"dex_candles",
	"dex_config",
	"dex_deposit_address",
	"dex_estimate_send_fee",
	"dex_exchanges",
	"dex_market_summary",
	"dex_max_buy",
	"dex_max_sell",
	"dex_mm_archived_runs",
	"dex_mm_market_report",
	"dex_mm_run_logs",
	"dex_mm_status",
	"dex_notifications",
	"dex_order",
	"dex_orderbook",
	"dex_orders",
	"dex_orders_history",
	"dex_preorder",
	"dex_rates",
	"dex_trades",
	"dex_version",
	"dex_wallet",
	"dex_wallet_tx",
	"dex_wallet_txs",
	"dex_wallets",
	"explorer_address",
	"explorer_block_by_hash",
	"explorer_block_by_height",
	"explorer_mempool",
	"explorer_recent_blocks",
	"explorer_search",
	"explorer_transaction",
	"governance_agendas",
	"governance_proposal_detail",
	"governance_proposal_vote_eligibility",
	"governance_proposals",
	"governance_refresh_proposal_detail",
	"governance_refresh_proposals",
	"governance_treasury_policies",
	"governance_tspend_policies",
	"governance_vote_trickle_events",
	"governance_vote_trickle_status",
	"lightning_activity",
	"lightning_balance",
	"lightning_channels",
	"lightning_info",
	"lightning_invoices",
	"lightning_payments",
	"ln_autopilot_status",
	"ln_decode_invoice",
	"ln_graph_node",
	"ln_graph_routes",
	"ln_graph_search",
	"ln_liquidity_defaults",
	"ln_liquidity_estimate",
	"ln_network",
	"ln_peer_presets",
	"ln_watchtowers",
	"node_blockchain_info",
	"node_dashboard",
	"node_mempool",
	"node_network",
	"node_peers",
	"node_staking_overview",
	"node_status",
	"node_supply",
	"privacy_status",
	"staking_autobuyer_settings",
	"staking_autobuyer_status",
	"staking_info",
	"staking_purchase_status",
	"staking_tickets",
	"staking_used_vsps",
	"staking_vsp_info",
	"staking_vsps",
	"timestamp_export",
	"timestamp_proof",
	"timestamp_records",
	"timestamp_status",
	"timestamp_validate",
	"timestamp_verify",
	"tor_control",
	"tor_settings",
	"tor_status",
	"treasury_balance_history",
	"treasury_info",
	"treasury_scan_progress",
	"treasury_scan_results",
	"treasury_scan_start",
	"wallet_accounts",
	"wallet_addresses",
	"wallet_construct_transaction",
	"wallet_dashboard",
	"wallet_decode_signed_transaction",
	"wallet_new_address",
	"wallet_status",
	"wallet_sync_progress",
	"wallet_transactions",
	"wallet_validate_address",
}

// TestReadOnlyClassificationIsDeclared pins the read/write split to the catalog
// rather than to the annotation that the same registration call sets.
func TestReadOnlyClassificationIsDeclared(t *testing.T) {
	golden := make(map[string]bool, len(goldenReadOnlyTools))
	for _, n := range goldenReadOnlyTools {
		golden[n] = true
	}
	seen := map[string]bool{}
	for _, td := range toolCatalog {
		if td.name == "" {
			t.Errorf("a tool in domain %q has no name, so the catalog cannot be checked by name", td.domain)
			continue
		}
		seen[td.name] = true
		switch {
		case td.readOnly && !golden[td.name]:
			t.Errorf("%s is registered read-only but is not in goldenReadOnlyTools. If it only reads, add it. If it changes anything, register it with agentTool and gate it on a scope.", td.name)
		case !td.readOnly && golden[td.name]:
			t.Errorf("%s is a write tool but is listed in goldenReadOnlyTools; remove it from the list.", td.name)
		}
	}
	for _, n := range goldenReadOnlyTools {
		if !seen[n] {
			t.Errorf("goldenReadOnlyTools lists %q, which is no longer in the catalog; remove it.", n)
		}
	}
}

// TestWireAnnotationsMatchCatalog makes the advertised annotation and the
// in-process classification the same fact, so they cannot drift apart and leave
// the gating test reading a stale hint.
func TestWireAnnotationsMatchCatalog(t *testing.T) {
	want := map[string]bool{}
	for _, td := range toolCatalog {
		want[td.name] = td.readOnly
	}
	domains := map[string]bool{}
	for _, d := range catalogDomains() {
		domains[d] = true
	}
	cs := connectTo(t, testAgent("anno-match", "anno", domains))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tl := range res.Tools {
		onWire := tl.Annotations != nil && tl.Annotations.ReadOnlyHint
		if onWire != want[tl.Name] {
			t.Errorf("%s: wire read-only hint is %v but the catalog says %v", tl.Name, onWire, want[tl.Name])
		}
	}
}
