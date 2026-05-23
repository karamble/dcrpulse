// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
		http.Error(w, err.Error(), http.StatusBadGateway)
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
		http.Error(w, err.Error(), http.StatusBadGateway)
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
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(id)
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
	embedContactRe   = regexp.MustCompile(`^[0-9a-f]{16}$`)
	embedFilenameRe  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	downloadNickRe   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	downloadFileRe   = regexp.MustCompile(`^[A-Za-z0-9._ -]+$`)
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
// SendFile RPC. Caps the upload at 100 MiB.
func BisonrelayFileSendHandler(w http.ResponseWriter, r *http.Request) {
	const maxUpload = 100 << 20
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
		http.Error(w, err.Error(), http.StatusBadGateway)
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
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		ModTime  int64  `json:"mtime"`
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
		http.Error(w, err.Error(), http.StatusBadGateway)
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
		http.Error(w, err.Error(), http.StatusBadGateway)
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
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BisonrelayContactHandshakeHandler proxies brclientd's /contacts/handshake.
// Starts a 3-way handshake with the specified contact.
func BisonrelayContactHandshakeHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := decodeBisonrelayUIDBody(w, r)
	if !ok {
		return
	}
	if err := rpc.BrclientdHandshake(r.Context(), uid); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
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
		http.Error(w, err.Error(), http.StatusBadGateway)
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
		http.Error(w, err.Error(), http.StatusBadGateway)
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
		http.Error(w, err.Error(), http.StatusBadGateway)
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
		http.Error(w, err.Error(), http.StatusBadGateway)
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
		http.Error(w, err.Error(), http.StatusBadGateway)
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
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	body, err := rpc.BrclientdAcceptInvite(r.Context(), value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
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
		http.Error(w, err.Error(), http.StatusBadGateway)
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
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
