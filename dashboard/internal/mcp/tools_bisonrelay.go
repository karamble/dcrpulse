// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/rpc"
)

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
}
