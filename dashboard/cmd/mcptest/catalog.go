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

	// staking
	rd("staking", "staking_tickets"),
	rd("staking", "staking_info"),
	rd("staking", "staking_vsps"),
	rd("staking", "staking_used_vsps"),
	rd("staking", "staking_autobuyer_settings"),

	// governance
	rd("governance", "governance_agendas"),
	rd("governance", "governance_treasury_policies"),
	rd("governance", "governance_tspend_policies"),
	rd("governance", "governance_proposals"),

	// treasury
	rd("treasury", "treasury_info"),
	rd("treasury", "treasury_mempool_tspends"),

	// lightning
	rd("lightning", "lightning_info"),
	rd("lightning", "lightning_balance"),
	rd("lightning", "lightning_channels"),
	rd("lightning", "lightning_activity"),
	rd("lightning", "lightning_payments"),
	rd("lightning", "lightning_invoices"),

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

	// timestamp
	rd("timestamp", "timestamp_records"),

	// tor
	rd("tor", "tor_status"),
	rd("tor", "tor_control"),
	rd("tor", "tor_settings"),

	// dex
	rd("dex", "dex_version"),
	rd("dex", "dex_exchanges"),
	rd("dex", "dex_wallets"),
	rd("dex", "dex_orders"),
	rd("dex", "dex_notifications"),

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

	// spend / write (gated; placeholder calls must be refused with no grant)
	sp("wallet", "wallet_send", "no spend grant", "", func(*discovered) map[string]any {
		return map[string]any{"account": 0, "address": phAddr, "amountDcr": 0.001}
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
	}
}
