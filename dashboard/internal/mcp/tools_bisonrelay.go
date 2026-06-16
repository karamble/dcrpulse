// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/rpc"
)

type brSaveProductInput struct {
	SKU          string   `json:"sku" jsonschema:"unique product SKU"`
	Title        string   `json:"title" jsonschema:"product title"`
	Description  string   `json:"description,omitempty" jsonschema:"product description"`
	Price        float64  `json:"price" jsonschema:"price in DCR"`
	Tags         []string `json:"tags,omitempty" jsonschema:"optional tags"`
	Shipping     bool     `json:"shipping,omitempty" jsonschema:"true if the product must be shipped"`
	Disabled     bool     `json:"disabled,omitempty" jsonschema:"true to hide the product from the store"`
	SendFilename string   `json:"sendfilename,omitempty" jsonschema:"relative path of a file already uploaded to the store (via the dashboard), delivered to the buyer on purchase"`
}

type brDeleteProductInput struct {
	SKU string `json:"sku" jsonschema:"SKU of the product to delete"`
}

type brSendMessageInput struct {
	UID     string `json:"uid" jsonschema:"contact identity (nick, alias, or hex UID)"`
	Message string `json:"message" jsonschema:"message text to send"`
}

type brSendGCMessageInput struct {
	GCID    string `json:"gcid" jsonschema:"group chat id, hex"`
	Message string `json:"message" jsonschema:"message text to send"`
}

type brTipInput struct {
	UID         string  `json:"uid" jsonschema:"contact hex UID to tip"`
	AmountDCR   float64 `json:"amountDcr" jsonschema:"tip amount in DCR (paid over Lightning)"`
	MaxAttempts int32   `json:"maxAttempts,omitempty" jsonschema:"max payment attempts; defaults to 1"`
}

type brUnshareInput struct {
	FID       string `json:"fid" jsonschema:"shared file id (hex) to revoke"`
	TargetUID string `json:"targetUid,omitempty" jsonschema:"empty revokes the global share; a hex UID revokes a per-user share"`
}

type brNotificationsInput struct {
	Count int `json:"count,omitempty" jsonschema:"max notifications to return (default 50)"`
}

type brPostInput struct {
	UID string `json:"uid" jsonschema:"author identity, hex"`
	PID string `json:"pid" jsonschema:"post id, hex"`
}

type brPmHistoryInput struct {
	UID      string `json:"uid" jsonschema:"peer identity, hex"`
	Page     int    `json:"page,omitempty" jsonschema:"page number, 0-based"`
	PageSize int    `json:"pageSize,omitempty" jsonschema:"messages per page (default 50)"`
}

type brGCInput struct {
	GCID string `json:"gcid" jsonschema:"group chat id, hex"`
}

type brGCHistoryInput struct {
	GCID     string `json:"gcid" jsonschema:"group chat id, hex"`
	Page     int    `json:"page,omitempty" jsonschema:"page number, 0-based"`
	PageSize int    `json:"pageSize,omitempty" jsonschema:"messages per page (default 50)"`
}

func brPageSize(n int) int {
	if n <= 0 {
		return 50
	}
	return n
}

