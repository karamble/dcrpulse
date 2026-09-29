// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"dcrpulse/internal/rpc"
)

// BisonrelayGCListHandler proxies brclientd's GET /gc.
func BisonrelayGCListHandler(w http.ResponseWriter, r *http.Request) {
	brProxyJSON(w, func() (json.RawMessage, error) { return rpc.BrclientdGCList(r.Context()) })
}

// BisonrelayGCCreateHandler creates a new GC.
func BisonrelayGCCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	brProxyJSON(w, func() (json.RawMessage, error) { return rpc.BrclientdGCCreate(r.Context(), req.Name) })
}

// BisonrelayGCInvitesListHandler lists pending GC invites for the local user.
func BisonrelayGCInvitesListHandler(w http.ResponseWriter, r *http.Request) {
	brProxyJSON(w, func() (json.RawMessage, error) { return rpc.BrclientdGCInvitesList(r.Context()) })
}

// BisonrelayGCInvitesAcceptHandler accepts an invite by IID.
func BisonrelayGCInvitesAcceptHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IID uint64 `json:"iid"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.IID == 0 {
		http.Error(w, "iid is required", http.StatusBadRequest)
		return
	}
	brDo204(w, func() error { return rpc.BrclientdGCInvitesAccept(r.Context(), req.IID) })
}

// BisonrelayGCDetailHandler returns the full GC record including members + blocklist.
func BisonrelayGCDetailHandler(w http.ResponseWriter, r *http.Request) {
	gcid, ok := brPathID(w, mux.Vars(r)["gcid"], "gcid")
	if !ok {
		return
	}
	brProxyJSON(w, func() (json.RawMessage, error) { return rpc.BrclientdGCDetail(r.Context(), gcid) })
}

// BisonrelayGCInviteHandler invites a contact to a GC.
func BisonrelayGCInviteHandler(w http.ResponseWriter, r *http.Request) {
	gcMemberAction(w, r, rpc.BrclientdGCInvite)
}

// BisonrelayGCMessageHandler sends a GC message. JSON body shape mirrors
// BisonrelayPMHandler: {msg, embed?: {name, mime, data_b64}}. Embed is
// rendered into the bruig --embed[...]-- tag with the same builder PMs use.
// Returns {body: "<synthesised wire body>"} so the caller can echo it
// optimistically.
func BisonrelayGCMessageHandler(w http.ResponseWriter, r *http.Request) {
	gcid, ok := brPathID(w, mux.Vars(r)["gcid"], "gcid")
	if !ok {
		return
	}
	var req struct {
		Msg   string   `json:"msg"`
		Mode  int      `json:"mode"`
		Embed *brEmbed `json:"embed,omitempty"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	req.Msg = strings.TrimSpace(req.Msg)
	if req.Embed == nil && req.Msg == "" {
		http.Error(w, "msg or embed is required", http.StatusBadRequest)
		return
	}
	body, ok := brMessageBody(w, req.Msg, req.Embed)
	if !ok {
		return
	}
	if err := rpc.BrclientdGCMessage(r.Context(), gcid, body, req.Mode); err != nil {
		brWriteErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"body": body})
}

