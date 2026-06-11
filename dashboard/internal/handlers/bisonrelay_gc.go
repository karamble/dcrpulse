// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"dcrpulse/internal/rpc"
)

// BisonrelayGCListHandler proxies brclientd's GET /gc.
func BisonrelayGCListHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdGCList(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayGCCreateHandler creates a new GC.
func BisonrelayGCCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdGCCreate(r.Context(), req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayGCInvitesListHandler lists pending GC invites for the local user.
func BisonrelayGCInvitesListHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdGCInvitesList(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayGCInvitesAcceptHandler accepts an invite by IID.
func BisonrelayGCInvitesAcceptHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IID uint64 `json:"iid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.IID == 0 {
		http.Error(w, "iid is required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdGCInvitesAccept(r.Context(), req.IID); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayGCDetailHandler returns the full GC record including members + blocklist.
func BisonrelayGCDetailHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	body, err := rpc.BrclientdGCDetail(r.Context(), gcid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayGCInviteHandler invites a contact to a GC.
func BisonrelayGCInviteHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	var req struct {
		UID string `json:"uid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdGCInvite(r.Context(), gcid, req.UID); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayGCMessageHandler sends a GC message. JSON body shape mirrors
// BisonrelayPMHandler: {msg, embed?: {name, mime, data_b64}}. Embed is
// rendered into the bruig --embed[...]-- tag with the same builder PMs use.
// Returns {body: "<synthesised wire body>"} so the caller can echo it
// optimistically.
func BisonrelayGCMessageHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	var req struct {
		Msg   string `json:"msg"`
		Mode  int    `json:"mode"`
		Embed *struct {
			Name    string `json:"name"`
			Mime    string `json:"mime"`
			DataB64 string `json:"data_b64"`
		} `json:"embed,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Msg = strings.TrimSpace(req.Msg)
	if req.Embed == nil && req.Msg == "" {
		http.Error(w, "msg or embed is required", http.StatusBadRequest)
		return
	}

	body := req.Msg
	if req.Embed != nil {
		decoded, err := base64.StdEncoding.DecodeString(req.Embed.DataB64)
		if err != nil {
			http.Error(w, "embed data_b64: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(decoded) > maxInlineEmbedBytes {
			http.Error(w, "embed exceeds inline size cap", http.StatusRequestEntityTooLarge)
			return
		}
		tag := buildEmbedTag(req.Embed.Name, req.Embed.Mime, req.Embed.DataB64)
		if body == "" {
			body = tag
		} else {
			body = body + "\n" + tag
		}
	}

	if err := rpc.BrclientdGCMessage(r.Context(), gcid, body, req.Mode); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"body": body})
}

// BisonrelayGCHistoryHandler paginates GC message history.
func BisonrelayGCHistoryHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	body, err := rpc.BrclientdGCHistory(r.Context(), gcid, page, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayGCPartHandler leaves a GC (non-owner action).
func BisonrelayGCPartHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := rpc.BrclientdGCPart(r.Context(), gcid, req.Reason); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayGCKillHandler dissolves a GC (owner-only).
func BisonrelayGCKillHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := rpc.BrclientdGCKill(r.Context(), gcid, req.Reason); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayGCKickHandler kicks a member (admin action).
func BisonrelayGCKickHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	var req struct {
		UID    string `json:"uid"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdGCKick(r.Context(), gcid, req.UID, req.Reason); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayGCBlockHandler client-side blocks a member.
func BisonrelayGCBlockHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	var req struct {
		UID string `json:"uid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdGCBlock(r.Context(), gcid, req.UID); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayGCUnblockHandler removes a member from the local block list.
func BisonrelayGCUnblockHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	var req struct {
		UID string `json:"uid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdGCUnblock(r.Context(), gcid, req.UID); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayGCAdminsHandler replaces the ExtraAdmins list (v1+ only).
func BisonrelayGCAdminsHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	var req struct {
		ExtraAdmins []string `json:"extra_admins"`
		Reason      string   `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdGCModifyAdmins(r.Context(), gcid, req.ExtraAdmins, req.Reason); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayGCOwnerHandler swaps the GC owner (Members[0]).
func BisonrelayGCOwnerHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	var req struct {
		NewOwner string `json:"new_owner"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdGCModifyOwner(r.Context(), gcid, req.NewOwner, req.Reason); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayGCUpgradeHandler bumps the GC protocol version (one-way).
func BisonrelayGCUpgradeHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	var req struct {
		NewVersion uint8 `json:"new_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdGCUpgrade(r.Context(), gcid, req.NewVersion); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayGCAliasHandler sets the local alias for a GC (DB-only).
func BisonrelayGCAliasHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	var req struct {
		Alias string `json:"alias"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdGCAlias(r.Context(), gcid, req.Alias); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayGCResendListHandler resends the GC member list to one or all members.
func BisonrelayGCResendListHandler(w http.ResponseWriter, r *http.Request) {
	gcid := mux.Vars(r)["gcid"]
	var req struct {
		UID string `json:"uid"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := rpc.BrclientdGCResendList(r.Context(), gcid, req.UID); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