// bisonrelayTools are the read-only "bisonrelay" domain tools. They call the
// same brclientd status-server endpoints the Bison Relay UI uses. Sending
// messages, KX, and store/page edits (state-changing) come in a later phase.
var bisonrelayTools = []toolDef{
	readTool("bisonrelay", "br_status",
		"Get the Bison Relay client status (connection stage, nick, server, wallet check).",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdStatus(ctx) }),
	readTool("bisonrelay", "br_identity",
		"Get the local Bison Relay public identity (nick and public keys).",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdUserPublicIdentity(ctx) }),
	readTool("bisonrelay", "br_connection",
		"Get the Bison Relay connection state (online intent, session state, server policy).",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdConnectionState(ctx) }),
	readTool("bisonrelay", "br_contacts",
		"List Bison Relay contacts (completed key exchanges).",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdContacts(ctx) }),
	readTool("bisonrelay", "br_blocked_contacts",
		"List blocked Bison Relay contacts.",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdBlockedContacts(ctx) }),
	readTool("bisonrelay", "br_contact_groups",
		"Get the Bison Relay contact group layout (groups and per-contact assignments).",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdContactGroups(ctx) }),
	readTool("bisonrelay", "br_notifications",
		"List recent Bison Relay notifications. Optional count (default 50).",
		func(ctx context.Context, in brNotificationsInput) (any, error) {
			n := in.Count
			if n <= 0 {
				n = 50
			}
			return rpc.BrclientdRecentNotifications(ctx, n)
		}),
	readTool("bisonrelay", "br_pm_history",
		"Get paginated private-message history with a contact. Requires 'uid'; optional page and pageSize (default 50).",
		func(ctx context.Context, in brPmHistoryInput) (any, error) {
			return rpc.BrclientdHistoryPM(ctx, in.UID, in.Page, brPageSize(in.PageSize))
		}),
	readTool("bisonrelay", "br_posts",
		"Get the Bison Relay posts feed.",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdPostsFeed(ctx) }),
	readTool("bisonrelay", "br_post",
		"Get the full body of a single post. Requires 'uid' (author) and 'pid' (post id).",
		func(ctx context.Context, in brPostInput) (any, error) {
			return rpc.BrclientdPostBody(ctx, in.UID, in.PID)
		}),
	readTool("bisonrelay", "br_post_comments",
		"List the comments on a post. Requires 'uid' (author) and 'pid' (post id).",
		func(ctx context.Context, in brPostInput) (any, error) {
			return rpc.BrclientdPostComments(ctx, in.UID, in.PID)
		}),
	readTool("bisonrelay", "br_groupchats",
		"List Bison Relay group chats.",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdGCList(ctx) }),
	readTool("bisonrelay", "br_groupchat",
		"Get a group chat's details, members, and blocklist. Requires 'gcid'.",
		func(ctx context.Context, in brGCInput) (any, error) { return rpc.BrclientdGCDetail(ctx, in.GCID) }),
	readTool("bisonrelay", "br_groupchat_history",
		"Get paginated group-chat message history. Requires 'gcid'; optional page and pageSize (default 50).",
		func(ctx context.Context, in brGCHistoryInput) (any, error) {
			return rpc.BrclientdGCHistory(ctx, in.GCID, in.Page, brPageSize(in.PageSize))
		}),
	readTool("bisonrelay", "br_shared_files",
		"List files the local user is sharing over Bison Relay.",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdSharedFiles(ctx) }),
	readTool("bisonrelay", "br_stats",
		"Get a compact Bison Relay statistics overview (counters, top contacts, connection health).",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdStatsOverview(ctx) }),
	readTool("bisonrelay", "br_store",
		"Get the Bison Relay simplestore mode and configuration.",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdStoreMode(ctx) }),
	readTool("bisonrelay", "br_store_products",
		"List the Bison Relay storefront product catalog.",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdStoreProducts(ctx) }),
	readTool("bisonrelay", "br_pages",
		"List the markdown pages this node hosts over Bison Relay.",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdPagesLocalList(ctx) }),
	readTool("bisonrelay", "br_rates",
		"Get the latest DCR/USD and BTC/USD exchange rates known to brclientd.",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdRates(ctx) }),
	agentTool("bisonrelay", "br_store_save_product",
		"Create or update a product in the Bison Relay storefront. Requires a grant with Bison Relay write enabled. Price is in DCR.",
		func(ctx context.Context, a *agent, in brSaveProductInput) (any, error) {
			if err := grants.authorizeBRWrite(a.id, time.Now()); err != nil {
				recordSpend(a, "br_store_save_product", 0, 0, in.SKU, "denied", err.Error())
				return nil, err
			}
			tags := in.Tags
			if tags == nil {
				tags = []string{}
			}
			body := map[string]any{
				"sku":          in.SKU,
				"title":        in.Title,
				"description":  in.Description,
				"price":        in.Price,
				"tags":         tags,
				"shipping":     in.Shipping,
				"disabled":     in.Disabled,
				"sendfilename": in.SendFilename,
			}
			if err := rpc.BrclientdSaveStoreProduct(ctx, body); err != nil {
				recordSpend(a, "br_store_save_product", 0, 0, in.SKU, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_store_save_product", 0, 0, in.SKU, "ok", in.Title)
			return map[string]any{"sku": in.SKU, "title": in.Title, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_store_delete_product",
		"Delete a product from the Bison Relay storefront by SKU. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brDeleteProductInput) (any, error) {
			if err := grants.authorizeBRWrite(a.id, time.Now()); err != nil {
				recordSpend(a, "br_store_delete_product", 0, 0, in.SKU, "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdDeleteStoreProduct(ctx, in.SKU); err != nil {
				recordSpend(a, "br_store_delete_product", 0, 0, in.SKU, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_store_delete_product", 0, 0, in.SKU, "ok", "")
			return map[string]any{"sku": in.SKU, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_send_message",
		"Send a private message to a Bison Relay contact (text only). Requires a grant with Bison Relay write enabled. To deliver a Lightning invoice, generate it with ln_add_invoice and send the bolt11 string as the message.",
		func(ctx context.Context, a *agent, in brSendMessageInput) (any, error) {
			if err := grants.authorizeBRWrite(a.id, time.Now()); err != nil {
				recordSpend(a, "br_send_message", 0, 0, in.UID, "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdSendPM(ctx, in.UID, in.Message); err != nil {
				recordSpend(a, "br_send_message", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_send_message", 0, 0, in.UID, "ok", "")
			return map[string]any{"uid": in.UID, "sent": true}, nil
		}),
	agentTool("bisonrelay", "br_send_groupchat_message",
		"Post a message to a Bison Relay group chat. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brSendGCMessageInput) (any, error) {
			if err := grants.authorizeBRWrite(a.id, time.Now()); err != nil {
				recordSpend(a, "br_send_groupchat_message", 0, 0, in.GCID, "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdGCMessage(ctx, in.GCID, in.Message, 0); err != nil {
				recordSpend(a, "br_send_groupchat_message", 0, 0, in.GCID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_send_groupchat_message", 0, 0, in.GCID, "ok", "")
			return map[string]any{"gcid": in.GCID, "sent": true}, nil
		}),
	agentTool("bisonrelay", "br_tip_user",
		"Tip a Bison Relay contact over Lightning. Spends DCR: requires a grant with Lightning enabled, and the amount counts against the per-transaction and daily caps. dcrlnd must be unlocked.",
		func(ctx context.Context, a *agent, in brTipInput) (any, error) {
			amt, err := dcrutil.NewAmount(in.AmountDCR)
			if err != nil {
				return nil, fmt.Errorf("invalid amount: %w", err)
			}
			atoms := int64(amt)
			if err := grants.authorizeLightning(a.id, atoms, time.Now()); err != nil {
				if tripwire(a.id, err) {
					recordSpend(a, "br_tip_user", 0, in.AmountDCR, in.UID, "blocked", "spend-limit violation: grant revoked and token blocked")
				} else {
					recordSpend(a, "br_tip_user", 0, in.AmountDCR, in.UID, "denied", err.Error())
				}
				return nil, err
			}
			attempts := in.MaxAttempts
			if attempts <= 0 {
				attempts = 1
			}
			if err := rpc.BrclientdTipUser(ctx, in.UID, in.AmountDCR, attempts); err != nil {
				grants.refund(a.id, atoms)
				recordSpend(a, "br_tip_user", 0, in.AmountDCR, in.UID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_tip_user", 0, in.AmountDCR, in.UID, "ok", "tip initiated")
			return map[string]any{"uid": in.UID, "amountDcr": in.AmountDCR, "initiated": true}, nil
		}),
	agentTool("bisonrelay", "br_unshare_file",
		"Revoke a shared file on Bison Relay by file id. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brUnshareInput) (any, error) {
			if err := grants.authorizeBRWrite(a.id, time.Now()); err != nil {
				recordSpend(a, "br_unshare_file", 0, 0, in.FID, "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdUnshareFile(ctx, in.FID, in.TargetUID); err != nil {
				recordSpend(a, "br_unshare_file", 0, 0, in.FID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_unshare_file", 0, 0, in.FID, "ok", "")
			return map[string]any{"fid": in.FID, "unshared": true}, nil
		}),
}