// BisonrelayGCHistoryHandler paginates GC message history.
func BisonrelayGCHistoryHandler(w http.ResponseWriter, r *http.Request) {
	gcid, ok := brPathID(w, mux.Vars(r)["gcid"], "gcid")
	if !ok {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	brProxyJSON(w, func() (json.RawMessage, error) { return rpc.BrclientdGCHistory(r.Context(), gcid, page, pageSize) })
}

// BisonrelayGCClearHistoryHandler wipes the locally stored scrollback for a GC.
// Local-only and irreversible: the group and its members are untouched and the
// other members keep their own copies.
func BisonrelayGCClearHistoryHandler(w http.ResponseWriter, r *http.Request) {
	brPathAction(w, r, "gcid", rpc.BrclientdGCClearHistory)
}

// BisonrelayGCPartHandler leaves a GC (non-owner action).
func BisonrelayGCPartHandler(w http.ResponseWriter, r *http.Request) {
	gcReasonAction(w, r, rpc.BrclientdGCPart)
}

// gcReasonAction runs one GC action with an optional {reason} body; a missing
// body is deliberately tolerated.
func gcReasonAction(w http.ResponseWriter, r *http.Request, act func(ctx context.Context, gcid rpc.ShortIDHex, reason string) error) {
	gcid, ok := brPathID(w, mux.Vars(r)["gcid"], "gcid")
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	brDo204(w, func() error { return act(r.Context(), gcid, req.Reason) })
}

// BisonrelayGCKillHandler dissolves a GC (owner-only).
func BisonrelayGCKillHandler(w http.ResponseWriter, r *http.Request) {
	gcReasonAction(w, r, rpc.BrclientdGCKill)
}

// BisonrelayGCKickHandler kicks a member (admin action).
func BisonrelayGCKickHandler(w http.ResponseWriter, r *http.Request) {
	brPathJSONAction(w, r, "gcid", func(ctx context.Context, gcid rpc.ShortIDHex, req struct {
		UID    brHexID `json:"uid"`
		Reason string  `json:"reason"`
	}) error {
		return rpc.BrclientdGCKick(ctx, gcid, string(req.UID), req.Reason)
	})
}

// gcMemberAction decodes {uid} and runs one member-scoped GC action.
func gcMemberAction(w http.ResponseWriter, r *http.Request, action func(ctx context.Context, gcid rpc.ShortIDHex, uid string) error) {
	brPathJSONAction(w, r, "gcid", func(ctx context.Context, gcid rpc.ShortIDHex, req struct {
		UID brHexID `json:"uid"`
	}) error {
		return action(ctx, gcid, string(req.UID))
	})
}

// BisonrelayGCBlockHandler client-side blocks a member.
func BisonrelayGCBlockHandler(w http.ResponseWriter, r *http.Request) {
	gcMemberAction(w, r, rpc.BrclientdGCBlock)
}

// BisonrelayGCUnblockHandler removes a member from the local block list.
func BisonrelayGCUnblockHandler(w http.ResponseWriter, r *http.Request) {
	gcMemberAction(w, r, rpc.BrclientdGCUnblock)
}

// BisonrelayGCAdminsHandler replaces the ExtraAdmins list (v1+ only).
func BisonrelayGCAdminsHandler(w http.ResponseWriter, r *http.Request) {
	brPathJSONAction(w, r, "gcid", func(ctx context.Context, gcid rpc.ShortIDHex, req struct {
		ExtraAdmins brHexIDs `json:"extra_admins"`
		Reason      string   `json:"reason"`
	}) error {
		return rpc.BrclientdGCModifyAdmins(ctx, gcid, req.ExtraAdmins, req.Reason)
	})
}

// BisonrelayGCOwnerHandler swaps the GC owner (Members[0]).
func BisonrelayGCOwnerHandler(w http.ResponseWriter, r *http.Request) {
	brPathJSONAction(w, r, "gcid", func(ctx context.Context, gcid rpc.ShortIDHex, req struct {
		NewOwner brHexID `json:"new_owner"`
		Reason   string  `json:"reason"`
	}) error {
		return rpc.BrclientdGCModifyOwner(ctx, gcid, string(req.NewOwner), req.Reason)
	})
}

// BisonrelayGCUpgradeHandler bumps the GC protocol version (one-way).
func BisonrelayGCUpgradeHandler(w http.ResponseWriter, r *http.Request) {
	brPathJSONAction(w, r, "gcid", func(ctx context.Context, gcid rpc.ShortIDHex, req struct {
		NewVersion uint8 `json:"new_version"`
	}) error {
		return rpc.BrclientdGCUpgrade(ctx, gcid, req.NewVersion)
	})
}

// BisonrelayGCAliasHandler sets the local alias for a GC (DB-only).
func BisonrelayGCAliasHandler(w http.ResponseWriter, r *http.Request) {
	brPathJSONAction(w, r, "gcid", func(ctx context.Context, gcid rpc.ShortIDHex, req struct {
		Alias string `json:"alias"`
	}) error {
		return rpc.BrclientdGCAlias(ctx, gcid, req.Alias)
	})
}

// BisonrelayGCResendListHandler resends the GC member list to one or all members.
func BisonrelayGCResendListHandler(w http.ResponseWriter, r *http.Request) {
	gcMemberAction(w, r, rpc.BrclientdGCResendList)
}
