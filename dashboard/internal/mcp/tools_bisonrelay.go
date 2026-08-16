// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
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

type brSendMessageImageInput struct {
	UID     string `json:"uid" jsonschema:"recipient identity (nick, alias, or hex UID)"`
	Path    string `json:"path" jsonschema:"relative path to the image under the agent outbox dir"`
	Message string `json:"message,omitempty" jsonschema:"optional caption text shown above the image"`
	Mime    string `json:"mime,omitempty" jsonschema:"optional MIME type; inferred from the file when empty"`
}

type brSendGroupchatImageInput struct {
	GCID    string `json:"gcid" jsonschema:"group chat id, hex"`
	Path    string `json:"path" jsonschema:"relative path to the image under the agent outbox dir"`
	Message string `json:"message,omitempty" jsonschema:"optional caption text shown above the image"`
	Mime    string `json:"mime,omitempty" jsonschema:"optional MIME type; inferred from the file when empty"`
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
	UID        string `json:"uid" jsonschema:"peer identity, hex"`
	Page       int    `json:"page,omitempty" jsonschema:"page number, 0-based"`
	PageSize   int    `json:"pageSize,omitempty" jsonschema:"messages per page (default 50)"`
	Since      int64  `json:"since,omitempty" jsonschema:"only entries with timestamp >= this unix-seconds value; scans newest-first"`
	OnlyEmbeds bool   `json:"onlyEmbeds,omitempty" jsonschema:"only entries carrying an --embed[...]-- tag (e.g. delivered images)"`
}

type brGCInput struct {
	GCID string `json:"gcid" jsonschema:"group chat id, hex"`
}

type brGCHistoryInput struct {
	GCID     string `json:"gcid" jsonschema:"group chat id, hex"`
	Page     int    `json:"page,omitempty" jsonschema:"page number, 0-based"`
	PageSize int    `json:"pageSize,omitempty" jsonschema:"messages per page (default 50)"`
}

type brPostCreateInput struct {
	Post  string `json:"post" jsonschema:"post body markdown"`
	Descr string `json:"descr,omitempty" jsonschema:"optional short description shown as the post subtitle"`
}

type brPostCommentInput struct {
	UID       string `json:"uid" jsonschema:"author identity, hex"`
	PID       string `json:"pid" jsonschema:"post id, hex"`
	Comment   string `json:"comment" jsonschema:"comment text"`
	Parent    string `json:"parent,omitempty" jsonschema:"parent comment status id (hex) to reply to; empty posts a top-level comment"`
	ImagePath string `json:"imagePath,omitempty" jsonschema:"optional image to embed: relative path under the agent outbox dir"`
	ImageMime string `json:"imageMime,omitempty" jsonschema:"optional MIME type for the embedded image"`
}

type brPostHeartInput struct {
	UID   string `json:"uid" jsonschema:"author identity, hex"`
	PID   string `json:"pid" jsonschema:"post id, hex"`
	Heart bool   `json:"heart" jsonschema:"true to add a heart, false to remove it"`
}

type brPostRelayInput struct {
	UID   string `json:"uid" jsonschema:"original author identity, hex"`
	PID   string `json:"pid" jsonschema:"post id, hex"`
	ToUID string `json:"toUid,omitempty" jsonschema:"relay target identity (hex); empty relays to all post subscribers"`
}

type brPageSaveInput struct {
	Name    string `json:"name" jsonschema:"page name or relative path (no leading slash, no .. segments)"`
	Content string `json:"content" jsonschema:"page markdown content"`
}

type brPageDeleteInput struct {
	Name string `json:"name" jsonschema:"page name or relative path to delete"`
}

type brPageGetInput struct {
	Name string `json:"name" jsonschema:"page name or relative path to read"`
}

type brEmbedGetInput struct {
	Localfilename string `json:"localfilename" jsonschema:"received-embed reference exactly as PM history reports it: embeds/<uid16>/<file>"`
}

type brPageImportEmbedInput struct {
	Source string `json:"source" jsonschema:"received-embed reference from PM history: embeds/<uid16>/<file>"`
	Dest   string `json:"dest" jsonschema:"image path to create inside the pages directory, e.g. articles/img/hero.jpg"`
}

type brFileSendInput struct {
	UID      string `json:"uid" jsonschema:"recipient identity (nick, alias, or hex UID)"`
	Filename string `json:"filename" jsonschema:"file name to present to the recipient"`
	Mime     string `json:"mime,omitempty" jsonschema:"optional MIME type of the file"`
	DataB64  string `json:"dataB64" jsonschema:"file bytes, base64-encoded"`
}

type brFileSendPathInput struct {
	UID      string `json:"uid" jsonschema:"recipient identity (nick, alias, or hex UID)"`
	Path     string `json:"path" jsonschema:"relative path under the agent outbox dir (no leading slash, no .. segments)"`
	Filename string `json:"filename,omitempty" jsonschema:"optional name to present to the recipient; defaults to the file's base name"`
	Mime     string `json:"mime,omitempty" jsonschema:"optional MIME type of the file"`
}

type brFileAddInput struct {
	Filename  string  `json:"filename" jsonschema:"file name for the share"`
	Mime      string  `json:"mime,omitempty" jsonschema:"optional MIME type of the file"`
	DataB64   string  `json:"dataB64" jsonschema:"file bytes, base64-encoded"`
	CostDCR   float64 `json:"costDcr,omitempty" jsonschema:"per-download price in DCR; 0 makes the file free"`
	TargetUID string  `json:"targetUid,omitempty" jsonschema:"empty shares globally; a hex UID shares only with that contact"`
	Descr     string  `json:"descr,omitempty" jsonschema:"optional description for the share"`
}

type brStoreOrderStatusInput struct {
	UID    string `json:"uid" jsonschema:"buyer identity, hex"`
	ID     uint64 `json:"id" jsonschema:"order id"`
	Status string `json:"status" jsonschema:"new order status"`
}

type brStoreOrderCommentInput struct {
	UID     string `json:"uid" jsonschema:"buyer identity, hex"`
	ID      uint64 `json:"id" jsonschema:"order id"`
	Comment string `json:"comment" jsonschema:"comment to send to the buyer"`
}

type brStoreFileUploadInput struct {
	Path      string `json:"path,omitempty" jsonschema:"relative destination path under the store dir (no leading slash, no .. segments)"`
	Filename  string `json:"filename" jsonschema:"file name (no path separators)"`
	Mime      string `json:"mime,omitempty" jsonschema:"optional MIME type of the file"`
	DataB64   string `json:"dataB64" jsonschema:"file bytes, base64-encoded"`
	Overwrite bool   `json:"overwrite,omitempty" jsonschema:"overwrite an existing file at the destination (default false)"`
}

type brDownloadDeleteInput struct {
	FID string `json:"fid" jsonschema:"file id, hex"`
	UID string `json:"uid,omitempty" jsonschema:"optional sender identity (hex) to disambiguate the same file from multiple peers"`
}

type brNotificationDeleteInput struct {
	ID int64 `json:"id" jsonschema:"notification id to delete"`
}

type brStorePathInput struct {
	Path string `json:"path" jsonschema:"relative store file path (no leading slash, no .. segments)"`
}

type brStoreTemplateSaveInput struct {
	Name    string `json:"name" jsonschema:"template name or relative path (no leading slash, no .. segments)"`
	Content string `json:"content" jsonschema:"template content"`
}

type brStoreTemplateDeleteInput struct {
	Name string `json:"name" jsonschema:"template name or relative path to delete"`
}

type brGCCreateInput struct {
	Name string `json:"name" jsonschema:"group chat name"`
}

type brGCInviteInput struct {
	GCID string `json:"gcid" jsonschema:"group chat id, hex"`
	UID  string `json:"uid" jsonschema:"contact identity to invite, hex"`
}

type brGCInvitesAcceptInput struct {
	IID uint64 `json:"iid" jsonschema:"pending invite id"`
}

type brGCPartInput struct {
	GCID   string `json:"gcid" jsonschema:"group chat id, hex"`
	Reason string `json:"reason,omitempty" jsonschema:"optional reason"`
}

type brGCKickInput struct {
	GCID   string `json:"gcid" jsonschema:"group chat id, hex"`
	UID    string `json:"uid" jsonschema:"member identity to kick, hex"`
	Reason string `json:"reason,omitempty" jsonschema:"optional reason"`
}

type brGCKillInput struct {
	GCID   string `json:"gcid" jsonschema:"group chat id, hex"`
	Reason string `json:"reason,omitempty" jsonschema:"optional reason"`
}

type brGCMemberInput struct {
	GCID string `json:"gcid" jsonschema:"group chat id, hex"`
	UID  string `json:"uid" jsonschema:"member identity, hex"`
}

type brGCAdminsInput struct {
	GCID        string   `json:"gcid" jsonschema:"group chat id, hex"`
	ExtraAdmins []string `json:"extraAdmins" jsonschema:"complete replacement list of extra admin identities, hex"`
	Reason      string   `json:"reason,omitempty" jsonschema:"optional reason"`
}

type brGCOwnerInput struct {
	GCID     string `json:"gcid" jsonschema:"group chat id, hex"`
	NewOwner string `json:"newOwner" jsonschema:"identity of the new owner, hex"`
	Reason   string `json:"reason,omitempty" jsonschema:"optional reason"`
}

