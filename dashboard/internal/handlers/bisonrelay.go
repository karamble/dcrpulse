// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
)

// BisonrelayVersionHandler proxies brclientd's VersionService.Version
// through to the dashboard's HTTP API. Returns the brclientd
// appName / appVersion / goRuntime triple as JSON, or 502 if brclientd
// is unreachable.
func BisonrelayVersionHandler(w http.ResponseWriter, r *http.Request) {
	ver, err := rpc.BrclientdVersion(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ver)
}

// BisonrelayStatusHandler proxies brclientd's /status endpoint, returning
// the current stage, server LN node, and the most recent CheckLNWalletUsable
// error verbatim so the UI can render it.
func BisonrelayStatusHandler(w http.ResponseWriter, r *http.Request) {
	status, err := rpc.BrclientdStatus(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// BisonrelayIdentityHandler returns brclientd's local BR identity payload
// (nick + zkidentity public keys) by proxying ChatService.UserPublicIdentity.
// 502 if brclientd is unreachable or has not yet reached the ready stage.
func BisonrelayIdentityHandler(w http.ResponseWriter, r *http.Request) {
	id, err := rpc.BrclientdUserPublicIdentity(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(id)
}

// BisonrelaySetAvatarHandler proxies brclientd's /avatar. Body: {avatar}
// base64-encoded image bytes; an empty string clears the avatar. BR caps the
// raw size at 200KiB and broadcasts the change to all contacts.
func BisonrelaySetAvatarHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Avatar string `json:"avatar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdSetAvatar(r.Context(), req.Avatar); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayEventsHandler upgrades to WebSocket and streams live PM / KX /
// GCM events from brclientd to the browser. Each frame is a JSON object
// with {type, payload}; payload is the raw event JSON.
func BisonrelayEventsHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: middleware.SameOriginWS}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("BisonrelayEventsHandler upgrade: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	events, unsubscribe := services.Bisonrelay().Subscribe(64)
	defer unsubscribe()

	go func() {
		for {
			if _, _, err := conn.NextReader(); err != nil {
				cancel()
				return
			}
		}
	}()

	pinger := time.NewTicker(30 * time.Second)
	defer pinger.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-pinger.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case evt, ok := <-events:
			if !ok {
				return
			}
			if err := conn.WriteJSON(evt); err != nil {
				return
			}
		}
	}
}

var (
	embedContactRe  = regexp.MustCompile(`^[0-9a-f]{16}$`)
	embedFilenameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	downloadNickRe  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	downloadFileRe  = regexp.MustCompile(`^[A-Za-z0-9._ -]+$`)
	// fid/uid are zkidentity.ShortID hex strings (32 bytes); a cheap first gate
	// before proxying a delete to brclientd.
	downloadIDRe = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
)

// BisonrelayEmbedHandler serves an inline embed file that BR's clientdb has
// already extracted from a PM body and persisted at
// <brclientd-data>/<network>/db/embeds/<contact_short>/<filename>. The
// dashboard locates the brclientd data root via services.BrclientdDataDir
// (env-driven, see [[project-bisonrelay-integration]]).
func BisonrelayEmbedHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	contact := vars["contact"]
	filename := vars["filename"]
	if !embedContactRe.MatchString(contact) || !embedFilenameRe.MatchString(filename) {
		http.NotFound(w, r)
		return
	}

	network, _ := services.CurrentNetwork(r.Context())
	if network == "" {
		network = "mainnet"
	}
	root := filepath.Clean(services.BrclientdEmbedsDir(network))
	candidate := filepath.Clean(filepath.Join(root, contact, filename))
	if !strings.HasPrefix(candidate, root+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, candidate)
}

// BisonrelayFileSendHandler accepts a multipart upload (user + file) from
// the browser and proxies it to brclientd's /files/send endpoint, which
// stores the file under brclientd's UploadDir and dispatches it to BR's
// SendFile RPC. Caps the upload at 1 GiB.
func BisonrelayFileSendHandler(w http.ResponseWriter, r *http.Request) {
	const maxUpload = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	user := strings.TrimSpace(r.FormValue("user"))
	if user == "" {
		http.Error(w, "user field is required", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file part missing: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	mime := header.Header.Get("Content-Type")
	result, err := rpc.BrclientdSendFile(r.Context(), user, header.Filename, mime, file)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// BisonrelayDownloadHandler serves a completed file-transfer download that
// brclientd has written to <brclientd-data>/<network>/downloads/<nick>/<filename>.
func BisonrelayDownloadHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	contact := vars["contact"]
	filename := vars["filename"]
	if !downloadNickRe.MatchString(contact) || !downloadFileRe.MatchString(filename) {
		http.NotFound(w, r)
		return
	}
	network, _ := services.CurrentNetwork(r.Context())
	if network == "" {
		network = "mainnet"
	}
	root := filepath.Clean(services.BrclientdDownloadsDir(network))
	candidate := filepath.Clean(filepath.Join(root, contact, filename))
	if !strings.HasPrefix(candidate, root+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	http.ServeFile(w, r, candidate)
}

// BisonrelayDownloadsListHandler returns the list of files that brclientd has
// completed downloading from a given contact. Mirrors what shows up in the
// "files received from this contact" view of the chat thread. The contact
// path segment is the sender's nick as recorded under
// <brclientd-data>/<network>/downloads/<nick>/.
func BisonrelayDownloadsListHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	contact := vars["contact"]
	if !downloadNickRe.MatchString(contact) {
		http.NotFound(w, r)
		return
	}
	network, _ := services.CurrentNetwork(r.Context())
	if network == "" {
		network = "mainnet"
	}
	dir := filepath.Join(services.BrclientdDownloadsDir(network), contact)
	entries, err := os.ReadDir(dir)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"files":[]}`))
		return
	}
	type fileEntry struct {
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		ModTime int64  `json:"mtime"`
	}
	out := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, fileEntry{Name: e.Name(), Size: fi.Size(), ModTime: fi.ModTime().Unix()})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string][]fileEntry{"files": out})
}