type brRTDTCreateInput struct {
	Size        uint16 `json:"size,omitempty" jsonschema:"max session size"`
	Description string `json:"description,omitempty" jsonschema:"optional session description"`
}

type brRTDTCreateInstantInput struct {
	UIDs []string `json:"uids" jsonschema:"identities to call, hex"`
}

type brRTDTInviteInput struct {
	RV          string   `json:"rv" jsonschema:"session rendezvous id"`
	UIDs        []string `json:"uids" jsonschema:"identities to invite, hex"`
	AsPublisher bool     `json:"asPublisher,omitempty" jsonschema:"invite as a publisher (can speak) rather than a listener"`
}

type brRTDTAcceptInput struct {
	RV          string `json:"rv" jsonschema:"session rendezvous id"`
	Inviter     string `json:"inviter" jsonschema:"identity that sent the invite, hex"`
	AsPublisher bool   `json:"asPublisher,omitempty" jsonschema:"join as a publisher rather than a listener"`
}

type brRTDTRVInput struct {
	RV string `json:"rv" jsonschema:"session rendezvous id"`
}

type brRTDTChatInput struct {
	RV      string `json:"rv" jsonschema:"session rendezvous id"`
	Message string `json:"message" jsonschema:"message text to send into the session"`
}

type brRTDTKickInput struct {
	RV         string `json:"rv" jsonschema:"session rendezvous id"`
	PeerID     uint32 `json:"peerId" jsonschema:"live peer id to kick"`
	BanSeconds int64  `json:"banSeconds,omitempty" jsonschema:"ban duration in seconds; 0 for no ban"`
}

type brRTDTRemoveInput struct {
	RV     string `json:"rv" jsonschema:"session rendezvous id"`
	UID    string `json:"uid" jsonschema:"member identity to remove, hex"`
	Reason string `json:"reason,omitempty" jsonschema:"optional reason"`
}

func brPageSize(n int) int {
	if n <= 0 {
		return 50
	}
	return n
}

var brUIDRe = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// brContactEntries fetches the brclientd address book and returns its
// entries as generic maps, so this layer never has to track BR's
// AddressBookEntry shape.
func brContactEntries(ctx context.Context) ([]map[string]any, error) {
	raw, err := rpc.BrclientdContacts(ctx)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode contacts: %w", err)
	}
	return envelope.Entries, nil
}

// brContactStrings pulls the identity handles out of one contacts entry.
func brContactStrings(entry map[string]any) (uid, nick, alias, name string) {
	if id, ok := entry["id"].(map[string]any); ok {
		uid, _ = id["identity"].(string)
		nick, _ = id["nick"].(string)
		name, _ = id["name"].(string)
	}
	alias, _ = entry["nick_alias"].(string)
	return
}

// bisonrelayTools are the read-only "bisonrelay" domain tools. They call the
// same brclientd status-server endpoints the Bison Relay UI uses. Sending
// messages, KX, and store/page edits (state-changing) come in a later phase.
type brPageFetchInput struct {
	UID        string   `json:"uid" jsonschema:"contact hex UID whose page to fetch"`
	Path       []string `json:"path,omitempty" jsonschema:"page path segments; defaults to [\"index.md\"] (the contact's root page). Follow a link by passing its path segments."`
	SessionID  uint64   `json:"sessionId,omitempty" jsonschema:"session id from a prior fetch, to navigate within the same session"`
	ParentPage uint64   `json:"parentPage,omitempty" jsonschema:"parent page id from a prior fetch"`
}

type brPageSubmitInput struct {
	UID        string            `json:"uid" jsonschema:"contact hex UID hosting the page"`
	Path       []string          `json:"path" jsonschema:"page path segments to submit to (the form's action), e.g. [\"addToCart\"] or [\"placeOrder\"]"`
	Data       map[string]any    `json:"data" jsonschema:"form data payload as a JSON object matching the page's form fields, e.g. {\"sku\":\"abc\",\"qty\":1}"`
	SessionID  uint64            `json:"sessionId,omitempty" jsonschema:"session id from a prior fetch"`
	FieldTypes map[string]string `json:"fieldTypes,omitempty" jsonschema:"optional per-field type hints so the store coerces values, e.g. qty=intinput; usually unnecessary when data already uses correct JSON types"`
}

type brShopUIDInput struct {
	UID string `json:"uid" jsonschema:"merchant contact hex UID"`
}

type brShopAddToCartInput struct {
	UID      string `json:"uid" jsonschema:"merchant contact hex UID"`
	SKU      string `json:"sku" jsonschema:"product SKU to add"`
	Quantity uint32 `json:"quantity,omitempty" jsonschema:"quantity to add; defaults to 1"`
}

type brShopOrderInput struct {
	UID string `json:"uid" jsonschema:"merchant contact hex UID"`
	ID  uint64 `json:"id" jsonschema:"order id"`
}

type brShopShippingAddress struct {
	Name        string `json:"name,omitempty" jsonschema:"recipient name"`
	Address1    string `json:"address1,omitempty" jsonschema:"address line 1"`
	Address2    string `json:"address2,omitempty" jsonschema:"address line 2"`
	City        string `json:"city,omitempty" jsonschema:"city"`
	State       string `json:"state,omitempty" jsonschema:"state or region"`
	PostalCode  string `json:"postalCode,omitempty" jsonschema:"postal code"`
	Phone       string `json:"phone,omitempty" jsonschema:"phone number"`
	CountryCode string `json:"countrycode,omitempty" jsonschema:"ISO country code"`
}

type brShopPlaceOrderInput struct {
	UID      string                 `json:"uid" jsonschema:"merchant contact hex UID"`
	Shipping *brShopShippingAddress `json:"shipping,omitempty" jsonschema:"shipping address; required only when an ordered product needs shipping"`
}

type brShopOrderCommentInput struct {
	UID     string `json:"uid" jsonschema:"merchant contact hex UID"`
	ID      uint64 `json:"id" jsonschema:"order id"`
	Comment string `json:"comment" jsonschema:"comment to add to the order"`
}

type brContentGetInput struct {
	UID          string `json:"uid" jsonschema:"contact hex UID hosting the shared file"`
	FID          string `json:"fid" jsonschema:"shared-file id (from a page download embed)"`
	MaxCostAtoms uint64 `json:"maxCostAtoms,omitempty" jsonschema:"maximum total atoms the download may cost; 0 (default) accepts only free files. A nonzero ceiling is paid over Lightning and needs the lightning write scope; the whole ceiling is reserved against the spend caps and is not refunded."`
}

type brFidInput struct {
	FID string `json:"fid" jsonschema:"shared-file id of the in-flight download"`
}

type brResolveNickInput struct {
	Nick string `json:"nick" jsonschema:"nick, local alias, or name to resolve"`
}

type brResolveUIDInput struct {
	UID string `json:"uid" jsonschema:"contact identity, 64-char hex"`
}

type brContactAvatarInput struct {
	UID string `json:"uid" jsonschema:"contact identity, 64-char hex"`
}

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
		"List Bison Relay contacts (completed key exchanges). Avatar bytes and key material are omitted; entries carry hasAvatar and br_contact_avatar fetches an avatar.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			entries, err := brContactEntries(ctx)
			if err != nil {
				return nil, err
			}
			for _, entry := range entries {
				var has bool
				if id, ok := entry["id"].(map[string]any); ok {
					if avatar, _ := id["avatar"].(string); avatar != "" {
						has = true
					}
					// Avatar bytes and raw key material are noise to an
					// agent; identity handles and timestamps remain.
					for _, k := range []string{"avatar", "key", "sigKey", "signature", "digest"} {
						delete(id, k)
					}
				}
				delete(entry, "myResetRV")
				delete(entry, "theirResetRV")
				entry["hasAvatar"] = has
			}
			return map[string]any{"entries": entries}, nil
		}),
	readTool("bisonrelay", "br_resolve_nick",
		"Resolve a contact nick, local alias, or name to its 64-hex uid. Nicks are not unique, so ALL matches are returned.",
		func(ctx context.Context, in brResolveNickInput) (any, error) {
			want := strings.TrimSpace(in.Nick)
			if want == "" {
				return nil, fmt.Errorf("nick is required")
			}
			entries, err := brContactEntries(ctx)
			if err != nil {
				return nil, err
			}
			matches := []map[string]string{}
			for _, entry := range entries {
				uid, nick, alias, name := brContactStrings(entry)
				if strings.EqualFold(nick, want) || strings.EqualFold(alias, want) ||
					strings.EqualFold(name, want) {
					matches = append(matches, map[string]string{
						"uid": uid, "nick": nick, "alias": alias, "name": name,
					})
				}
			}
			return map[string]any{"matches": matches, "count": len(matches)}, nil
		}),
	readTool("bisonrelay", "br_resolve_uid",
		"Resolve a contact's 64-hex uid to its nick, local alias, and name.",
		func(ctx context.Context, in brResolveUIDInput) (any, error) {
			if !brUIDRe.MatchString(in.UID) {
				return nil, fmt.Errorf("uid must be 64 hex characters")
			}
			entries, err := brContactEntries(ctx)
			if err != nil {
				return nil, err
			}
			for _, entry := range entries {
				uid, nick, alias, name := brContactStrings(entry)
				if strings.EqualFold(uid, in.UID) {
					return map[string]string{
						"uid": uid, "nick": nick, "alias": alias, "name": name,
					}, nil
				}
			}
			return nil, fmt.Errorf("no contact with uid %s", in.UID)
		}),
	readTool("bisonrelay", "br_contact_avatar",
		"Get a contact's avatar image by 64-hex uid, as base64 with a sniffed content type.",
		func(ctx context.Context, in brContactAvatarInput) (any, error) {
			if !brUIDRe.MatchString(in.UID) {
				return nil, fmt.Errorf("uid must be 64 hex characters")
			}
			entries, err := brContactEntries(ctx)
			if err != nil {
				return nil, err
			}
			for _, entry := range entries {
				uid, nick, _, _ := brContactStrings(entry)
				if !strings.EqualFold(uid, in.UID) {
					continue
				}
				var avatarB64 string
				if id, ok := entry["id"].(map[string]any); ok {
					avatarB64, _ = id["avatar"].(string)
				}
				if avatarB64 == "" {
					return nil, fmt.Errorf("contact %s has no avatar", nick)
				}
				data, err := base64.StdEncoding.DecodeString(avatarB64)
				if err != nil {
					return nil, fmt.Errorf("decode avatar: %w", err)
				}
				return map[string]any{
					"uid":         uid,
					"nick":        nick,
					"contentType": http.DetectContentType(data),
					"dataB64":     avatarB64,
				}, nil
			}
			return nil, fmt.Errorf("no contact with uid %s", in.UID)
		}),
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
		"Get paginated private-message history with a contact. Requires 'uid'; optional page and pageSize (default 50). Optional 'since' (unix seconds) and 'onlyEmbeds' filters scan newest-first and return only matching entries.",
		func(ctx context.Context, in brPmHistoryInput) (any, error) {
			if in.Since <= 0 && !in.OnlyEmbeds {
				return rpc.BrclientdHistoryPM(ctx, in.UID, in.Page, brPageSize(in.PageSize))
			}
			return filteredPMHistory(ctx, in)
		}),
	readTool("bisonrelay", "br_embed_get",
		"Read a received chat embed image by the embeds/<uid16>/<file> localfilename PM history reports. Raster images only (jpeg/png/gif/webp).",
		func(ctx context.Context, in brEmbedGetInput) (any, error) {
			if !pageImageExtRE.MatchString(in.Localfilename) {
				return nil, fmt.Errorf("not a raster image embed")
			}
			data, err := readChatEmbed(ctx, in.Localfilename)
			if err != nil {
				return nil, err
			}
			contentType := http.DetectContentType(data)
			switch contentType {
			case "image/jpeg", "image/png", "image/gif", "image/webp":
			default:
				return nil, fmt.Errorf("not a raster image embed")
			}
			return map[string]any{
				"localfilename": in.Localfilename,
				"contentType":   contentType,
				"sizeBytes":     len(data),
				"dataB64":       base64.StdEncoding.EncodeToString(data),
			}, nil
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
		func(ctx context.Context, in brGCInput) (any, error) {
			gcid, err := rpc.ParseShortIDHex(in.GCID)
			if err != nil {
				return nil, err
			}
			return rpc.BrclientdGCDetail(ctx, gcid)
		}),
	readTool("bisonrelay", "br_groupchat_history",
		"Get paginated group-chat message history. Requires 'gcid'; optional page and pageSize (default 50).",
		func(ctx context.Context, in brGCHistoryInput) (any, error) {
			gcid, err := rpc.ParseShortIDHex(in.GCID)
			if err != nil {
				return nil, err
			}
			return rpc.BrclientdGCHistory(ctx, gcid, in.Page, brPageSize(in.PageSize))
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
	readTool("bisonrelay", "br_page_get",
		"Read the raw markdown of one page this node hosts. Requires 'name'.",
		func(ctx context.Context, in brPageGetInput) (any, error) {
			if !safeBRName(in.Name) {
				return nil, fmt.Errorf("invalid name")
			}
			return rpc.BrclientdPagesLocalFile(ctx, in.Name)
		}),
	readTool("bisonrelay", "br_rates",
		"Get the latest DCR/USD and BTC/USD exchange rates known to brclientd.",
		func(ctx context.Context, _ emptyInput) (any, error) { return rpc.BrclientdRates(ctx) }),
	agentTool("bisonrelay", "br_store_save_product",
		"Create or update a product in the Bison Relay storefront. Requires a grant with Bison Relay write enabled. Price is in DCR.",
		func(ctx context.Context, a *agent, in brSaveProductInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
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
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
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
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
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
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_send_groupchat_message", 0, 0, in.GCID, "denied", err.Error())
				return nil, err
			}
			gcid, err := rpc.ParseShortIDHex(in.GCID)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdGCMessage(ctx, gcid, in.Message, 0); err != nil {
				recordSpend(a, "br_send_groupchat_message", 0, 0, in.GCID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_send_groupchat_message", 0, 0, in.GCID, "ok", "")
			return map[string]any{"gcid": in.GCID, "sent": true}, nil
		}),
	agentTool("bisonrelay", "br_send_message_image",
		"Send a private message with an image attached inline in the chat (renders in the message bubble, not as a separate file download). The image is read from the agent outbox by relative path; the agent supplies only the path and an optional caption, and the tool reads the file and embeds it (no base64 handling by the agent). Inline images are capped at 800 KiB; for larger files use br_file_send_path. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brSendMessageImageInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_send_message_image", 0, 0, in.UID, "denied", err.Error())
				return nil, err
			}
			body, err := buildImageEmbedBody(in.Path, in.Message, in.Mime)
			if err != nil {
				recordSpend(a, "br_send_message_image", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdSendPM(ctx, in.UID, body); err != nil {
				recordSpend(a, "br_send_message_image", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_send_message_image", 0, 0, in.UID, "ok", filepath.Base(in.Path))
			return map[string]any{"uid": in.UID, "sent": true}, nil
		}),
	agentTool("bisonrelay", "br_send_groupchat_image",
		"Post a group-chat message with an image attached inline (renders in the message, not as a separate file download). The image is read from the agent outbox by relative path; the agent supplies only the path and an optional caption, and the tool reads the file and embeds it (no base64 handling by the agent). Inline images are capped at 800 KiB; for larger files use br_file_send_path. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brSendGroupchatImageInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_send_groupchat_image", 0, 0, in.GCID, "denied", err.Error())
				return nil, err
			}
			body, err := buildImageEmbedBody(in.Path, in.Message, in.Mime)
			if err != nil {
				recordSpend(a, "br_send_groupchat_image", 0, 0, in.GCID, "error", err.Error())
				return nil, err
			}
			gcid, err := rpc.ParseShortIDHex(in.GCID)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdGCMessage(ctx, gcid, body, 0); err != nil {
				recordSpend(a, "br_send_groupchat_image", 0, 0, in.GCID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_send_groupchat_image", 0, 0, in.GCID, "ok", filepath.Base(in.Path))
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
			if err := grants.authorizeLightning(ctx, a.id, atoms, time.Now()); err != nil {
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
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
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
	agentTool("bisonrelay", "br_post_create",
		"Author a new Bison Relay post. Requires a grant with Bison Relay write enabled. Returns the created post metadata.",
		func(ctx context.Context, a *agent, in brPostCreateInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_post_create", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			body, err := rpc.BrclientdCreatePost(ctx, in.Post, in.Descr)
			if err != nil {
				recordSpend(a, "br_post_create", 0, 0, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_post_create", 0, 0, "", "ok", "")
			return decodeBRResult(body), nil
		}),
	agentTool("bisonrelay", "br_post_comment",
		"Comment on a Bison Relay post (or reply to a comment via 'parent'), optionally embedding an image from the agent outbox via imagePath. Requires a grant with Bison Relay write enabled. Returns the new comment identifier.",
		func(ctx context.Context, a *agent, in brPostCommentInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_post_comment", 0, 0, in.PID, "denied", err.Error())
				return nil, err
			}
			comment := in.Comment
			if in.ImagePath != "" {
				embedded, err := buildImageEmbedBody(in.ImagePath, in.Comment, in.ImageMime)
				if err != nil {
					recordSpend(a, "br_post_comment", 0, 0, in.PID, "error", err.Error())
					return nil, err
				}
				comment = embedded
			}
			identifier, err := rpc.BrclientdPostComment(ctx, in.UID, in.PID, comment, in.Parent)
			if err != nil {
				recordSpend(a, "br_post_comment", 0, 0, in.PID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_post_comment", 0, 0, in.PID, "ok", "")
			return map[string]any{"pid": in.PID, "identifier": identifier, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_post_heart",
		"Add or remove the local identity's heart on a Bison Relay post. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brPostHeartInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_post_heart", 0, 0, in.PID, "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdPostHeart(ctx, in.UID, in.PID, in.Heart); err != nil {
				recordSpend(a, "br_post_heart", 0, 0, in.PID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_post_heart", 0, 0, in.PID, "ok", "")
			return map[string]any{"pid": in.PID, "heart": in.Heart, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_post_relay",
		"Relay a Bison Relay post to one contact, or to all post subscribers when 'toUid' is empty. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brPostRelayInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_post_relay", 0, 0, in.PID, "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdRelayPost(ctx, in.UID, in.PID, in.ToUID); err != nil {
				recordSpend(a, "br_post_relay", 0, 0, in.PID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_post_relay", 0, 0, in.PID, "ok", "")
			return map[string]any{"pid": in.PID, "relayed": true}, nil
		}),
	agentTool("bisonrelay", "br_page_save",
		"Create or overwrite a markdown page this node hosts over Bison Relay. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brPageSaveInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_page_save", 0, 0, in.Name, "denied", err.Error())
				return nil, err
			}
			if !safeBRName(in.Name) {
				err := fmt.Errorf("invalid name")
				recordSpend(a, "br_page_save", 0, 0, in.Name, "error", err.Error())
				return nil, err
			}
			body := map[string]any{"name": in.Name, "content": in.Content}
			if err := rpc.BrclientdPagesLocalSave(ctx, body); err != nil {
				recordSpend(a, "br_page_save", 0, 0, in.Name, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_page_save", 0, 0, in.Name, "ok", "")
			return map[string]any{"name": in.Name, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_page_import_embed",
		"Copy a received chat embed image into the pages directory server-side, so a page can reference it via an embed localfilename without the bytes transiting the agent. Imported assets cannot be deleted via MCP. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brPageImportEmbedInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_page_import_embed", 0, 0, in.Dest, "denied", err.Error())
				return nil, err
			}
			if !chatEmbedRE.MatchString(in.Source) || strings.Contains(in.Source, "..") {
				err := fmt.Errorf("invalid source: must be embeds/<uid16>/<file>")
				recordSpend(a, "br_page_import_embed", 0, 0, in.Dest, "error", err.Error())
				return nil, err
			}
			if !safeBRName(in.Dest) || !pageImageExtRE.MatchString(in.Dest) {
				err := fmt.Errorf("invalid dest: must be an image path (jpg/jpeg/jfif/png/gif/webp) inside the pages directory")
				recordSpend(a, "br_page_import_embed", 0, 0, in.Dest, "error", err.Error())
				return nil, err
			}
			res, err := rpc.BrclientdPagesImportEmbed(ctx, map[string]any{"source": in.Source, "dest": in.Dest})
			if err != nil {
				recordSpend(a, "br_page_import_embed", 0, 0, in.Dest, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_page_import_embed", 0, 0, in.Dest, "ok", "")
			return res, nil
		}),
	agentTool("bisonrelay", "br_page_delete",
		"Delete a hosted Bison Relay page by name. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brPageDeleteInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_page_delete", 0, 0, in.Name, "denied", err.Error())
				return nil, err
			}
			if !safeBRName(in.Name) {
				err := fmt.Errorf("invalid name")
				recordSpend(a, "br_page_delete", 0, 0, in.Name, "error", err.Error())
				return nil, err
			}
			body := map[string]any{"name": in.Name}
			if err := rpc.BrclientdPagesLocalDelete(ctx, body); err != nil {
				recordSpend(a, "br_page_delete", 0, 0, in.Name, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_page_delete", 0, 0, in.Name, "ok", "")
			return map[string]any{"name": in.Name, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_file_send",
		"Send a file directly to a Bison Relay contact. The file bytes are supplied base64-encoded in 'dataB64'. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brFileSendInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_file_send", 0, 0, in.UID, "denied", err.Error())
				return nil, err
			}
			data, err := base64.StdEncoding.DecodeString(in.DataB64)
			if err != nil {
				err = fmt.Errorf("invalid dataB64: %w", err)
				recordSpend(a, "br_file_send", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			result, err := rpc.BrclientdSendFile(ctx, in.UID, in.Filename, in.Mime, bytes.NewReader(data))
			if err != nil {
				recordSpend(a, "br_file_send", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_file_send", 0, 0, in.UID, "ok", in.Filename)
			return map[string]any{"uid": in.UID, "filename": in.Filename, "result": result, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_file_send_path",
		"Send a file to a Bison Relay contact by reading it from the agent outbox directory, avoiding base64 for large files. 'path' is relative to that directory (set via MCP_AGENT_OUTBOX_DIR, default <brclientd-data>/agent-outbox); absolute paths and '..' segments are rejected. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brFileSendPathInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_file_send_path", 0, 0, in.UID, "denied", err.Error())
				return nil, err
			}
			candidate, err := resolveOutboxPath(in.Path)
			if err != nil {
				recordSpend(a, "br_file_send_path", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			f, err := os.Open(candidate)
			if err != nil {
				recordSpend(a, "br_file_send_path", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			defer f.Close()
			fi, err := f.Stat()
			if err != nil {
				recordSpend(a, "br_file_send_path", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			if fi.IsDir() {
				err := fmt.Errorf("path is a directory")
				recordSpend(a, "br_file_send_path", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			name := in.Filename
			if name == "" {
				name = filepath.Base(candidate)
			}
			result, err := rpc.BrclientdSendFile(ctx, in.UID, name, in.Mime, f)
			if err != nil {
				recordSpend(a, "br_file_send_path", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_file_send_path", 0, 0, in.UID, "ok", name)
			return map[string]any{"uid": in.UID, "filename": name, "result": result, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_file_add",
		"Add a file to Bison Relay shared files (globally or scoped to one contact). The file bytes are supplied base64-encoded in 'dataB64'; an optional per-download cost is set in DCR. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brFileAddInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_file_add", 0, 0, in.Filename, "denied", err.Error())
				return nil, err
			}
			data, err := base64.StdEncoding.DecodeString(in.DataB64)
			if err != nil {
				err = fmt.Errorf("invalid dataB64: %w", err)
				recordSpend(a, "br_file_add", 0, 0, in.Filename, "error", err.Error())
				return nil, err
			}
			if in.CostDCR < 0 {
				err := fmt.Errorf("costDcr must not be negative")
				recordSpend(a, "br_file_add", 0, 0, in.Filename, "error", err.Error())
				return nil, err
			}
			amt, err := dcrutil.NewAmount(in.CostDCR)
			if err != nil {
				err = fmt.Errorf("invalid costDcr: %w", err)
				recordSpend(a, "br_file_add", 0, 0, in.Filename, "error", err.Error())
				return nil, err
			}
			body, err := rpc.BrclientdShareFile(ctx, in.Filename, in.Mime, bytes.NewReader(data), uint64(amt), in.TargetUID, in.Descr)
			if err != nil {
				recordSpend(a, "br_file_add", 0, 0, in.Filename, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_file_add", 0, 0, in.Filename, "ok", in.Filename)
			return decodeBRResult(body), nil
		}),
	agentTool("bisonrelay", "br_store_order_status",
		"Update the status of a Bison Relay storefront order. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brStoreOrderStatusInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_store_order_status", 0, 0, in.UID, "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdSetStoreOrderStatus(ctx, in.UID, in.ID, in.Status); err != nil {
				recordSpend(a, "br_store_order_status", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_store_order_status", 0, 0, in.UID, "ok", in.Status)
			return map[string]any{"uid": in.UID, "id": in.ID, "status": in.Status, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_store_order_comment",
		"Append a merchant comment to a Bison Relay storefront order; brclientd DMs the buyer. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brStoreOrderCommentInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_store_order_comment", 0, 0, in.UID, "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdAddStoreOrderComment(ctx, in.UID, in.ID, in.Comment); err != nil {
				recordSpend(a, "br_store_order_comment", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_store_order_comment", 0, 0, in.UID, "ok", "")
			return map[string]any{"uid": in.UID, "id": in.ID, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_store_file_upload",
		"Upload a digital-download file into the Bison Relay storefront. The file bytes are supplied base64-encoded in 'dataB64'. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brStoreFileUploadInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_store_file_upload", 0, 0, in.Filename, "denied", err.Error())
				return nil, err
			}
			if in.Path != "" && !safeBRName(in.Path) {
				err := fmt.Errorf("invalid path")
				recordSpend(a, "br_store_file_upload", 0, 0, in.Filename, "error", err.Error())
				return nil, err
			}
			if !safeBRName(in.Filename) || containsSlash(in.Filename) {
				err := fmt.Errorf("invalid filename")
				recordSpend(a, "br_store_file_upload", 0, 0, in.Filename, "error", err.Error())
				return nil, err
			}
			data, err := base64.StdEncoding.DecodeString(in.DataB64)
			if err != nil {
				err = fmt.Errorf("invalid dataB64: %w", err)
				recordSpend(a, "br_store_file_upload", 0, 0, in.Filename, "error", err.Error())
				return nil, err
			}
			body, err := rpc.BrclientdUploadStoreFile(ctx, in.Path, in.Filename, in.Mime, in.Overwrite, bytes.NewReader(data))
			if err != nil {
				recordSpend(a, "br_store_file_upload", 0, 0, in.Filename, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_store_file_upload", 0, 0, in.Filename, "ok", in.Filename)
			return decodeBRResult(body), nil
		}),
	agentTool("bisonrelay", "br_store_template_save",
		"Create or overwrite a Bison Relay storefront template file. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brStoreTemplateSaveInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_store_template_save", 0, 0, in.Name, "denied", err.Error())
				return nil, err
			}
			if !safeBRName(in.Name) {
				err := fmt.Errorf("invalid name")
				recordSpend(a, "br_store_template_save", 0, 0, in.Name, "error", err.Error())
				return nil, err
			}
			body := map[string]any{"name": in.Name, "content": in.Content}
			if err := rpc.BrclientdSaveStoreTemplate(ctx, body); err != nil {
				recordSpend(a, "br_store_template_save", 0, 0, in.Name, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_store_template_save", 0, 0, in.Name, "ok", "")
			return map[string]any{"name": in.Name, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_store_template_delete",
		"Delete a Bison Relay storefront template file by name. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brStoreTemplateDeleteInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_store_template_delete", 0, 0, in.Name, "denied", err.Error())
				return nil, err
			}
			if !safeBRName(in.Name) {
				err := fmt.Errorf("invalid name")
				recordSpend(a, "br_store_template_delete", 0, 0, in.Name, "error", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdDeleteStoreTemplate(ctx, in.Name); err != nil {
				recordSpend(a, "br_store_template_delete", 0, 0, in.Name, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_store_template_delete", 0, 0, in.Name, "ok", "")
			return map[string]any{"name": in.Name, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_gc_create",
		"Create a new Bison Relay group chat. Requires a grant with Bison Relay write enabled. Returns the new group chat metadata.",
		func(ctx context.Context, a *agent, in brGCCreateInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_gc_create", 0, 0, in.Name, "denied", err.Error())
				return nil, err
			}
			body, err := rpc.BrclientdGCCreate(ctx, in.Name)
			if err != nil {
				recordSpend(a, "br_gc_create", 0, 0, in.Name, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_gc_create", 0, 0, in.Name, "ok", "")
			return decodeBRResult(body), nil
		}),
	agentTool("bisonrelay", "br_gc_invite",
		"Invite a contact to a Bison Relay group chat. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brGCInviteInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_gc_invite", 0, 0, in.GCID, "denied", err.Error())
				return nil, err
			}
			gcid, err := rpc.ParseShortIDHex(in.GCID)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdGCInvite(ctx, gcid, in.UID); err != nil {
				recordSpend(a, "br_gc_invite", 0, 0, in.GCID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_gc_invite", 0, 0, in.GCID, "ok", in.UID)
			return map[string]any{"gcid": in.GCID, "uid": in.UID, "invited": true}, nil
		}),
	agentTool("bisonrelay", "br_gc_invites_accept",
		"Accept a pending Bison Relay group-chat invite by invite id. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brGCInvitesAcceptInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_gc_invites_accept", 0, 0, fmt.Sprintf("%d", in.IID), "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdGCInvitesAccept(ctx, in.IID); err != nil {
				recordSpend(a, "br_gc_invites_accept", 0, 0, fmt.Sprintf("%d", in.IID), "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_gc_invites_accept", 0, 0, fmt.Sprintf("%d", in.IID), "ok", "")
			return map[string]any{"iid": in.IID, "accepted": true}, nil
		}),
	agentTool("bisonrelay", "br_gc_part",
		"Leave a Bison Relay group chat (non-owner). Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brGCPartInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_gc_part", 0, 0, in.GCID, "denied", err.Error())
				return nil, err
			}
			gcid, err := rpc.ParseShortIDHex(in.GCID)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdGCPart(ctx, gcid, in.Reason); err != nil {
				recordSpend(a, "br_gc_part", 0, 0, in.GCID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_gc_part", 0, 0, in.GCID, "ok", "")
			return map[string]any{"gcid": in.GCID, "parted": true}, nil
		}),
	agentTool("bisonrelay", "br_rtdt_create",
		"Create a Bison Relay realtime-voice (RTDT) session. Requires a grant with Bison Relay write enabled. Returns the session metadata.",
		func(ctx context.Context, a *agent, in brRTDTCreateInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_rtdt_create", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			body, err := rpc.BrclientdRTDTCreate(ctx, in.Size, in.Description)
			if err != nil {
				recordSpend(a, "br_rtdt_create", 0, 0, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_rtdt_create", 0, 0, "", "ok", "")
			return decodeBRResult(body), nil
		}),
	agentTool("bisonrelay", "br_rtdt_create_instant",
		"Create an instant Bison Relay realtime-voice call to a set of contacts. Requires a grant with Bison Relay write enabled. Returns the session metadata.",
		func(ctx context.Context, a *agent, in brRTDTCreateInstantInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_rtdt_create_instant", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			body, err := rpc.BrclientdRTDTCreateInstant(ctx, in.UIDs)
			if err != nil {
				recordSpend(a, "br_rtdt_create_instant", 0, 0, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_rtdt_create_instant", 0, 0, "", "ok", "")
			return decodeBRResult(body), nil
		}),
	agentTool("bisonrelay", "br_rtdt_invite",
		"Invite contacts to an existing Bison Relay realtime-voice session. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brRTDTInviteInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_rtdt_invite", 0, 0, in.RV, "denied", err.Error())
				return nil, err
			}
			rv, err := rpc.ParseShortIDHex(in.RV)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdRTDTInvite(ctx, rv, in.UIDs, in.AsPublisher); err != nil {
				recordSpend(a, "br_rtdt_invite", 0, 0, in.RV, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_rtdt_invite", 0, 0, in.RV, "ok", "")
			return map[string]any{"rv": in.RV, "invited": true}, nil
		}),
	agentTool("bisonrelay", "br_rtdt_accept",
		"Accept a pending Bison Relay realtime-voice invite. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brRTDTAcceptInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_rtdt_accept", 0, 0, in.RV, "denied", err.Error())
				return nil, err
			}
			rv, err := rpc.ParseShortIDHex(in.RV)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdRTDTAccept(ctx, rv, in.Inviter, in.AsPublisher); err != nil {
				recordSpend(a, "br_rtdt_accept", 0, 0, in.RV, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_rtdt_accept", 0, 0, in.RV, "ok", "")
			return map[string]any{"rv": in.RV, "accepted": true}, nil
		}),
	agentTool("bisonrelay", "br_rtdt_join",
		"Join the live audio of a Bison Relay realtime-voice session. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brRTDTRVInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_rtdt_join", 0, 0, in.RV, "denied", err.Error())
				return nil, err
			}
			rv, err := rpc.ParseShortIDHex(in.RV)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdRTDTJoin(ctx, rv); err != nil {
				recordSpend(a, "br_rtdt_join", 0, 0, in.RV, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_rtdt_join", 0, 0, in.RV, "ok", "")
			return map[string]any{"rv": in.RV, "joined": true}, nil
		}),
	agentTool("bisonrelay", "br_rtdt_leave",
		"Leave a Bison Relay realtime-voice session. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brRTDTRVInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_rtdt_leave", 0, 0, in.RV, "denied", err.Error())
				return nil, err
			}
			rv, err := rpc.ParseShortIDHex(in.RV)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdRTDTLeave(ctx, rv); err != nil {
				recordSpend(a, "br_rtdt_leave", 0, 0, in.RV, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_rtdt_leave", 0, 0, in.RV, "ok", "")
			return map[string]any{"rv": in.RV, "left": true}, nil
		}),
	agentTool("bisonrelay", "br_rtdt_dissolve",
		"Dissolve a Bison Relay realtime-voice session you own. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brRTDTRVInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_rtdt_dissolve", 0, 0, in.RV, "denied", err.Error())
				return nil, err
			}
			rv, err := rpc.ParseShortIDHex(in.RV)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdRTDTDissolve(ctx, rv); err != nil {
				recordSpend(a, "br_rtdt_dissolve", 0, 0, in.RV, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_rtdt_dissolve", 0, 0, in.RV, "ok", "")
			return map[string]any{"rv": in.RV, "dissolved": true}, nil
		}),
	agentTool("bisonrelay", "br_rtdt_chat",
		"Send a text message into a live Bison Relay realtime-voice session. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brRTDTChatInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_rtdt_chat", 0, 0, in.RV, "denied", err.Error())
				return nil, err
			}
			rv, err := rpc.ParseShortIDHex(in.RV)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdRTDTChat(ctx, rv, in.Message); err != nil {
				recordSpend(a, "br_rtdt_chat", 0, 0, in.RV, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_rtdt_chat", 0, 0, in.RV, "ok", "")
			return map[string]any{"rv": in.RV, "sent": true}, nil
		}),
	agentTool("bisonrelay", "br_gc_kick",
		"Kick a member from a Bison Relay group chat (admin action). Requires a grant with Bison Relay group admin enabled.",
		func(ctx context.Context, a *agent, in brGCKickInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBRAdmin, time.Now()); err != nil {
				recordSpend(a, "br_gc_kick", 0, 0, in.GCID, "denied", err.Error())
				return nil, err
			}
			gcid, err := rpc.ParseShortIDHex(in.GCID)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdGCKick(ctx, gcid, in.UID, in.Reason); err != nil {
				recordSpend(a, "br_gc_kick", 0, 0, in.GCID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_gc_kick", 0, 0, in.GCID, "ok", in.UID)
			return map[string]any{"gcid": in.GCID, "uid": in.UID, "kicked": true}, nil
		}),
	agentTool("bisonrelay", "br_gc_kill",
		"Dissolve a Bison Relay group chat (owner only). Requires a grant with Bison Relay group admin enabled.",
		func(ctx context.Context, a *agent, in brGCKillInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBRAdmin, time.Now()); err != nil {
				recordSpend(a, "br_gc_kill", 0, 0, in.GCID, "denied", err.Error())
				return nil, err
			}
			gcid, err := rpc.ParseShortIDHex(in.GCID)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdGCKill(ctx, gcid, in.Reason); err != nil {
				recordSpend(a, "br_gc_kill", 0, 0, in.GCID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_gc_kill", 0, 0, in.GCID, "ok", "")
			return map[string]any{"gcid": in.GCID, "killed": true}, nil
		}),
	agentTool("bisonrelay", "br_gc_block",
		"Client-side block a member of a Bison Relay group chat. Requires a grant with Bison Relay group admin enabled.",
		func(ctx context.Context, a *agent, in brGCMemberInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBRAdmin, time.Now()); err != nil {
				recordSpend(a, "br_gc_block", 0, 0, in.GCID, "denied", err.Error())
				return nil, err
			}
			gcid, err := rpc.ParseShortIDHex(in.GCID)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdGCBlock(ctx, gcid, in.UID); err != nil {
				recordSpend(a, "br_gc_block", 0, 0, in.GCID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_gc_block", 0, 0, in.GCID, "ok", in.UID)
			return map[string]any{"gcid": in.GCID, "uid": in.UID, "blocked": true}, nil
		}),
	agentTool("bisonrelay", "br_gc_unblock",
		"Remove a member from a Bison Relay group chat's local block list. Requires a grant with Bison Relay group admin enabled.",
		func(ctx context.Context, a *agent, in brGCMemberInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBRAdmin, time.Now()); err != nil {
				recordSpend(a, "br_gc_unblock", 0, 0, in.GCID, "denied", err.Error())
				return nil, err
			}
			gcid, err := rpc.ParseShortIDHex(in.GCID)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdGCUnblock(ctx, gcid, in.UID); err != nil {
				recordSpend(a, "br_gc_unblock", 0, 0, in.GCID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_gc_unblock", 0, 0, in.GCID, "ok", in.UID)
			return map[string]any{"gcid": in.GCID, "uid": in.UID, "unblocked": true}, nil
		}),
	agentTool("bisonrelay", "br_gc_admins",
		"Replace the extra-admins list of a Bison Relay group chat (v1+ groups). Send the complete desired list; it overwrites the current one. Requires a grant with Bison Relay group admin enabled.",
		func(ctx context.Context, a *agent, in brGCAdminsInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBRAdmin, time.Now()); err != nil {
				recordSpend(a, "br_gc_admins", 0, 0, in.GCID, "denied", err.Error())
				return nil, err
			}
			admins := in.ExtraAdmins
			if admins == nil {
				admins = []string{}
			}
			gcid, err := rpc.ParseShortIDHex(in.GCID)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdGCModifyAdmins(ctx, gcid, admins, in.Reason); err != nil {
				recordSpend(a, "br_gc_admins", 0, 0, in.GCID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_gc_admins", 0, 0, in.GCID, "ok", "")
			return map[string]any{"gcid": in.GCID, "extraAdmins": admins, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_gc_owner",
		"Transfer ownership of a Bison Relay group chat to another member. Requires a grant with Bison Relay group admin enabled.",
		func(ctx context.Context, a *agent, in brGCOwnerInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBRAdmin, time.Now()); err != nil {
				recordSpend(a, "br_gc_owner", 0, 0, in.GCID, "denied", err.Error())
				return nil, err
			}
			gcid, err := rpc.ParseShortIDHex(in.GCID)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdGCModifyOwner(ctx, gcid, in.NewOwner, in.Reason); err != nil {
				recordSpend(a, "br_gc_owner", 0, 0, in.GCID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_gc_owner", 0, 0, in.GCID, "ok", in.NewOwner)
			return map[string]any{"gcid": in.GCID, "newOwner": in.NewOwner, "ok": true}, nil
		}),
	agentTool("bisonrelay", "br_rtdt_kick",
		"Kick a live peer from a Bison Relay realtime-voice session, optionally banning them for a number of seconds. Requires a grant with Bison Relay group admin enabled.",
		func(ctx context.Context, a *agent, in brRTDTKickInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBRAdmin, time.Now()); err != nil {
				recordSpend(a, "br_rtdt_kick", 0, 0, in.RV, "denied", err.Error())
				return nil, err
			}
			rv, err := rpc.ParseShortIDHex(in.RV)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdRTDTKick(ctx, rv, in.PeerID, in.BanSeconds); err != nil {
				recordSpend(a, "br_rtdt_kick", 0, 0, in.RV, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_rtdt_kick", 0, 0, in.RV, "ok", fmt.Sprintf("peer %d", in.PeerID))
			return map[string]any{"rv": in.RV, "peerId": in.PeerID, "kicked": true}, nil
		}),
	agentTool("bisonrelay", "br_rtdt_remove",
		"Remove a member from a Bison Relay realtime-voice session's metadata. Requires a grant with Bison Relay group admin enabled.",
		func(ctx context.Context, a *agent, in brRTDTRemoveInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBRAdmin, time.Now()); err != nil {
				recordSpend(a, "br_rtdt_remove", 0, 0, in.RV, "denied", err.Error())
				return nil, err
			}
			rv, err := rpc.ParseShortIDHex(in.RV)
			if err != nil {
				return nil, err
			}
			if err := rpc.BrclientdRTDTRemove(ctx, rv, in.UID, in.Reason); err != nil {
				recordSpend(a, "br_rtdt_remove", 0, 0, in.RV, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_rtdt_remove", 0, 0, in.RV, "ok", in.UID)
			return map[string]any{"rv": in.RV, "uid": in.UID, "removed": true}, nil
		}),

	// Remote pages + storefront (buyer side). Browsing is read-only; outward,
	// state-changing submissions are gated on scopeBR like br_send_message.
	readTool("bisonrelay", "br_page_fetch",
		"Visit and navigate a Bison Relay page or storefront hosted by a remote contact. Defaults to the contact's root page (index.md); follow links by passing their path segments and reuse sessionId to stay in one session. Returns the page markdown plus parsed segments (form fields and download embeds) and session/page ids. Read-only browsing.",
		func(ctx context.Context, in brPageFetchInput) (any, error) {
			if err := rejectMutatingPagePath(in.Path); err != nil {
				return nil, err
			}
			return brPageFetch(ctx, in.UID, in.Path, in.SessionID, in.ParentPage, nil)
		}),
	readTool("bisonrelay", "br_shop_cart",
		"View your current cart at a remote Bison Relay simplestore merchant. Read-only.",
		func(ctx context.Context, in brShopUIDInput) (any, error) {
			return brPageFetch(ctx, in.UID, []string{"cart"}, 0, 0, nil)
		}),
	readTool("bisonrelay", "br_shop_orders",
		"List your orders at a remote Bison Relay simplestore merchant. Read-only.",
		func(ctx context.Context, in brShopUIDInput) (any, error) {
			return brPageFetch(ctx, in.UID, []string{"orders"}, 0, 0, nil)
		}),
	readTool("bisonrelay", "br_shop_order",
		"View one of your orders at a remote Bison Relay simplestore merchant (status, items, invoice). Read-only.",
		func(ctx context.Context, in brShopOrderInput) (any, error) {
			return brPageFetch(ctx, in.UID, []string{"order", strconv.FormatUint(in.ID, 10)}, 0, 0, nil)
		}),
	readTool("bisonrelay", "br_downloads",
		"List Bison Relay file transfers (in-flight and completed). A purchased digital download lands here after the merchant sends it on payment.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			body, err := rpc.BrclientdListDownloads(ctx)
			if err != nil {
				return nil, err
			}
			return decodeBRResult(body), nil
		}),
	agentTool("bisonrelay", "br_page_submit",
		"Submit a form on a Bison Relay page hosted by a remote contact (the outward, state-changing counterpart of br_page_fetch), e.g. a storefront add-to-cart or place-order. Provide the form's action path and a JSON data payload matching its fields. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brPageSubmitInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_page_submit", 0, 0, in.UID, "denied", err.Error())
				return nil, err
			}
			var data json.RawMessage
			if in.Data != nil {
				b, err := json.Marshal(in.Data)
				if err != nil {
					return nil, fmt.Errorf("encode data: %w", err)
				}
				data = b
			}
			res, err := brPageFetch(ctx, in.UID, in.Path, in.SessionID, 0, data, in.FieldTypes)
			if err != nil {
				recordSpend(a, "br_page_submit", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_page_submit", 0, 0, in.UID, "ok", strings.Join(in.Path, "/"))
			return res, nil
		}),
	agentTool("bisonrelay", "br_shop_add_to_cart",
		"Add a product to your cart at a remote Bison Relay simplestore merchant. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brShopAddToCartInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_shop_add_to_cart", 0, 0, in.UID, "denied", err.Error())
				return nil, err
			}
			qty := in.Quantity
			if qty == 0 {
				qty = 1
			}
			data, _ := json.Marshal(map[string]any{"sku": in.SKU, "qty": qty})
			res, err := brPageFetch(ctx, in.UID, []string{"addToCart"}, 0, 0, data)
			if err != nil {
				recordSpend(a, "br_shop_add_to_cart", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_shop_add_to_cart", 0, 0, in.UID, "ok", fmt.Sprintf("%s x%d", in.SKU, qty))
			return res, nil
		}),
	agentTool("bisonrelay", "br_shop_clear_cart",
		"Empty your cart at a remote Bison Relay simplestore merchant. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brShopUIDInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_shop_clear_cart", 0, 0, in.UID, "denied", err.Error())
				return nil, err
			}
			res, err := brPageFetch(ctx, in.UID, []string{"clearCart"}, 0, 0, nil)
			if err != nil {
				recordSpend(a, "br_shop_clear_cart", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_shop_clear_cart", 0, 0, in.UID, "ok", "")
			return res, nil
		}),
	agentTool("bisonrelay", "br_shop_place_order",
		"Place an order for the items in your cart at a remote Bison Relay simplestore merchant. Returns the order page, and (when present) the extracted Lightning invoice to pay with ln_pay. Provide a shipping address only if an ordered product requires shipping. This creates an order but does not itself move funds. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brShopPlaceOrderInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_shop_place_order", 0, 0, in.UID, "denied", err.Error())
				return nil, err
			}
			var data json.RawMessage
			if in.Shipping != nil {
				if b, err := json.Marshal(in.Shipping); err == nil {
					data = b
				}
			}
			res, err := brPageFetch(ctx, in.UID, []string{"placeOrder"}, 0, 0, data)
			if err != nil {
				recordSpend(a, "br_shop_place_order", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			if m, ok := res.(map[string]any); ok {
				if md, _ := m["markdown"].(string); md != "" {
					if inv := extractLNInvoice(md); inv != "" {
						m["invoice"] = inv
						m["pay_type"] = "ln"
					}
				}
			}
			recordSpend(a, "br_shop_place_order", 0, 0, in.UID, "ok", "")
			return res, nil
		}),
	agentTool("bisonrelay", "br_shop_order_comment",
		"Add a comment to one of your orders at a remote Bison Relay simplestore merchant. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brShopOrderCommentInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_shop_order_comment", 0, 0, in.UID, "denied", err.Error())
				return nil, err
			}
			// simplestore's orderaddcomment expects the comment as a bare JSON string.
			data, _ := json.Marshal(in.Comment)
			res, err := brPageFetch(ctx, in.UID, []string{"orderaddcomment", strconv.FormatUint(in.ID, 10)}, 0, 0, data)
			if err != nil {
				recordSpend(a, "br_shop_order_comment", 0, 0, in.UID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_shop_order_comment", 0, 0, in.UID, "ok", fmt.Sprintf("order %d", in.ID))
			return res, nil
		}),
	agentTool("bisonrelay", "br_content_get",
		"Start downloading a shared file advertised by a Bison Relay page download embed (e.g. a purchased digital product). maxCostAtoms defaults to 0 (free files only). A paid download spends over Lightning, so it additionally requires the Lightning scope and reserves the whole ceiling against the per-transaction and daily caps. Track progress with br_downloads. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brContentGetInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_content_get", 0, 0, in.UID, "denied", err.Error())
				return nil, err
			}
			// maxCostAtoms is the ceiling brclientd auto-approves the whole
			// file against, so it bounds what dcrlnd pays the peer. Reserve it
			// like any other Lightning spend; the download is fire-and-forget,
			// so there is no settlement to refund the unused part from.
			if in.MaxCostAtoms > math.MaxInt64 {
				return nil, fmt.Errorf("maxCostAtoms is out of range")
			}
			capAtoms := int64(in.MaxCostAtoms)
			capDCR := dcrutil.Amount(capAtoms).ToCoin()
			if capAtoms > 0 {
				action := fmt.Sprintf("pay up to %s to download a Bison Relay file from %s", dcrAmountStr(capAtoms), in.UID)
				if err := grants.authorizeSpendScoped(ctx, a.id, scopeLightning, capAtoms, action, time.Now()); err != nil {
					if tripwire(a.id, err) {
						recordSpend(a, "br_content_get", 0, capDCR, in.UID, "blocked", "spend-limit violation: grant revoked and token blocked")
					} else {
						recordSpend(a, "br_content_get", 0, capDCR, in.UID, "denied", err.Error())
					}
					return nil, err
				}
			}
			if err := rpc.BrclientdContentGet(ctx, in.UID, in.FID, in.MaxCostAtoms); err != nil {
				grants.refund(a.id, capAtoms)
				recordSpend(a, "br_content_get", 0, capDCR, in.UID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_content_get", 0, capDCR, in.UID, "ok", in.FID)
			return map[string]any{"uid": in.UID, "fid": in.FID, "started": true}, nil
		}),
	agentTool("bisonrelay", "br_download_cancel",
		"Cancel an in-flight Bison Relay file download by fid. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brFidInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_download_cancel", 0, 0, in.FID, "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdCancelDownload(ctx, in.FID); err != nil {
				recordSpend(a, "br_download_cancel", 0, 0, in.FID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_download_cancel", 0, 0, in.FID, "ok", "")
			return map[string]any{"fid": in.FID, "cancelled": true}, nil
		}),
	agentTool("bisonrelay", "br_download_delete",
		"Delete a completed or failed Bison Relay download from disk by fid (optionally uid to disambiguate the same file from multiple peers). To fetch it again later, call br_content_get. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brDownloadDeleteInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_download_delete", 0, 0, in.FID, "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdDeleteDownload(ctx, in.FID, in.UID); err != nil {
				recordSpend(a, "br_download_delete", 0, 0, in.FID, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_download_delete", 0, 0, in.FID, "ok", "")
			return map[string]any{"fid": in.FID, "deleted": true}, nil
		}),
	agentTool("bisonrelay", "br_notification_delete",
		"Delete a single Bison Relay notification-bell entry by id. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brNotificationDeleteInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_notification_delete", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdDeleteNotification(ctx, in.ID); err != nil {
				recordSpend(a, "br_notification_delete", 0, 0, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_notification_delete", 0, 0, "", "ok", "")
			return map[string]any{"id": in.ID, "deleted": true}, nil
		}),
	agentTool("bisonrelay", "br_notifications_clear",
		"Clear all Bison Relay notification-bell entries. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, _ emptyInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_notifications_clear", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdClearNotifications(ctx); err != nil {
				recordSpend(a, "br_notifications_clear", 0, 0, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_notifications_clear", 0, 0, "", "ok", "")
			return map[string]any{"cleared": true}, nil
		}),
	readTool("bisonrelay", "br_store_files",
		"List the media files in the Bison Relay storefront directory (images and other assets referenced by products and templates).",
		func(ctx context.Context, _ emptyInput) (any, error) {
			body, err := rpc.BrclientdListStoreFiles(ctx)
			if err != nil {
				return nil, err
			}
			return decodeBRResult(body), nil
		}),
	readTool("bisonrelay", "br_store_file_get",
		"Fetch one Bison Relay storefront media file by path. Returns its content type and base64 bytes.",
		func(ctx context.Context, in brStorePathInput) (any, error) {
			data, contentType, err := rpc.BrclientdGetStoreFile(ctx, in.Path)
			if err != nil {
				return nil, err
			}
			return map[string]any{"path": in.Path, "contentType": contentType, "dataB64": base64.StdEncoding.EncodeToString(data)}, nil
		}),
	agentTool("bisonrelay", "br_store_file_delete",
		"Delete a media file from the Bison Relay storefront directory by path. Requires a grant with Bison Relay write enabled.",
		func(ctx context.Context, a *agent, in brStorePathInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeBR, time.Now()); err != nil {
				recordSpend(a, "br_store_file_delete", 0, 0, in.Path, "denied", err.Error())
				return nil, err
			}
			if err := rpc.BrclientdDeleteStoreFile(ctx, in.Path); err != nil {
				recordSpend(a, "br_store_file_delete", 0, 0, in.Path, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "br_store_file_delete", 0, 0, in.Path, "ok", "")
			return map[string]any{"path": in.Path, "deleted": true}, nil
		}),
}

// decodeBRResult unmarshals a brclientd JSON response into a generic value so
// the MCP layer returns structured data rather than an opaque byte string. On a
// decode failure it falls back to the raw text.
func decodeBRResult(body []byte) any {
	if len(body) == 0 {
		return map[string]any{"ok": true}
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return map[string]any{"raw": string(body)}
	}
	return v
}

// resolveOutboxPath validates a caller-supplied relative path and returns the
// absolute path under the agent outbox dir (services.AgentOutboxDir). It is the
// sandbox guard for br_file_send_path: rejects absolute paths, "..", and any
// path that would resolve outside the outbox, so a path-based send can never
// reach arbitrary host files.
func resolveOutboxPath(rel string) (string, error) {
	if !safeBRName(rel) {
		return "", fmt.Errorf("invalid path")
	}
	root := filepath.Clean(services.AgentOutboxDir())
	candidate := filepath.Clean(filepath.Join(root, rel))
	if candidate != root && !strings.HasPrefix(candidate, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path outside outbox")
	}
	return candidate, nil
}

// buildImageEmbedBody reads an image from the agent outbox (path-sandboxed via
// resolveOutboxPath), enforces the inline-embed size cap, and returns a chat
// message body with the image carried inline as a bruig --embed[...]-- tag. The
// agent supplies only the relative path and an optional caption; the file read
// and base64 encoding happen here, never in the agent's context. Mirrors the
// dashboard's BisonrelayPMHandler embed assembly.
func buildImageEmbedBody(relPath, message, mimeOverride string) (string, error) {
	candidate, err := resolveOutboxPath(relPath)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(candidate)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", fmt.Errorf("path is a directory")
	}
	if fi.Size() > services.MaxInlineEmbedBytes {
		return "", fmt.Errorf("image is %d bytes, over the %d-byte inline embed cap; use br_file_send_path to send it as a file transfer instead", fi.Size(), services.MaxInlineEmbedBytes)
	}
	data, err := os.ReadFile(candidate)
	if err != nil {
		return "", err
	}
	mimeType := strings.TrimSpace(mimeOverride)
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(candidate))
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	tag := services.BuildEmbedTag(filepath.Base(candidate), mimeType, base64.StdEncoding.EncodeToString(data))
	if message == "" {
		return tag, nil
	}
	return message + "\n" + tag, nil
}

// chatEmbedRE matches the localfilename form PM history uses for a received
// chat embed: embeds/<16-hex ShortLogID>/<file>. The file segment has no
// separators, so a match names exactly one file in one peer's embed folder.
var chatEmbedRE = regexp.MustCompile(`^embeds/([0-9a-f]{16})/([A-Za-z0-9._-]+)$`)

// pageImageExtRE is the raster set a page may embed (and br_embed_get may
// serve); mirrors brclientd's pageAssetNameRE extension set.
var pageImageExtRE = regexp.MustCompile(`\.(?:jpg|jpeg|jfif|png|gif|webp)$`)

// maxEmbedGetBytes bounds br_embed_get reads; BR transport cannot deliver
// larger embeds and the cap also protects the agent's context window.
const maxEmbedGetBytes = 4 << 20

// readChatEmbed reads a received chat embed strictly inside brclientd's
// embeds store (mounted read-only). The regex is the first gate; os.Root
// makes traversal and symlink escape impossible even past it.
func readChatEmbed(ctx context.Context, localfilename string) ([]byte, error) {
	m := chatEmbedRE.FindStringSubmatch(localfilename)
	if m == nil || strings.Contains(localfilename, "..") {
		return nil, fmt.Errorf("localfilename must be embeds/<uid16>/<file>")
	}
	network, _ := services.CurrentNetwork(ctx)
	if network == "" {
		network = "mainnet"
	}
	root, err := os.OpenRoot(services.BrclientdEmbedsDir(network))
	if err != nil {
		return nil, fmt.Errorf("embeds store unavailable: %w", err)
	}
	defer root.Close()
	rel := m[1] + "/" + m[2]
	fi, err := root.Lstat(rel)
	if err != nil {
		return nil, fmt.Errorf("embed not found")
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("embed is not a regular file")
	}
	if fi.Size() > maxEmbedGetBytes {
		return nil, fmt.Errorf("embed exceeds %d bytes", maxEmbedGetBytes)
	}
	f, err := root.Open(rel)
	if err != nil {
		return nil, fmt.Errorf("open embed: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxEmbedGetBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read embed: %w", err)
	}
	if len(data) > maxEmbedGetBytes {
		return nil, fmt.Errorf("embed exceeds %d bytes", maxEmbedGetBytes)
	}
	return data, nil
}

// filteredPMHistory scans PM history newest-first (page 0 is the newest) and
// returns the entries matching the since/onlyEmbeds filters. The scan is
// bounded so a filter cannot walk an arbitrarily long history; sparse pages
// (envelope frames are dropped server-side after pagination) are not treated
// as the end of history.
func filteredPMHistory(ctx context.Context, in brPmHistoryInput) (any, error) {
	const maxPages = 10
	pageSize := brPageSize(in.PageSize)
	out := make([]map[string]any, 0)
	pages := 0
	for page := 0; page < maxPages; page++ {
		raw, err := rpc.BrclientdHistoryPM(ctx, in.UID, page, pageSize)
		if err != nil {
			return nil, err
		}
		var h struct {
			Entries []map[string]any `json:"entries"`
		}
		if err := json.Unmarshal(raw, &h); err != nil {
			return nil, fmt.Errorf("decode history: %w", err)
		}
		pages++
		if len(h.Entries) == 0 {
			break
		}
		olderSeen := false
		for _, e := range h.Entries {
			ts, _ := e["timestamp"].(float64)
			if in.Since > 0 && int64(ts) < in.Since {
				olderSeen = true
				continue
			}
			if in.OnlyEmbeds {
				msg, _ := e["message"].(string)
				if !strings.Contains(msg, "--embed[") {
					continue
				}
			}
			out = append(out, e)
		}
		if olderSeen {
			break
		}
	}
	return map[string]any{
		"uid":          in.UID,
		"entries":      out,
		"pagesScanned": pages,
		"since":        in.Since,
		"onlyEmbeds":   in.OnlyEmbeds,
	}, nil
}

// safeBRName mirrors the dashboard handlers' safeBRPath guard for page,
// template, and store-file names handed to brclientd: brclientd owns the real
// containment, this is a defense-in-depth check that rejects empty names,
// absolute paths, backslashes, NUL, and any ".." segment.
func safeBRName(p string) bool {
	if p == "" || len(p) > 255 {
		return false
	}
	if strings.ContainsRune(p, 0) || strings.ContainsRune(p, '\\') || strings.HasPrefix(p, "/") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// brPageFetch fetches a remote Bison Relay page/resource and decorates the reply
// with the parsed markdown segments (form fields, download embeds), mirroring the
// dashboard's BisonrelayPagesFetchHandler. data is nil for a plain navigation
// GET, or a JSON form payload for a submission. The returned map is the
// structured tool result.
// mutatingPagePaths are the simplestore routes that change state on the remote
// merchant. They reach the same primitive as browsing, so a read-only fetch has
// to refuse them explicitly or it becomes an ungated write.
var mutatingPagePaths = map[string]bool{
	"placeorder":      true,
	"addtocart":       true,
	"clearcart":       true,
	"orderaddcomment": true,
	"admin":           true,
}

// rejectMutatingPagePath refuses a browsing path that would act rather than
// read. Only the first segment is dispatched on by the simplestore handler.
func rejectMutatingPagePath(path []string) error {
	if len(path) == 0 {
		return nil
	}
	if head := strings.ToLower(strings.TrimSpace(path[0])); mutatingPagePaths[head] {
		return fmt.Errorf("%q changes state on the merchant; use the matching br_shop_* tool, which requires a Bison Relay write grant", head)
	}
	return nil
}

func brPageFetch(ctx context.Context, uid string, path []string, sessionID, parentPage uint64, data json.RawMessage, fieldTypes ...map[string]string) (any, error) {
	if strings.TrimSpace(uid) == "" {
		return nil, fmt.Errorf("uid is required")
	}
	if len(path) == 0 {
		path = []string{"index.md"}
	}
	body := map[string]any{
		"uid":         uid,
		"path":        path,
		"session_id":  sessionID,
		"parent_page": parentPage,
	}
	if len(data) > 0 {
		body["data"] = data
	}
	if len(fieldTypes) > 0 && len(fieldTypes[0]) > 0 {
		body["field_types"] = fieldTypes[0]
	}
	raw, err := rpc.BrclientdPagesFetch(ctx, body)
	if err != nil {
		return nil, err
	}
	var fetched struct {
		SessionID  uint64            `json:"session_id"`
		PageID     uint64            `json:"page_id"`
		ParentPage uint64            `json:"parent_page"`
		Status     uint16            `json:"status"`
		Meta       map[string]string `json:"meta"`
		Markdown   string            `json:"markdown"`
	}
	if err := json.Unmarshal(raw, &fetched); err != nil {
		return nil, fmt.Errorf("decode page reply: %w", err)
	}
	return map[string]any{
		"uid":         uid,
		"path":        path,
		"session_id":  fetched.SessionID,
		"page_id":     fetched.PageID,
		"parent_page": fetched.ParentPage,
		"status":      fetched.Status,
		"meta":        fetched.Meta,
		"markdown":    fetched.Markdown,
		"segments":    leanPageSegments(services.SplitAndRenderBRPage(fetched.Markdown)),
	}, nil
}

// leanPageSegments drops the heavy rendered HTML and inline base64 image bytes
// from page segments: an agent reads the markdown, and keeping those would risk
// exceeding the MCP result size. The useful structured bits stay: form fields
// (to submit) and file-download embeds (fid/cost/filename, for br_content_get).
func leanPageSegments(segs []services.BRPageSegment) []services.BRPageSegment {
	for i := range segs {
		segs[i].HTML = ""
		segs[i].DataB64 = ""
	}
	return segs
}

// lnInvoiceRE matches a bolt11 Lightning invoice, with or without simplestore's
// "lnpay://" URL prefix, across dcrlnd's mainnet/testnet/simnet/regnet HRPs
// (lndcr/lntdcr/lnsdcr/lnrdcr).
var lnInvoiceRE = regexp.MustCompile(`(?i)(?:lnpay://)?\b(ln[tsr]?dcr[0-9][a-z0-9]+)`)

// extractLNInvoice pulls a bolt11 invoice out of a place-order reply's markdown,
// stripping the "lnpay://" scheme so the result hands straight to ln_pay. Returns
// "" when the page carries no LN invoice (e.g. an on-chain or custom template).
func extractLNInvoice(markdown string) string {
	if m := lnInvoiceRE.FindStringSubmatch(markdown); m != nil {
		return m[1]
	}
	return ""
}

func containsSlash(s string) bool { return strings.ContainsRune(s, '/') }