// BisonrelayContactsHandler proxies brclientd's /contacts endpoint.
// Returns the BR client's in-memory address book (peers with completed KX).
func BisonrelayContactsHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdContacts(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayContactRenameHandler proxies brclientd's /contacts/rename
// endpoint. Body: {uid (hex), new_nick}. Persists a local NickAlias only;
// nothing is broadcast.
func BisonrelayContactRenameHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UID     string `json:"uid"`
		NewNick string `json:"new_nick"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.UID == "" || req.NewNick == "" {
		http.Error(w, "uid and new_nick are required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdRenameContact(r.Context(), req.UID, req.NewNick); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactKXResetHandler proxies brclientd's /contacts/kx-reset.
// Triggers a ratchet reset with the specified contact.
func BisonrelayContactKXResetHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := decodeBisonrelayUIDBody(w, r)
	if !ok {
		return
	}
	if err := rpc.BrclientdKXReset(r.Context(), uid); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactResetAllHandler proxies brclientd's /contacts/reset-all,
// which initiates a ratchet reset with every contact whose last received
// message is older than age_days (0 = brclientd's default). Passes the
// {started, count} JSON through so the UI can report how many resets were
// initiated.
func BisonrelayContactResetAllHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgeDays int `json:"age_days"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	raw, err := rpc.BrclientdResetAllRatchets(r.Context(), req.AgeDays)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

// BisonrelayConnectionHandler proxies brclientd's /connection: GET reports
// the requested online intent + effective session state + server policy;
// POST {online} flips the intent. The offline intent is runtime-only and
// resets to online when the daemon restarts.
func BisonrelayConnectionHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		raw, err := rpc.BrclientdConnectionState(r.Context())
		if err != nil {
			brWriteErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	case http.MethodPost:
		var req struct {
			Online bool `json:"online"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := rpc.BrclientdSetConnection(r.Context(), req.Online); err != nil {
			brWriteErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// BisonrelayTipAttemptsHandler returns the tracked tip attempts to one
// contact.
func BisonrelayTipAttemptsHandler(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimSpace(r.URL.Query().Get("uid"))
	if uid == "" {
		http.Error(w, "uid query param is required", http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdTipAttempts(r.Context(), uid)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayRunningTipsHandler returns the tip attempts the daemon is
// actively driving.
func BisonrelayRunningTipsHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdRunningTipAttempts(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayRTDTMessagesHandler returns the chat messages tracked for a
// live RTDT session.
func BisonrelayRTDTMessagesHandler(w http.ResponseWriter, r *http.Request) {
	rv := mux.Vars(r)["rv"]
	raw, err := rpc.BrclientdRTDTMessages(r.Context(), rv)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

// BisonrelayRTDTChatHandler sends a text message into a live RTDT session.
func BisonrelayRTDTChatHandler(w http.ResponseWriter, r *http.Request) {
	rv := mux.Vars(r)["rv"]
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdRTDTChat(r.Context(), rv, req.Message); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayKXSearchesHandler proxies brclientd's outstanding KX searches.
func BisonrelayKXSearchesHandler(w http.ResponseWriter, r *http.Request) {
	raw, err := rpc.BrclientdKXSearches(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

// BisonrelayMediateIDsHandler proxies the in-flight mediated introductions:
// GET lists, POST {mediator, target} cancels one.
func BisonrelayMediateIDsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		raw, err := rpc.BrclientdMediateIDs(r.Context())
		if err != nil {
			brWriteErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	case http.MethodPost:
		var req struct {
			Mediator string `json:"mediator"`
			Target   string `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Mediator == "" || req.Target == "" {
			http.Error(w, "mediator and target are required", http.StatusBadRequest)
			return
		}
		if err := rpc.BrclientdCancelMediateID(r.Context(), req.Mediator, req.Target); err != nil {
			brWriteErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// BisonrelayRecentNotificationsHandler returns brclientd's persisted daemon
// notes (newest first) for the BR notification bell.
func BisonrelayRecentNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	n := 50
	if v := strings.TrimSpace(r.URL.Query().Get("n")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			n = parsed
		}
	}
	body, err := rpc.BrclientdRecentNotifications(r.Context(), n)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayDeleteNotificationHandler removes a single BR notification-bell
// entry by id.
func BisonrelayDeleteNotificationHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdDeleteNotification(r.Context(), req.ID); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayClearNotificationsHandler removes all BR notification-bell entries.
func BisonrelayClearNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	if err := rpc.BrclientdClearNotifications(r.Context()); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayBehaviorSettingsHandler proxies brclientd's /settings/behavior: GET
// reports {saved, effective}; POST persists a partial update. The settings are
// fixed at BR-client construction, so a change takes effect on the next Bison
// Relay restart (nothing restarts on its own).
func BisonrelayBehaviorSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		raw, err := rpc.BrclientdBRBehavior(r.Context())
		if err != nil {
			brWriteErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	case http.MethodPost:
		var update map[string]any
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := rpc.BrclientdSetBRBehavior(r.Context(), update); err != nil {
			brWriteErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// BisonrelayFiltersHandler proxies brclientd's content filters: GET lists,
// POST upserts (id 0 creates) and returns the stored filter. brclientd 400s
// (e.g. an invalid regexp) pass through as 400 so the form can show them
// inline.
func BisonrelayFiltersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		raw, err := rpc.BrclientdListFilters(r.Context())
		if err != nil {
			brWriteErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	case http.MethodPost:
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		raw, err := rpc.BrclientdUpsertFilter(r.Context(), body)
		if err != nil {
			status := http.StatusBadGateway
			if strings.Contains(err.Error(), "HTTP 400") {
				status = http.StatusBadRequest
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// BisonrelayFilterDeleteHandler proxies brclientd's /filters/delete.
func BisonrelayFilterDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID uint64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdDeleteFilter(r.Context(), req.ID); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelaySubscribeAllPostsHandler proxies brclientd's
// /posts/subscribe-all, subscribing to the posts of every KX'd contact.
func BisonrelaySubscribeAllPostsHandler(w http.ResponseWriter, r *http.Request) {
	if err := rpc.BrclientdSubscribeAllPosts(r.Context()); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayKXListHandler proxies brclientd's /kx/list diagnostic (in-flight
// key exchanges, including reset KXs).
func BisonrelayKXListHandler(w http.ResponseWriter, r *http.Request) {
	raw, err := rpc.BrclientdKXList(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

// BisonrelayContactHandshakeHandler proxies brclientd's /contacts/handshake.
// Starts a 3-way handshake with the specified contact.
func BisonrelayContactHandshakeHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := decodeBisonrelayUIDBody(w, r)
	if !ok {
		return
	}
	if err := rpc.BrclientdHandshake(r.Context(), uid); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactBlockHandler proxies brclientd's /contacts/block. Blocks
// the contact: BR notifies the peer and the contact is removed locally.
func BisonrelayContactBlockHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := decodeBisonrelayUIDBody(w, r)
	if !ok {
		return
	}
	if err := rpc.BrclientdBlockContact(r.Context(), uid); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayBlockedContactsHandler proxies brclientd's /contacts/blocked. GET
// returns the locally blocked users ({blocked: [{uid, blockedAt}]}).
func BisonrelayBlockedContactsHandler(w http.ResponseWriter, r *http.Request) {
	raw, err := rpc.BrclientdBlockedContacts(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

// BisonrelayContactUnblockHandler proxies brclientd's /contacts/unblock. Body:
// {uid}. Removes the uid from the block list and restarts brclientd so the
// change takes effect; returns brclientd's body ({restarting: true}). Only
// clears this side: reconnecting still needs the peer to unblock and a fresh KX.
func BisonrelayContactUnblockHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := decodeBisonrelayUIDBody(w, r)
	if !ok {
		return
	}
	raw, err := rpc.BrclientdUnblockContact(r.Context(), uid)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

// BisonrelayClearHistoryHandler proxies brclientd's /history/pm/clear. Body:
// {uid}. Permanently deletes the local PM history + media for the contact;
// the contact itself remains. Irreversible.
func BisonrelayClearHistoryHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := decodeBisonrelayUIDBody(w, r)
	if !ok {
		return
	}
	if err := rpc.BrclientdClearPMHistory(r.Context(), uid); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactGroupsHandler proxies brclientd's /contacts/groups. GET
// returns the group layout (uid-keyed assignments); POST runs an action:
// {action: "create"|"rename"|"delete", id?, name?}.
func BisonrelayContactGroupsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		raw, err := rpc.BrclientdContactGroups(r.Context())
		if err != nil {
			brWriteErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
		return
	}
	var req struct {
		Action string `json:"action"`
		ID     string `json:"id"`
		Name   string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Action == "create" {
		raw, err := rpc.BrclientdContactGroupCreate(r.Context(), req.Name)
		if err != nil {
			brWriteErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
		return
	}
	if err := rpc.BrclientdContactGroupAction(r.Context(), req.Action, req.ID, req.Name); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactGroupAssignHandler proxies brclientd's
// /contacts/groups/assign. Body: {uid, group, pinned}.
func BisonrelayContactGroupAssignHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UID    string `json:"uid"`
		Group  string `json:"group"`
		Pinned bool   `json:"pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.UID == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdContactGroupAssign(r.Context(), req.UID, req.Group, req.Pinned); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactGroupSettingsHandler proxies brclientd's
// /contacts/groups/settings. Body: {auto_archive_days}.
func BisonrelayContactGroupSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AutoArchiveDays int `json:"auto_archive_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdContactGroupSettings(r.Context(), req.AutoArchiveDays); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayClearPayStatsHandler proxies brclientd's /stats/payments/clear.
// Body: {uid}. Permanently clears the recorded payment totals and breakdowns
// for the contact; funds, history, and the contact itself are untouched.
func BisonrelayClearPayStatsHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := decodeBisonrelayUIDBody(w, r)
	if !ok {
		return
	}
	if err := rpc.BrclientdClearPayStats(r.Context(), uid); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactIgnoreHandler proxies brclientd's /contacts/ignore. Body:
// {uid, ignore}. Sets or clears the local ignore flag for the contact.
func BisonrelayContactIgnoreHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UID    string `json:"uid"`
		Ignore bool   `json:"ignore"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.UID) == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdIgnoreContact(r.Context(), req.UID, req.Ignore); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactSuggestKXHandler proxies /contacts/suggest-kx. Body:
// {invitee, target}. The invitee is asked (over BR) to KX with the target.
func BisonrelayContactSuggestKXHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Invitee string `json:"invitee"`
		Target  string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Invitee == "" || req.Target == "" {
		http.Error(w, "invitee and target are required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdSuggestKX(r.Context(), req.Invitee, req.Target); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactTransResetHandler proxies /contacts/trans-reset. Body:
// {mediator, target}. The mediator is asked to forward a reset request to
// the target on our behalf.
func BisonrelayContactTransResetHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mediator string `json:"mediator"`
		Target   string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Mediator == "" || req.Target == "" {
		http.Error(w, "mediator and target are required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdTransReset(r.Context(), req.Mediator, req.Target); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactSubscribePostsHandler proxies the brclientd subscribe-
// posts endpoint. Body: {uid (hex)}. Asynchronous; the new subscription
// state is published as a posts-subscribed event.
func BisonrelayContactSubscribePostsHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := decodeBisonrelayUIDBody(w, r)
	if !ok {
		return
	}
	if err := rpc.BrclientdSubscribePosts(r.Context(), uid); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactUnsubscribePostsHandler proxies the brclientd
// unsubscribe-posts endpoint.
func BisonrelayContactUnsubscribePostsHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := decodeBisonrelayUIDBody(w, r)
	if !ok {
		return
	}
	if err := rpc.BrclientdUnsubscribePosts(r.Context(), uid); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactListPostsHandler proxies the brclientd list-posts
// endpoint. Async: the response lands on the live-event bus as a
// posts-list-received event for the matching uid.
func BisonrelayContactListPostsHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := decodeBisonrelayUIDBody(w, r)
	if !ok {
		return
	}
	if err := rpc.BrclientdListUserPosts(r.Context(), uid); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactFetchPostHandler proxies the brclientd fetch-post
// endpoint. Async: the post body arrives via the post-received event.
func BisonrelayContactFetchPostHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UID string `json:"uid"`
		PID string `json:"pid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.UID == "" || req.PID == "" {
		http.Error(w, "uid and pid are required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdFetchPost(r.Context(), req.UID, req.PID); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayPostCommentsHandler returns the comment list for a post.
func BisonrelayPostCommentsHandler(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimSpace(r.URL.Query().Get("uid"))
	pid := strings.TrimSpace(r.URL.Query().Get("pid"))
	if uid == "" || pid == "" {
		http.Error(w, "uid and pid query params are required", http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdPostComments(r.Context(), uid, pid)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayPostCommentHandler publishes a new comment on a post.
func BisonrelayPostCommentHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UID     string `json:"uid"`
		PID     string `json:"pid"`
		Comment string `json:"comment"`
		Parent  string `json:"parent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.UID == "" || req.PID == "" || strings.TrimSpace(req.Comment) == "" {
		http.Error(w, "uid, pid, and comment are required", http.StatusBadRequest)
		return
	}
	identifier, err := rpc.BrclientdPostComment(r.Context(), req.UID, req.PID, req.Comment, req.Parent)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"identifier": identifier})
}

// BisonrelayPostReceiveReceiptsHandler returns the receive receipts for one
// of the local user's own posts.
func BisonrelayPostReceiveReceiptsHandler(w http.ResponseWriter, r *http.Request) {
	pid := strings.TrimSpace(r.URL.Query().Get("pid"))
	if pid == "" {
		http.Error(w, "pid query param is required", http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdPostReceiveReceipts(r.Context(), pid)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayPostRelayHandler relays a post to one user or to all of the
// local client's post subscribers.
func BisonrelayPostRelayHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UID   string `json:"uid"`
		PID   string `json:"pid"`
		ToUID string `json:"toUid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.UID == "" || req.PID == "" {
		http.Error(w, "uid and pid are required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdRelayPost(r.Context(), req.UID, req.PID, req.ToUID); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayPostCommentReceiptsHandler returns the receive receipts for the
// comments on one of the local user's own posts, grouped by status id.
func BisonrelayPostCommentReceiptsHandler(w http.ResponseWriter, r *http.Request) {
	pid := strings.TrimSpace(r.URL.Query().Get("pid"))
	if pid == "" {
		http.Error(w, "pid query param is required", http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdPostCommentReceipts(r.Context(), pid)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayPostHeartsHandler returns the current heart count + my-own
// state for a single post.
func BisonrelayPostHeartsHandler(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimSpace(r.URL.Query().Get("uid"))
	pid := strings.TrimSpace(r.URL.Query().Get("pid"))
	if uid == "" || pid == "" {
		http.Error(w, "uid and pid query params are required", http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdPostHearts(r.Context(), uid, pid)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayPostHeartHandler toggles the local identity's heart on a post.
func BisonrelayPostHeartHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UID   string `json:"uid"`
		PID   string `json:"pid"`
		Heart bool   `json:"heart"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.UID == "" || req.PID == "" {
		http.Error(w, "uid and pid are required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdPostHeart(r.Context(), req.UID, req.PID, req.Heart); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelaySharedFilesHandler proxies brclientd's /shared-files list.
func BisonrelaySharedFilesHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdSharedFiles(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayManageAddHandler accepts a multipart upload from the browser
// (file + form fields cost_dcr, target_uid?, descr?) and proxies it as
// the /shared-files/add request to brclientd. Cost is collected in DCR
// for UX, converted to milliatoms before forwarding.
func BisonrelayManageAddHandler(w http.ResponseWriter, r *http.Request) {
	const maxUpload = 200 << 20 // 200 MiB upper bound; BR can store larger via chunks
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	costDCRStr := strings.TrimSpace(r.FormValue("cost_dcr"))
	var costAtoms uint64
	if costDCRStr != "" {
		costDCR, err := strconv.ParseFloat(costDCRStr, 64)
		if err != nil || costDCR < 0 {
			http.Error(w, "invalid cost_dcr", http.StatusBadRequest)
			return
		}
		// BR shared-file costs are in atoms (1 DCR = 1e8), not the milli-atoms
		// used for payment/tip records.
		costAtoms = uint64(math.Round(costDCR * 1e8))
	}
	targetUID := strings.TrimSpace(r.FormValue("target_uid"))
	descr := strings.TrimSpace(r.FormValue("descr"))

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file part missing: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()
	mime := header.Header.Get("Content-Type")
	body, err := rpc.BrclientdShareFile(r.Context(), header.Filename, mime, file, costAtoms, targetUID, descr)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayManageUnshareHandler removes a share. Body: {fid, target_uid?}.
func BisonrelayManageUnshareHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FID       string `json:"fid"`
		TargetUID string `json:"target_uid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.FID == "" {
		http.Error(w, "fid is required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdUnshareFile(r.Context(), req.FID, req.TargetUID); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayManageDownloadsHandler returns the flat downloads list.
func BisonrelayManageDownloadsHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdListDownloads(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayManageCancelDownloadHandler aborts an in-flight download.
func BisonrelayManageCancelDownloadHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FID string `json:"fid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.FID == "" {
		http.Error(w, "fid is required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdCancelDownload(r.Context(), req.FID); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayManageDeleteDownloadHandler deletes a completed received download's
// file from disk by proxying to brclientd (the dashboard mounts the BR data dir
// read-only, so it cannot remove the file itself). The file is addressed by fid
// (+ optional uid); brclientd resolves it to its own recorded path and refuses
// anything outside the downloads directory.
func BisonrelayManageDeleteDownloadHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FID string `json:"fid"`
		UID string `json:"uid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !downloadIDRe.MatchString(req.FID) {
		http.Error(w, "invalid fid", http.StatusBadRequest)
		return
	}
	if req.UID != "" && !downloadIDRe.MatchString(req.UID) {
		http.Error(w, "invalid uid", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdDeleteDownload(r.Context(), req.FID, req.UID); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayStatsOverviewHandler proxies brclientd's /stats/overview.
func BisonrelayStatsOverviewHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdStatsOverview(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayStatsPaymentsHandler proxies brclientd's /stats/payments.
func BisonrelayStatsPaymentsHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdStatsPayments(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayStatsNetworkHandler proxies brclientd's /stats/network.
func BisonrelayStatsNetworkHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdStatsNetwork(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayStatsContactsHandler proxies brclientd's /stats/contacts.
func BisonrelayStatsContactsHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdStatsContacts(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayStatsPostsHandler proxies brclientd's /stats/posts.
func BisonrelayStatsPostsHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdStatsPosts(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// ---- RTDT realtime-voice control plane ----------------------------------

// BisonrelayRTDTListHandler returns the list of RTDT sessions.
func BisonrelayRTDTListHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdRTDTList(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayRTDTCreateHandler creates a fresh session.
func BisonrelayRTDTCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Size        uint16 `json:"size"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdRTDTCreate(r.Context(), req.Size, req.Description)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayRTDTCreateInstantHandler creates an instant call.
func BisonrelayRTDTCreateInstantHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UIDs []string `json:"uids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdRTDTCreateInstant(r.Context(), req.UIDs)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayRTDTInviteHandler invites users to an existing session.
func BisonrelayRTDTInviteHandler(w http.ResponseWriter, r *http.Request) {
	rv := mux.Vars(r)["rv"]
	var req struct {
		UIDs        []string `json:"uids"`
		AsPublisher bool     `json:"as_publisher"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdRTDTInvite(r.Context(), rv, req.UIDs, req.AsPublisher); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayRTDTAcceptHandler accepts a pending invite.
func BisonrelayRTDTAcceptHandler(w http.ResponseWriter, r *http.Request) {
	rv := mux.Vars(r)["rv"]
	var req struct {
		Inviter     string `json:"inviter"`
		AsPublisher bool   `json:"as_publisher"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdRTDTAccept(r.Context(), rv, req.Inviter, req.AsPublisher); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayRTDTJoinHandler joins the live audio for a session.
func BisonrelayRTDTJoinHandler(w http.ResponseWriter, r *http.Request) {
	rv := mux.Vars(r)["rv"]
	if err := rpc.BrclientdRTDTJoin(r.Context(), rv); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayRTDTLeaveHandler leaves a session.
func BisonrelayRTDTLeaveHandler(w http.ResponseWriter, r *http.Request) {
	rv := mux.Vars(r)["rv"]
	if err := rpc.BrclientdRTDTLeave(r.Context(), rv); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayRTDTDissolveHandler dissolves a session (owner).
func BisonrelayRTDTDissolveHandler(w http.ResponseWriter, r *http.Request) {
	rv := mux.Vars(r)["rv"]
	if err := rpc.BrclientdRTDTDissolve(r.Context(), rv); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayRTDTKickHandler kicks a peer from the live session.
func BisonrelayRTDTKickHandler(w http.ResponseWriter, r *http.Request) {
	rv := mux.Vars(r)["rv"]
	var req struct {
		PeerID     uint32 `json:"peer_id"`
		BanSeconds int64  `json:"ban_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdRTDTKick(r.Context(), rv, req.PeerID, req.BanSeconds); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayRTDTRemoveHandler removes a member from the session metadata.
func BisonrelayRTDTRemoveHandler(w http.ResponseWriter, r *http.Request) {
	rv := mux.Vars(r)["rv"]
	var req struct {
		UID    string `json:"uid"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdRTDTRemove(r.Context(), rv, req.UID, req.Reason); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayRTDTRotateCookiesHandler invalidates current appointment cookies.
func BisonrelayRTDTRotateCookiesHandler(w http.ResponseWriter, r *http.Request) {
	rv := mux.Vars(r)["rv"]
	if err := rpc.BrclientdRTDTRotateCookies(r.Context(), rv); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayPostsRenderHandler renders a draft post body server-side so
// the editor's Preview tab matches the published Feed detail view. Body:
// {post}. Response shape mirrors /api/br/posts/body — same segmented
// {title, markdown, segments, attributes}.
func BisonrelayPostsRenderHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Post  string `json:"post"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	segments := services.SplitAndRenderBRPostBody(req.Post)
	out := map[string]any{
		"title":    req.Title,
		"markdown": req.Post,
		"segments": segments,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// BisonrelayPagesRenderHandler renders draft page markdown into structured
// segments (text/embed/form) with the same SplitAndRenderBRPage the Pages
// viewer uses, so the editor's Preview matches a hosted page (forms, sections,
// br:// links). Dashboard-only, no brclientd hop. Body: {markdown}.
func BisonrelayPagesRenderHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Markdown string `json:"markdown"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"markdown": req.Markdown,
		"segments": services.SplitAndRenderBRPage(req.Markdown),
	})
}

// BisonrelayPostsNewHandler authors a new post via brclientd.
func BisonrelayPostsNewHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Post  string `json:"post"`
		Descr string `json:"descr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Post) == "" {
		http.Error(w, "post body is required", http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdCreatePost(r.Context(), req.Post, req.Descr)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayPostsFeedHandler returns the local list of all received posts
// (summaries). Pure passthrough to brclientd's /posts/feed.
func BisonrelayPostsFeedHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdPostsFeed(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayPostBodyHandler fetches a single post's full body, splits it
// into segments around BR `--embed[...]--` tags, and renders each text
// segment to sanitized HTML. Returns {title, markdown, segments, attributes}
// where segments interleave rendered text and raw embed metadata.
func BisonrelayPostBodyHandler(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimSpace(r.URL.Query().Get("uid"))
	pid := strings.TrimSpace(r.URL.Query().Get("pid"))
	if uid == "" || pid == "" {
		http.Error(w, "uid and pid query params are required", http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdPostBody(r.Context(), uid, pid)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	var pm struct {
		Version    uint64            `json:"version"`
		Attributes map[string]string `json:"attributes"`
	}
	if err := json.Unmarshal(body, &pm); err != nil {
		http.Error(w, "decode post: "+err.Error(), http.StatusBadGateway)
		return
	}
	mainMD := pm.Attributes["main"]
	segments := services.SplitAndRenderBRPostBody(mainMD)
	out := map[string]any{
		"title":      pm.Attributes["title"],
		"markdown":   mainMD,
		"segments":   segments,
		"attributes": pm.Attributes,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// BisonrelayPagesFetchHandler fetches a single BR page (resource) via
// brclientd and renders its markdown into structured segments. Body:
// {uid, path, session_id?, parent_page?, data?, async_target_id?}. The
// brclientd call blocks until the reply lands (or times out), so this is a
// plain request/response from the dashboard's perspective.
func BisonrelayPagesFetchHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UID           string            `json:"uid"`
		Path          []string          `json:"path"`
		SessionID     uint64            `json:"session_id"`
		ParentPage    uint64            `json:"parent_page"`
		Data          json.RawMessage   `json:"data,omitempty"`
		FieldTypes    map[string]string `json:"field_types,omitempty"`
		AsyncTargetID string            `json:"async_target_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.UID) == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	// Coerce submitted form values to the JSON types the resource handler
	// expects, keyed by the declared field types, before relaying to the store
	// owner: an intinput becomes a JSON number, every other field a string. This
	// backstops the client (e.g. a quoted number would make the host reject the
	// form). field_types is the dashboard's own hint and is not forwarded.
	req.Data = sanitizeBRFormData(req.Data, req.FieldTypes)
	req.FieldTypes = nil
	body, err := rpc.BrclientdPagesFetch(r.Context(), req)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	var fetched struct {
		SessionID     uint64            `json:"session_id"`
		PageID        uint64            `json:"page_id"`
		ParentPage    uint64            `json:"parent_page"`
		Status        uint16            `json:"status"`
		Meta          map[string]string `json:"meta"`
		Markdown      string            `json:"markdown"`
		AsyncTargetID string            `json:"async_target_id"`
	}
	if err := json.Unmarshal(body, &fetched); err != nil {
		http.Error(w, "decode page reply: "+err.Error(), http.StatusBadGateway)
		return
	}
	out := map[string]any{
		"session_id":      fetched.SessionID,
		"page_id":         fetched.PageID,
		"parent_page":     fetched.ParentPage,
		"status":          fetched.Status,
		"meta":            fetched.Meta,
		"markdown":        fetched.Markdown,
		"async_target_id": fetched.AsyncTargetID,
		"segments":        services.SplitAndRenderBRPage(fetched.Markdown),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// sanitizeBRFormData coerces a page form's submitted values to the JSON types
// the resource handler expects, keyed by the field types the dashboard sent:
// intinput -> integer number, anything else -> string. Data that is not a JSON
// object (e.g. a bare-value form like order add-comment) or that has no field
// types is returned unchanged.
func sanitizeBRFormData(data json.RawMessage, fieldTypes map[string]string) json.RawMessage {
	if len(data) == 0 || len(fieldTypes) == 0 {
		return data
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return data
	}
	for k, v := range obj {
		if fieldTypes[k] == "intinput" {
			obj[k] = brFormInt(v)
		} else {
			obj[k] = brFormString(v)
		}
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return data
	}
	return b
}

func brFormInt(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case json.Number:
		n, _ := t.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return n
	default:
		return 0
	}
}

func brFormString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

// BisonrelayPagesLocalListHandler proxies the list of markdown pages this
// node hosts.
func BisonrelayPagesLocalListHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdPagesLocalList(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// safeBRPath reports whether a brclientd-bound page/template/store name or path
// is free of traversal. brclientd owns the real containment; this is a
// defense-in-depth guard so a crafted "../" never leaves the dashboard. It
// allows nested relative paths and rejects absolute paths, backslashes, NUL,
// the empty string, and any ".." segment.
func safeBRPath(p string) bool {
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

// safeStoreMediaPath is safeBRPath plus a denylist for the store file endpoints:
// .tmpl/.tmp must never be created or served as "media" because the store parses
// and executes *.tmpl as Go templates. Templates have their own /store/templates
// routes; this keeps the generic file upload/get/delete from reaching them.
func safeStoreMediaPath(p string) bool {
	if !safeBRPath(p) {
		return false
	}
	lower := strings.ToLower(strings.TrimRight(p, ". "))
	return !strings.HasSuffix(lower, ".tmpl") && !strings.HasSuffix(lower, ".tmp")
}

// BisonrelayPagesLocalFileHandler proxies the raw markdown of one hosted page.
func BisonrelayPagesLocalFileHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		http.Error(w, "name query param is required", http.StatusBadRequest)
		return
	}
	if !safeBRPath(name) {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdPagesLocalFile(r.Context(), name)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayPagesLocalSaveHandler creates or overwrites one hosted page.
// Body: {name, content}.
func BisonrelayPagesLocalSaveHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !safeBRPath(strings.TrimSpace(req.Name)) {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdPagesLocalSave(r.Context(), req); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayPagesLocalDeleteHandler removes one hosted page. Body: {name}.
func BisonrelayPagesLocalDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !safeBRPath(strings.TrimSpace(req.Name)) {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdPagesLocalDelete(r.Context(), req); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContentGetHandler initiates a download of a shared file (FID) that
// a page or post advertised via --embed[download=<fid>,cost=,...]--. The
// daemon pays per-chunk only up to maxCostAtoms (0 = free files only); a
// higher real share cost cancels the download and emits a
// file-download-cost-rejected event so the UI can re-confirm with the actual
// price. The bytes are served by BisonrelayContentFileHandler once the
// download completes. Body: {uid, fid, maxCostAtoms?}.
func BisonrelayContentGetHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UID          string `json:"uid"`
		FID          string `json:"fid"`
		MaxCostAtoms uint64 `json:"maxCostAtoms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.UID == "" || req.FID == "" {
		http.Error(w, "uid and fid are required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdContentGet(r.Context(), req.UID, req.FID, req.MaxCostAtoms); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContentFileHandler streams the bytes of a downloaded shared file
// from brclientd so the viewer can show it inline or offer it for download.
// Query: fid (required), uid (optional). Returns 404 until the download is
// complete. The brclientd side serves only files from its download records.
func BisonrelayContentFileHandler(w http.ResponseWriter, r *http.Request) {
	fid := strings.TrimSpace(r.URL.Query().Get("fid"))
	uid := strings.TrimSpace(r.URL.Query().Get("uid"))
	if fid == "" {
		http.Error(w, "fid query param is required", http.StatusBadRequest)
		return
	}
	resp, err := rpc.BrclientdContentFile(r.Context(), uid, fid)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		http.Error(w, string(body), resp.StatusCode)
		return
	}
	for _, h := range []string{"Content-Type", "Content-Disposition", "Content-Length"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	_, _ = io.Copy(w, resp.Body)
}

// maxEmbedServeBytes mirrors brclientd's per-embed ceiling: bound the proxied
// embed bytes so a crafted post cannot stream an outsized payload through the
// dashboard. brclientd already rejects larger embeds; this is belt-and-suspenders.
const maxEmbedServeBytes = 16 << 20

// BisonrelayPostsEmbedDataHandler streams the inline payload of one post
// embed from brclientd so feed cards can render images without shipping
// base64 in the feed JSON. Query: uid (author), pid (required), index
// (optional, default 0). Cache-Control is forwarded because posts are
// immutable and brclientd marks the bytes long-lived.
func BisonrelayPostsEmbedDataHandler(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimSpace(r.URL.Query().Get("uid"))
	pid := strings.TrimSpace(r.URL.Query().Get("pid"))
	if uid == "" || pid == "" {
		http.Error(w, "uid and pid query params are required", http.StatusBadRequest)
		return
	}
	index := 0
	if idxStr := strings.TrimSpace(r.URL.Query().Get("index")); idxStr != "" {
		n, err := strconv.Atoi(idxStr)
		if err != nil || n < 0 {
			http.Error(w, "invalid index", http.StatusBadRequest)
			return
		}
		index = n
	}
	resp, err := rpc.BrclientdPostEmbedData(r.Context(), uid, pid, index)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		http.Error(w, string(body), resp.StatusCode)
		return
	}
	for _, h := range []string{"Content-Type", "Content-Length", "Cache-Control"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	_, _ = io.Copy(w, io.LimitReader(resp.Body, maxEmbedServeBytes))
}

// Single-slot prepared-backup state. brclientd builds the entire backup
// tarball before sending response headers, so the dashboard prepares the
// download in a detached job and serves it from a local temp file once
// ready; the browser polls instead of hanging on a silent request.
var (
	brBackupMu        sync.Mutex
	brBackupState     string // "" (idle) | "preparing" | "ready" | "error"
	brBackupErr       string
	brBackupPath      string
	brBackupFilename  string
	brBackupCType     string
	brBackupSize      int64
	brBackupStartedAt time.Time
	brBackupReadyAt   time.Time
	// brBackupGen invalidates a prepare superseded by a newer one; only the
	// goroutine whose generation is still current may publish its result.
	brBackupGen int64
)

type brBackupStatus struct {
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
	Filename  string `json:"filename,omitempty"`
	Size      int64  `json:"size,omitempty"`
	StartedAt int64  `json:"startedAt,omitempty"`
	ReadyAt   int64  `json:"readyAt,omitempty"`
}

// brBackupStatusLocked snapshots the slot; callers must hold brBackupMu.
func brBackupStatusLocked() brBackupStatus {
	st := brBackupStatus{
		State:    brBackupState,
		Error:    brBackupErr,
		Filename: brBackupFilename,
		Size:     brBackupSize,
	}
	if st.State == "" {
		st.State = "idle"
	}
	if !brBackupStartedAt.IsZero() {
		st.StartedAt = brBackupStartedAt.Unix()
	}
	if !brBackupReadyAt.IsZero() {
		st.ReadyAt = brBackupReadyAt.Unix()
	}
	return st
}

// BisonrelayBackupPrepareHandler starts (or joins) a detached backup
// preparation job and immediately returns the slot status. The job outlives
// the request, so the browser may leave the page and poll later.
func BisonrelayBackupPrepareHandler(w http.ResponseWriter, r *http.Request) {
	brBackupMu.Lock()
	if brBackupState != "preparing" {
		brBackupState = "preparing"
		brBackupErr = ""
		brBackupStartedAt = time.Now()
		brBackupGen++
		go runBrBackupPrepare(brBackupGen, brBackupPath)
	}
	st := brBackupStatusLocked()
	brBackupMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// BisonrelayBackupStatusHandler reports the prepared-backup slot so the UI
// can poll a running preparation and resume after navigation.
func BisonrelayBackupStatusHandler(w http.ResponseWriter, r *http.Request) {
	brBackupMu.Lock()
	st := brBackupStatusLocked()
	brBackupMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// runBrBackupPrepare fetches the tarball from brclientd's /backup and spools
// it to a temp file. Detached from the originating request so navigation
// does not abort it; the context bounds a stalled stream so the slot cannot
// stay "preparing" forever.
func runBrBackupPrepare(gen int64, oldPath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Reap temp files orphaned by a previous dashboard process. The current
	// ready file is excluded: it stays serveable until the new one lands.
	if matches, err := filepath.Glob(filepath.Join(os.TempDir(), "br-backup-*.tar.gz")); err == nil {
		for _, m := range matches {
			if m != oldPath {
				_ = os.Remove(m)
			}
		}
	}

	resp, err := rpc.BrclientdBackup(ctx)
	if err != nil {
		failBrBackup(gen, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		failBrBackup(gen, fmt.Sprintf("brclientd /backup: HTTP %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body))))
		return
	}

	filename := "bisonrelay-backup.tar.gz"
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil {
		if fn := filepath.Base(params["filename"]); fn != "" && fn != "." {
			filename = fn
		}
	}
	filename = strings.NewReplacer("\r", "", "\n", "", `"`, "").Replace(filename)
	ctype := resp.Header.Get("Content-Type")
	if ctype == "" {
		ctype = "application/gzip"
	}

	tmp, err := os.CreateTemp("", "br-backup-*.tar.gz")
	if err != nil {
		failBrBackup(gen, "create temp file: "+err.Error())
		return
	}
	n, err := io.Copy(tmp, resp.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	// A short read must not publish a silently truncated backup; any
	// mismatch with the advertised length is a failure.
	if err == nil && resp.ContentLength > 0 && n != resp.ContentLength {
		err = fmt.Errorf("incomplete transfer: got %d of %d bytes", n, resp.ContentLength)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		failBrBackup(gen, "spool backup: "+err.Error())
		return
	}

	brBackupMu.Lock()
	if gen != brBackupGen {
		brBackupMu.Unlock()
		_ = os.Remove(tmp.Name())
		return
	}
	brBackupState = "ready"
	brBackupErr = ""
	brBackupPath = tmp.Name()
	brBackupFilename = filename
	brBackupCType = ctype
	brBackupSize = n
	brBackupReadyAt = time.Now()
	brBackupMu.Unlock()
	// In-flight readers of the old file keep their open fd; only the
	// name is unlinked.
	if oldPath != "" && oldPath != tmp.Name() {
		_ = os.Remove(oldPath)
	}
}

func failBrBackup(gen int64, msg string) {
	log.Printf("BR backup prepare failed: %s", msg)
	brBackupMu.Lock()
	if gen == brBackupGen {
		brBackupState = "error"
		brBackupErr = msg
	}
	brBackupMu.Unlock()
}

// BisonrelayBackupHandler serves the prepared backup tarball as an
// attachment. The file is opened while holding the slot lock so a
// concurrent re-prepare cannot unlink the name between the ready check and
// the open; once open, the fd keeps serving even if the file is replaced
// mid-transfer. ServeContent supplies Content-Length and range support for
// resumable multi-GB downloads.
func BisonrelayBackupHandler(w http.ResponseWriter, r *http.Request) {
	brBackupMu.Lock()
	if brBackupState != "ready" {
		brBackupMu.Unlock()
		http.Error(w, "no backup prepared", http.StatusConflict)
		return
	}
	f, err := os.Open(brBackupPath)
	filename, ctype, modtime := brBackupFilename, brBackupCType, brBackupReadyAt
	brBackupMu.Unlock()
	if err != nil {
		http.Error(w, "open backup: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	http.ServeContent(w, r, filename, modtime, f)
}

// BisonrelayRestoreBackupHandler streams an uploaded backup tarball through to
// brclientd's pre-setup /restore-backup endpoint. Only valid while brclientd
// is in the needs-identity stage; the daemon stages the tarball and restarts
// to extract it, so the BR setup wizard resumes via status polling. The
// upload arrives as multipart (exempt from the router-wide body cap) and its
// first part is streamed through without buffering; the cap mirrors
// brclientd's own restore limit.
func BisonrelayRestoreBackupHandler(w http.ResponseWriter, r *http.Request) {
	const maxUpload = 5 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	part, err := mr.NextPart()
	if err != nil {
		http.Error(w, "read multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer part.Close()
	if err := rpc.BrclientdRestoreBackup(r.Context(), part); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayRatesHandler proxies brclientd's /rates (DCR/USD + BTC/USD, with
// the source that produced them and a last-updated stamp).
func BisonrelayRatesHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdRates(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayStoreModeHandler proxies brclientd's /store/mode. GET returns the
// node's current resource-hosting mode; POST switches it (body {enabled,
// pay_type, account, ship_charge}).
func BisonrelayStoreModeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		body, err := rpc.BrclientdStoreMode(r.Context())
		if err != nil {
			brWriteErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdSetStoreMode(r.Context(), req)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayStoreProductsHandler proxies brclientd's /store/products: GET lists
// the catalog, POST upserts a product.
func BisonrelayStoreProductsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		body, err := rpc.BrclientdStoreProducts(r.Context())
		if err != nil {
			brWriteErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdSaveStoreProduct(r.Context(), req); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayStoreProductDeleteHandler removes a storefront product. Body: {sku}.
func BisonrelayStoreProductDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SKU string `json:"sku"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SKU == "" {
		http.Error(w, "sku is required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdDeleteStoreProduct(r.Context(), req.SKU); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayStoreOrdersHandler proxies brclientd's /store/orders (the full
// order list, newest first).
func BisonrelayStoreOrdersHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdStoreOrders(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayStoreOrderStatusHandler updates one order's status. Body:
// {uid, id, status}.
func BisonrelayStoreOrderStatusHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UID    string `json:"uid"`
		ID     uint64 `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.UID == "" || req.Status == "" {
		http.Error(w, "uid and status are required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdSetStoreOrderStatus(r.Context(), req.UID, req.ID, req.Status); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayStoreTemplatesHandler proxies brclientd's /store/templates (the
// list of storefront *.tmpl files).
func BisonrelayStoreTemplatesHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdStoreTemplates(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayStoreTemplateFileHandler returns one template's content. Query:
// name.
func BisonrelayStoreTemplateFileHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		http.Error(w, "name query param is required", http.StatusBadRequest)
		return
	}
	if !safeBRPath(name) {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	body, err := rpc.BrclientdStoreTemplateFile(r.Context(), name)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayStoreTemplateSaveHandler writes a template. Body: {name, content}.
func BisonrelayStoreTemplateSaveHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !safeBRPath(strings.TrimSpace(req.Name)) {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdSaveStoreTemplate(r.Context(), req); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayStoreTemplateDeleteHandler removes a template. Body: {name}.
func BisonrelayStoreTemplateDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !safeBRPath(strings.TrimSpace(req.Name)) {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdDeleteStoreTemplate(r.Context(), req.Name); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayStoreFilesListHandler proxies brclientd's /store/files/list (the
// user-managed media files under the store dir: covers, banner, downloads).
func BisonrelayStoreFilesListHandler(w http.ResponseWriter, r *http.Request) {
	body, err := rpc.BrclientdListStoreFiles(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayStoreFileGetHandler streams one store file (image preview or
// download). Query: path.
func BisonrelayStoreFileGetHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		http.Error(w, "path query param is required", http.StatusBadRequest)
		return
	}
	if !safeStoreMediaPath(path) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	data, ctype, err := rpc.BrclientdGetStoreFile(r.Context(), path)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	_, _ = w.Write(data)
}

// BisonrelayStoreFileDeleteHandler removes one media file under the store dir.
// Body: {path}.
func BisonrelayStoreFileDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	p := strings.TrimSpace(req.Path)
	if p == "" || !safeStoreMediaPath(p) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdDeleteStoreFile(r.Context(), p); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayStoreOrderCommentHandler appends a merchant comment to an order
// (brclientd DMs the buyer). Body: {uid, id, comment}.
func BisonrelayStoreOrderCommentHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UID     string `json:"uid"`
		ID      uint64 `json:"id"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.UID == "" || req.Comment == "" {
		http.Error(w, "uid and comment are required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdAddStoreOrderComment(r.Context(), req.UID, req.ID, req.Comment); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayStoreFileUploadHandler proxies a multipart upload to brclientd's
// /store/files/upload (a digital-download file stored at the given path under
// the store dir). Form: path + file.
func BisonrelayStoreFileUploadHandler(w http.ResponseWriter, r *http.Request) {
	const maxUpload = 200 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()
	relPath := strings.TrimSpace(r.FormValue("path"))
	overwrite := r.FormValue("overwrite") == "true"
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file part missing: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()
	if relPath != "" && !safeStoreMediaPath(relPath) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if strings.ContainsRune(header.Filename, '/') || !safeStoreMediaPath(header.Filename) {
		http.Error(w, "invalid file name", http.StatusBadRequest)
		return
	}
	mime := header.Header.Get("Content-Type")
	body, err := rpc.BrclientdUploadStoreFile(r.Context(), relPath, header.Filename, mime, overwrite, file)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayContactListContentHandler proxies the brclientd list-content
// endpoint. Async: the response lands as content-list-received.
func BisonrelayContactListContentHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := decodeBisonrelayUIDBody(w, r)
	if !ok {
		return
	}
	if err := rpc.BrclientdListUserContent(r.Context(), uid); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactTipHandler proxies PaymentsService.TipUser. Body:
// {uid, dcrAmount, maxAttempts}. uid is the 64-hex identity. dcrAmount is
// in DCR (float). maxAttempts is BR's retry budget for the tip; the
// dashboard defaults to 1 when omitted to match the modal's "send tip
// once" semantics.
func BisonrelayContactTipHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UID         string  `json:"uid"`
		DCRAmount   float64 `json:"dcrAmount"`
		MaxAttempts int32   `json:"maxAttempts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.UID == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return
	}
	if req.DCRAmount <= 0 {
		http.Error(w, "dcrAmount must be positive", http.StatusBadRequest)
		return
	}
	if req.MaxAttempts <= 0 {
		req.MaxAttempts = 1
	}
	if err := rpc.BrclientdTipUser(r.Context(), req.UID, req.DCRAmount, req.MaxAttempts); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactAcceptSuggestionHandler accepts an inbound KX
// suggestion: asks the mediator to introduce us to the target.
func BisonrelayContactAcceptSuggestionHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mediator string `json:"mediator"`
		Target   string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Mediator == "" || req.Target == "" {
		http.Error(w, "mediator and target are required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdAcceptSuggestion(r.Context(), req.Mediator, req.Target); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeBisonrelayUIDBody parses {uid: "<hex>"} from the request body.
// Writes a 400 + returns false on failure so callers can return immediately.
func decodeBisonrelayUIDBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		UID string `json:"uid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return "", false
	}
	if req.UID == "" {
		http.Error(w, "uid is required", http.StatusBadRequest)
		return "", false
	}
	return req.UID, true
}

// maxInlineEmbedBytes is the size cap (in decoded bytes) for an inline
// attachment that rides in the PM body via the bruig --embed[...]-- tag.
// Stays comfortably under the 1 MiB floor of BR's per-PM payload limit.
const maxInlineEmbedBytes = 800 * 1024

// BisonrelayPMHandler sends a PM through brclientd. Body:
// {user, msg, embed?: {name, mime, data_b64}}. When an embed is present
// it is rendered into the bruig-compatible --embed[...]-- markdown tag
// and appended to msg before forwarding to ChatService.PM. Responds with
// {body: "<synthesised wire body>"} so the caller can echo it optimistically.
func BisonrelayPMHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User  string `json:"user"`
		Msg   string `json:"msg"`
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
	req.User = strings.TrimSpace(req.User)
	req.Msg = strings.TrimSpace(req.Msg)
	if req.User == "" {
		http.Error(w, "user is required", http.StatusBadRequest)
		return
	}
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

	if err := rpc.BrclientdSendPM(r.Context(), req.User, body); err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"body": body})
}

// buildEmbedTag renders bruig's --embed[...]-- tag. Field order mirrors
// internal/mdembeds/mdembeds.go so an audited peer parses it identically.
// Bruig does no escaping on name/type/data; commas in name would break the
// parser, so we strip them defensively.
func buildEmbedTag(name, mime, dataB64 string) string {
	name = strings.ReplaceAll(name, ",", "")
	name = strings.ReplaceAll(name, "=", "")
	mime = strings.ReplaceAll(mime, ",", "")
	mime = strings.ReplaceAll(mime, "=", "")
	var parts []string
	if name != "" {
		parts = append(parts, "name="+name)
	}
	if mime != "" {
		parts = append(parts, "type="+mime)
	}
	if dataB64 != "" {
		parts = append(parts, "data="+dataB64)
	}
	return "--embed[" + strings.Join(parts, ",") + "]--"
}

// BisonrelayInviteWriteHandler asks brclientd to mint a fresh OOB invite.
// Returns {"invite_bytes": "<base64 binary blob>", "invite_key": "brpik1..."}
// so the caller can share whichever form is more convenient.
func BisonrelayInviteWriteHandler(w http.ResponseWriter, r *http.Request) {
	result, err := rpc.BrclientdWriteNewInvite(r.Context())
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"invite_bytes": result.InviteBytes,
		"invite_key":   result.InviteKey,
	})
}

// BisonrelayInviteAcceptHandler dispatches an inbound invite to the right
// brclientd path based on its format. brpik1 bech32 keys go through
// /invites/redeem-key (fetch encrypted blob + decrypt + accept); base64
// invite blobs go through ChatService.AcceptInvite. Body accepts either
// {invite: "..."} or the legacy {invite_bytes: "..."}.
func BisonrelayInviteAcceptHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Invite      string `json:"invite"`
		InviteBytes string `json:"invite_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(req.Invite)
	if value == "" {
		value = strings.TrimSpace(req.InviteBytes)
	}
	if value == "" {
		http.Error(w, "invite is required", http.StatusBadRequest)
		return
	}

	if strings.HasPrefix(value, "brpik1") {
		if err := rpc.BrclientdRedeemPaidInviteKey(r.Context(), value); err != nil {
			brWriteErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	body, err := rpc.BrclientdAcceptInvite(r.Context(), value)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelayMessagesHandler proxies brclientd's /history/pm endpoint. Query
// params: contact (hex peer UID, required), page (default 0), page_size
// (default 50, max 500). Returns the raw JSON envelope brclientd produces so
// the dashboard stays stateless w.r.t. chat history.
func BisonrelayMessagesHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	contact := strings.TrimSpace(q.Get("contact"))
	if contact == "" {
		http.Error(w, "contact query param is required", http.StatusBadRequest)
		return
	}
	page := 0
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			page = n
		}
	}
	pageSize := 50
	if v := q.Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			pageSize = n
		}
	}
	body, err := rpc.BrclientdHistoryPM(r.Context(), contact, page, pageSize)
	if err != nil {
		brWriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// BisonrelaySetupHandler proxies a nick/name pair to brclientd's pre-setup
// /create-identity endpoint. The frontend wizard only calls this when
// /api/br/status reports stage=needs-identity; outside that window
// brclientd's port is owned by clientrpc and the call 404s.
func BisonrelaySetupHandler(w http.ResponseWriter, r *http.Request) {
	if rejectWatchOnly(w, r) {
		return
	}
	if ready, reason := services.WalletReady(r.Context()); !ready {
		http.Error(w, reason, http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Nick string `json:"nick"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Nick = strings.TrimSpace(req.Nick)
	req.Name = strings.TrimSpace(req.Name)
	if req.Nick == "" {
		http.Error(w, "nick is required", http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdCreateIdentity(r.Context(), req.Nick, req.Name); err != nil {
		brWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
