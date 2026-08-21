// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strconv"
	"sync"
	"time"
)

// BrclientdConfig holds the brclientd clientrpc connection parameters.
// Server cert pins TLS trust; the client cert pair authenticates back.
type BrclientdConfig struct {
	Host           string
	Port           string
	StatusPort     string
	ServerCertPath string
	ClientCertPath string
	ClientKeyPath  string
}

var (
	// BrclientdCfg is the resolved config used for late-binding cert
	// reads on every call (cert files may not exist until brclientd has
	// finished its first-run identity setup).
	BrclientdCfg BrclientdConfig

	brclientdHTTPClient *http.Client
	// brclientdStreamHTTPClient has no overall timeout; backup tarballs can
	// take far longer than 90s to transfer.
	brclientdStreamHTTPClient *http.Client
	// brclientdBackupHTTPClient additionally stretches the response-header
	// deadline: brclientd builds the entire backup tarball before sending
	// headers, which can exceed the stream client's 60s on multi-GB states.
	brclientdBackupHTTPClient *http.Client
	// brclientdPagesHTTPClient has no overall timeout and no response-header
	// deadline: a page fetch travels over the relay and brclientd buffers the
	// whole reply before sending headers, so the transfer size and time are
	// unbounded. Total time is bounded by the request context (the caller's
	// own connection) instead of a fixed deadline.
	brclientdPagesHTTPClient *http.Client
	brclientdClientMu        sync.Mutex
)

// InitBrclientdConfig records the brclientd clientrpc connection settings.
// The HTTP client is built lazily on the first call so the dashboard can
// start before brclientd has issued its cert pair.
func InitBrclientdConfig(cfg BrclientdConfig) {
	brclientdClientMu.Lock()
	defer brclientdClientMu.Unlock()
	BrclientdCfg = cfg
	dropBrclientdClients()
}

// dropBrclientdClients releases the cached clients so the next call rebuilds
// them against the current certs. Callers hold brclientdClientMu. Closing idle
// connections first strands nothing on the old chain; a connection carrying a
// request is left alone.
func dropBrclientdClients() {
	for _, c := range []**http.Client{
		&brclientdHTTPClient,
		&brclientdStreamHTTPClient,
		&brclientdBackupHTTPClient,
		&brclientdPagesHTTPClient,
	} {
		if *c != nil {
			(*c).CloseIdleConnections()
			*c = nil
		}
	}
}

// brclientdConfig returns a consistent copy of the connection settings. The cert
// paths are rewritten on a wallet switch, so anything outside the cached clients
// must take a snapshot rather than copy the global.
func brclientdConfig() BrclientdConfig {
	brclientdClientMu.Lock()
	defer brclientdClientMu.Unlock()
	return BrclientdCfg
}

// UpdateBrclientdCerts repoints brclientd at a different wallet's identity certs
// and forces the HTTP client to rebuild on next use. Used on a wallet switch.
func UpdateBrclientdCerts(serverCertPath, clientCertPath, clientKeyPath string) {
	brclientdClientMu.Lock()
	BrclientdCfg.ServerCertPath = serverCertPath
	BrclientdCfg.ClientCertPath = clientCertPath
	BrclientdCfg.ClientKeyPath = clientKeyPath
	dropBrclientdClients()
	brclientdClientMu.Unlock()
	// Drop the live WS so it redials and rebuilds TLS with the new cert
	// immediately instead of waiting for its current socket to die.
	BrclientdWS().Reconnect()
}

// BrclientdVersionResult is the wire shape returned by VersionService.Version.
type BrclientdVersionResult struct {
	AppName         string `json:"appName"`
	AppVersion      string `json:"appVersion"`
	GoRuntime       string `json:"goRuntime"`
	BRClientVersion string `json:"brClientVersion,omitempty"`
}

// BrclientdVersion reads brclientd's /version status endpoint and returns the
// appName / appVersion / goRuntime triple.
func BrclientdVersion(ctx context.Context) (*BrclientdVersionResult, error) {
	raw, err := brclientdGetRaw(ctx, "/version", nil)
	if err != nil {
		return nil, err
	}
	var result BrclientdVersionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode version: %w", err)
	}
	return &result, nil
}

// BrclientdUserPublicIdentity reads brclientd's /public-identity status
// endpoint and returns the raw JSON. Used by the dashboard to confirm the BR
// client core is operational and to render the local user's pubkey + nick on
// the BR overview. identity and sigKey are base64-encoded.
func BrclientdUserPublicIdentity(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/public-identity", nil)
}

// BrclientdSetAvatar sets or clears the local user's avatar via brclientd's
// /avatar status endpoint. avatarB64 is base64-encoded image bytes; an empty
// string clears it. BR caps avatars at 200KiB and broadcasts the change.
func BrclientdSetAvatar(ctx context.Context, avatarB64 string) error {
	return brclientdPostJSON(ctx, "/avatar", map[string]string{"avatar": avatarB64})
}

// BrclientdCreateIdentity POSTs to brclientd's pre-setup HTTPS endpoint
// at /create-identity (the same port as clientrpc, served only while the
// daemon is in the needs-identity stage). Returns nil on HTTP 204.
func BrclientdCreateIdentity(ctx context.Context, nick, name string) error {
	cli, err := brclientdClient()
	if err != nil {
		return err
	}
	url, err := brclientdEndpoint(setupPort, "/create-identity", nil)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{"nick": nick, "name": name})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("brclientd /create-identity: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("brclientd /create-identity: HTTP %d: %s", resp.StatusCode, body)
	}
	return nil
}

// BrclientdSendFileResult is the JSON shape brclientd returns from
// /files/send: the on-disk filename it stored under UploadDir and the
// size of the upload.
type BrclientdSendFileResult struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// brclientdUpload posts a multipart form of the given fields plus one file
// part. cli is a parameter because a slow endpoint needs the no-deadline
// client; the request context bounds it instead.
func brclientdUpload(ctx context.Context, cli *http.Client, path brPath, fields [][2]string,
	filename, mime string, body io.Reader) (json.RawMessage, error) {

	endpoint, err := brclientdEndpoint(statusPort, path, nil)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	mp := multipart.NewWriter(pw)
	go func() {
		defer pw.Close()
		defer mp.Close()
		for _, f := range fields {
			if err := mp.WriteField(f[0], f[1]); err != nil {
				pw.CloseWithError(err)
				return
			}
		}
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
		if mime != "" {
			hdr.Set("Content-Type", mime)
		}
		part, err := mp.CreatePart(hdr)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, body); err != nil {
			pw.CloseWithError(err)
			return
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pr)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", mp.FormDataContentType())
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd %s: %w", path, err)
	}
	defer resp.Body.Close()
	respBody, err := readBrclientdBody(resp, path, 1<<20)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brclientd %s: HTTP %d: %s", path, resp.StatusCode, respBody)
	}
	return json.RawMessage(respBody), nil
}

// BrclientdSendFile uploads a file to brclientd's /files/send mTLS endpoint,
// which persists the bytes under UploadDir and dispatches them to BR's
// SendFile RPC. user can be a nick / alias / hex UID.
func BrclientdSendFile(ctx context.Context, user, filename, mime string, body io.Reader) (*BrclientdSendFileResult, error) {
	// brclientd's c.SendFile blocks synchronously on per-chunk relay acks, so the
	// response header can be delayed well past the default client's 60s header
	// deadline for a large/slow transfer. Use the shared no-deadline client (also
	// used for page fetch); the request context bounds it instead.
	cli, err := brclientdPagesClient()
	if err != nil {
		return nil, err
	}
	respBody, err := brclientdUpload(ctx, cli, "/files/send",
		[][2]string{{"user", user}}, filename, mime, body)
	if err != nil {
		return nil, err
	}
	var result BrclientdSendFileResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode send-file response: %w", err)
	}
	return &result, nil
}

// BrclientdContacts returns the BR client's address book entries from
// brclientd's /contacts endpoint. Returns the raw JSON envelope so the
// dashboard does not need to keep types in sync with BR's AddressBookEntry.
func BrclientdContacts(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRawLimit(ctx, "/contacts", nil, 4<<20)
}

// BrclientdMsigHistory returns the shared-wallet coordination frames
// exchanged with one contact from brclientd's /msig/history endpoint, both
// directions, oldest first. The msig engine uses it as the replay source
// after downtime; live frames ride the notification stream.
func BrclientdMsigHistory(ctx context.Context, uidHex string, limit int, since int64) (json.RawMessage, error) {
	return brclientdGetRawLimit(ctx, "/msig/history", map[string]string{
		"uid":   uidHex,
		"limit": strconv.Itoa(limit),
		"since": strconv.FormatInt(since, 10),
	}, 16<<20)
}

// BrclientdSendPM sends a private message through brclientd's /messages/send
// status endpoint. `user` can be a nick, alias, or hex peer UID.
func BrclientdSendPM(ctx context.Context, user, msg string) error {
	return brclientdPostJSON(ctx, "/messages/send", map[string]any{
		"user":    user,
		"message": msg,
	})
}

// BrclientdInviteResult bundles the two share-forms BR's WriteNewInvite
// produces: the binary OOB invite blob (base64 over the wire) and the
// bech32 brpik1 key that points at the same prepaid invite on the BR
// server. Sharing either gets a peer the same KX outcome.
type BrclientdInviteResult struct {
	InviteBytes string
	InviteKey   string
}

// BrclientdWriteNewInvite creates an OOB invite via brclientd's
// /invites/create status endpoint and returns both share-forms.
func BrclientdWriteNewInvite(ctx context.Context) (*BrclientdInviteResult, error) {
	raw, err := brclientdPostJSONRaw(ctx, "/invites/create", struct{}{})
	if err != nil {
		return nil, err
	}
	var resp struct {
		InviteBytes string `json:"inviteBytes"`
		InviteKey   string `json:"inviteKey"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode invite: %w", err)
	}
	return &BrclientdInviteResult{InviteBytes: resp.InviteBytes, InviteKey: resp.InviteKey}, nil
}

// BrclientdRedeemPaidInviteKey resolves a brpik1 bech32 key against the BR
// server and starts a key exchange with the resulting invite. Hits
// brclientd's /invites/redeem-key bridge endpoint which clientrpc itself
// does not expose.
func BrclientdRedeemPaidInviteKey(ctx context.Context, key string) error {
	return brclientdPostJSON(ctx, "/invites/redeem-key", map[string]string{"key": key})
}

// BrclientdRenameContact sets the local NickAlias on a contact. uidHex is
// the 64-char hex identity. Pure clientdb mutation; nothing is broadcast.
func BrclientdRenameContact(ctx context.Context, uidHex, newNick string) error {
	return brclientdPostJSON(ctx, "/contacts/rename", map[string]string{
		"uid":      uidHex,
		"new_nick": newNick,
	})
}

// BrclientdKXReset triggers a ratchet reset with the specified contact.
// Wraps brclientd's /contacts/kx-reset which calls client.ResetRatchet.
func BrclientdKXReset(ctx context.Context, uidHex string) error {
	return brclientdPostJSON(ctx, "/contacts/kx-reset", map[string]string{"uid": uidHex})
}

// BrclientdResetAllRatchets initiates a ratchet reset with every contact
// whose last received message is older than ageDays (0 = brclientd's
// default). Wraps brclientd's /contacts/reset-all; returns its
// {started, count} JSON. Initiation only - the resets complete in the
// background whenever each peer comes online.
func BrclientdResetAllRatchets(ctx context.Context, ageDays int) (json.RawMessage, error) {
	return brclientdPostJSONRaw(ctx, "/contacts/reset-all", map[string]int{"age_days": ageDays})
}

// BrclientdConnectionState returns brclientd's /connection JSON: requested
// online intent, effective connection state and the server policy.
func BrclientdConnectionState(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/connection", nil)
}

// BrclientdSetConnection flips the daemon's connection intent (GoOnline /
// RemainOffline). The offline intent does not survive a daemon restart.
func BrclientdSetConnection(ctx context.Context, online bool) error {
	return brclientdPostJSON(ctx, "/connection", map[string]bool{"online": online})
}

// BrclientdTipAttempts returns the locally tracked tip attempts to one
// contact (amounts, retries, invoice/payment timestamps, completion state).
func BrclientdTipAttempts(ctx context.Context, uidHex string) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/payments/tips", map[string]string{"uid": uidHex})
}

// BrclientdRunningTipAttempts returns the tip attempts the daemon is
// actively driving, with their next scheduled action.
func BrclientdRunningTipAttempts(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/payments/tips/running", nil)
}

// BrclientdRTDTMessages returns the chat messages tracked for a live RTDT
// session.
func BrclientdRTDTMessages(ctx context.Context, rv ShortIDHex) (json.RawMessage, error) {
	return brclientdGetRawID(ctx, "/rtdt/sessions", rv, "/messages", nil)
}

// BrclientdRTDTChat sends a text message into a live RTDT session.
func BrclientdRTDTChat(ctx context.Context, rv ShortIDHex, message string) error {
	return brclientdPostJSONID(ctx, "/rtdt/sessions", rv, "/chat", map[string]string{
		"message": message,
	})
}

// BrclientdKXSearches returns the outstanding KX searches.
func BrclientdKXSearches(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/kx/searches", nil)
}

// BrclientdMediateIDs returns the in-flight mediated introduction requests.
func BrclientdMediateIDs(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/kx/mediateids", nil)
}

// BrclientdCancelMediateID cancels an in-flight mediated introduction.
func BrclientdCancelMediateID(ctx context.Context, mediatorHex, targetHex string) error {
	return brclientdPostJSON(ctx, "/kx/mediateids", map[string]string{
		"mediator": mediatorHex,
		"target":   targetHex,
	})
}

// BrclientdRecentNotifications returns brclientd's persisted daemon notes
// (newest first) that power the BR notification bell. Unlike the live
// /notifications stream these survive the browser being closed.
func BrclientdRecentNotifications(ctx context.Context, n int) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/notifications/recent", map[string]string{
		"n": fmt.Sprintf("%d", n),
	})
}

// BrclientdDeleteNotification removes a single persisted daemon note by id.
func BrclientdDeleteNotification(ctx context.Context, id int64) error {
	return brclientdPostJSON(ctx, "/notifications/delete", map[string]any{"id": id})
}

// BrclientdClearNotifications removes all persisted daemon notes.
func BrclientdClearNotifications(ctx context.Context) error {
	return brclientdPostJSON(ctx, "/notifications/clear", nil)
}

// BrclientdBRBehavior returns brclientd's /settings/behavior JSON:
// {saved, effective} resolved BR behavior settings (send-receive-receipts, idle
// auto-removal, auto-subscribe, auto-handshake, GC invite expiry, RTDT chat).
func BrclientdBRBehavior(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/settings/behavior", nil)
}

// BrclientdSetBRBehavior persists a partial BR behavior update. The values are
// fixed at BR-client construction, so a change takes effect on the next daemon
// restart; brclientd does not restart on its own.
func BrclientdSetBRBehavior(ctx context.Context, update any) error {
	return brclientdPostJSON(ctx, "/settings/behavior", update)
}

// BrclientdListFilters returns the active content filters as brclientd's
// {filters: [...]} JSON.
func BrclientdListFilters(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/filters", nil)
}

// BrclientdUpsertFilter creates or updates a content filter (id 0 creates)
// and returns the stored filter JSON including the assigned id.
func BrclientdUpsertFilter(ctx context.Context, body json.RawMessage) (json.RawMessage, error) {
	return brclientdPostJSONRaw(ctx, "/filters", body)
}

// BrclientdDeleteFilter removes a content filter by id.
func BrclientdDeleteFilter(ctx context.Context, id uint64) error {
	return brclientdPostJSON(ctx, "/filters/delete", map[string]uint64{"id": id})
}

// BrclientdSubscribeAllPosts subscribes to the posts of every KX'd contact.
// Synchronous through brclientd's send queue; can take a while on large
// address books.
func BrclientdSubscribeAllPosts(ctx context.Context) error {
	return brclientdPostJSON(ctx, "/posts/subscribe-all", struct{}{})
}

// BrclientdKXList returns the in-flight key exchanges as brclientd's
// {kxs: [...]} JSON.
func BrclientdKXList(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/kx/list", nil)
}

// BrclientdHandshake starts a 3-way handshake with the specified contact.
// Wraps brclientd's /contacts/handshake which calls client.Handshake.
func BrclientdHandshake(ctx context.Context, uidHex string) error {
	return brclientdPostJSON(ctx, "/contacts/handshake", map[string]string{"uid": uidHex})
}

// BrclientdBlockContact blocks a contact. Wraps brclientd's /contacts/block
// which calls client.Block. Destructive: BR notifies the peer and removes
// the contact (and its message log) locally; irreversible short of a fresh KX.
func BrclientdBlockContact(ctx context.Context, uidHex string) error {
	return brclientdPostJSON(ctx, "/contacts/block", map[string]string{"uid": uidHex})
}

// BrclientdBlockedContacts returns the locally blocked users (uid + block time)
// read from brclientd's /contacts/blocked. Forwarded as-is to the dashboard.
func BrclientdBlockedContacts(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/contacts/blocked", nil)
}

// BrclientdUnblockContact removes a uid from the BR block list. Wraps
// brclientd's /contacts/unblock, which rewrites blockedusers.json and restarts
// the daemon so the change takes effect. Returns brclientd's response body
// ({"restarting":true}). Only clears this side; reconnecting still needs the
// peer to unblock and a fresh KX.
func BrclientdUnblockContact(ctx context.Context, uidHex string) (json.RawMessage, error) {
	return brclientdPostJSONRaw(ctx, "/contacts/unblock", map[string]string{"uid": uidHex})
}

// BrclientdClearPMHistory permanently deletes the local PM history (and inline
// media) for a contact. Wraps brclientd's /history/pm/clear, which removes the
// on-disk message log(s) + embeds for the uid. The contact and ratchet remain;
// only the local copy is wiped. Irreversible.
func BrclientdClearPMHistory(ctx context.Context, uidHex string) error {
	return brclientdPostJSON(ctx, "/history/pm/clear", map[string]string{"uid": uidHex})
}

// BrclientdClearPayStats removes the recorded payment stats for a contact.
// Wraps brclientd's /stats/payments/clear (client.ClearPayStats); the contact
// drops out of the payment stats listing until new payments are recorded.
func BrclientdClearPayStats(ctx context.Context, uidHex string) error {
	return brclientdPostJSON(ctx, "/stats/payments/clear", map[string]string{"uid": uidHex})
}

// BrclientdContactGroups returns the contact group layout: groups list,
// uid-keyed assignments, and the auto-archive threshold.
func BrclientdContactGroups(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/contacts/groups", nil)
}

// BrclientdContactGroupCreate creates a named contact group and returns it.
func BrclientdContactGroupCreate(ctx context.Context, name string) (json.RawMessage, error) {
	return brclientdPostJSONRaw(ctx, "/contacts/groups",
		map[string]string{"action": "create", "name": name})
}

// BrclientdContactGroupAction renames or deletes a contact group.
func BrclientdContactGroupAction(ctx context.Context, action, id, name string) error {
	return brclientdPostJSON(ctx, "/contacts/groups",
		map[string]string{"action": action, "id": id, "name": name})
}

// BrclientdContactGroupAssign moves a contact into a group ("" returns it to
// the regular list).
func BrclientdContactGroupAssign(ctx context.Context, uidHex, group string, pinned bool) error {
	return brclientdPostJSON(ctx, "/contacts/groups/assign",
		map[string]any{"uid": uidHex, "group": group, "pinned": pinned})
}

// BrclientdContactGroupSettings sets the auto-archive threshold in days.
func BrclientdContactGroupSettings(ctx context.Context, days int) error {
	return brclientdPostJSON(ctx, "/contacts/groups/settings",
		map[string]int{"auto_archive_days": days})
}

// BrclientdIgnoreContact sets or clears the local ignore flag on a contact.
// Wraps brclientd's /contacts/ignore which calls client.Ignore. Local-only;
// nothing is broadcast. The flag surfaces as the contact's `ignored` field.
func BrclientdIgnoreContact(ctx context.Context, uidHex string, ignore bool) error {
	return brclientdPostJSON(ctx, "/contacts/ignore", map[string]any{
		"uid":    uidHex,
		"ignore": ignore,
	})
}

// BrclientdSuggestKX asks `invitee` to KX with `target`. Wraps
// brclientd's /contacts/suggest-kx which calls client.SuggestKX.
func BrclientdSuggestKX(ctx context.Context, inviteeHex, targetHex string) error {
	return brclientdPostJSON(ctx, "/contacts/suggest-kx", map[string]string{
		"invitee": inviteeHex,
		"target":  targetHex,
	})
}

// BrclientdTransReset asks `mediator` to forward a reset request to
// `target`. Wraps brclientd's /contacts/trans-reset which calls
// client.RequestTransitiveReset.
func BrclientdTransReset(ctx context.Context, mediatorHex, targetHex string) error {
	return brclientdPostJSON(ctx, "/contacts/trans-reset", map[string]string{
		"mediator": mediatorHex,
		"target":   targetHex,
	})
}

// BrclientdSubscribePosts asks the remote user to start sending us their
// posts. Async: completion surfaces via the posts-subscribed live event.
func BrclientdSubscribePosts(ctx context.Context, uidHex string) error {
	return brclientdPostJSON(ctx, "/contacts/subscribe-posts", map[string]string{"uid": uidHex})
}

// BrclientdUnsubscribePosts is the inverse of BrclientdSubscribePosts.
func BrclientdUnsubscribePosts(ctx context.Context, uidHex string) error {
	return brclientdPostJSON(ctx, "/contacts/unsubscribe-posts", map[string]string{"uid": uidHex})
}

// BrclientdListUserPosts kicks off a request to the remote user for their
// post list. Async: results arrive via the posts-list-received event.
func BrclientdListUserPosts(ctx context.Context, uidHex string) error {
	return brclientdPostJSON(ctx, "/contacts/list-posts", map[string]string{"uid": uidHex})
}

// BrclientdListUserContent kicks off a request to the remote user for the
// list of files they have shared. Async: results arrive via the
// content-list-received event.
func BrclientdListUserContent(ctx context.Context, uidHex string) error {
	return brclientdPostJSON(ctx, "/contacts/list-content", map[string]string{"uid": uidHex})
}

// BrclientdFetchPost asks the remote user for a specific post. Wraps
// brclientd's /contacts/fetch-post which calls SubscribeToPostsAndFetch
// (idempotent w.r.t. subscription state). The body arrives via the
// post-received live event when the remote replies.
func BrclientdFetchPost(ctx context.Context, uidHex, pidHex string) error {
	return brclientdPostJSON(ctx, "/contacts/fetch-post", map[string]string{
		"uid": uidHex,
		"pid": pidHex,
	})
}

// BrclientdCreatePost authors a new post and shares it with our existing
// subscribers. Returns the new post's summary JSON envelope.
func BrclientdCreatePost(ctx context.Context, post, descr string) (json.RawMessage, error) {
	return brclientdPostJSONRaw(ctx, "/posts/new", map[string]string{
		"post":  post,
		"descr": descr,
	})
}

// BrclientdSharedFiles returns the list of files the local user has shared.
// Used by the BR editor's "Link to shared content" picker.
func BrclientdSharedFiles(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/shared-files", nil)
}

// BrclientdShareFile streams a local file to brclientd's /shared-files/add
// endpoint together with sharing parameters. costAtoms is the per-fetch price
// in atoms (1 DCR = 1e8; 0 = free), targetUIDHex empty = global share.
// Returns the new SharedFile envelope brclientd emits.
func BrclientdShareFile(ctx context.Context, filename, mime string, body io.Reader, costAtoms uint64, targetUIDHex, descr string) (json.RawMessage, error) {
	// brclientd chunks and hashes the whole upload into BR's content store
	// before answering, so the response header can be delayed well past the
	// shared client's 60s deadline for a large file. Use the no-deadline client
	// (as /files/send does); the request context bounds it instead.
	cli, err := brclientdPagesClient()
	if err != nil {
		return nil, err
	}
	fields := [][2]string{{"cost_atoms", strconv.FormatUint(costAtoms, 10)}}
	if targetUIDHex != "" {
		fields = append(fields, [2]string{"target_uid", targetUIDHex})
	}
	if descr != "" {
		fields = append(fields, [2]string{"descr", descr})
	}
	return brclientdUpload(ctx, cli, "/shared-files/add", fields, filename, mime, body)
}

// BrclientdUnshareFile revokes a share. targetUIDHex empty removes the
// global share entry; otherwise revokes just the per-user share.
func BrclientdUnshareFile(ctx context.Context, fidHex, targetUIDHex string) error {
	return brclientdPostJSON(ctx, "/shared-files/remove", map[string]string{
		"fid":        fidHex,
		"target_uid": targetUIDHex,
	})
}

// BrclientdListDownloads returns the flat list of in-flight + completed
// file transfers tracked by BR.
func BrclientdListDownloads(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/downloads", nil)
}

// BrclientdCancelDownload aborts an in-flight download by FID.
func BrclientdCancelDownload(ctx context.Context, fidHex string) error {
	return brclientdPostJSON(ctx, "/downloads/cancel", map[string]string{"fid": fidHex})
}

// BrclientdDeleteDownload removes a completed received download's file from disk
// via brclientd (uid disambiguates the same file from multiple peers; empty is
// allowed). brclientd hard-locks the removal to its downloads directory.
func BrclientdDeleteDownload(ctx context.Context, fidHex, uidHex string) error {
	return brclientdPostJSON(ctx, "/downloads/delete", map[string]string{"fid": fidHex, "uid": uidHex})
}

// BrclientdContentGet asks brclientd to start downloading a shared file (FID)
// from a remote user, as advertised by an --embed[download=<fid>,cost=,...]--
// tag. The daemon pays per-chunk only when the cost stored on the host's
// share is at most maxCostAtoms (0 = free files only); a higher real cost
// cancels the download and emits a file-download-cost-rejected event.
// Progress is tracked via BrclientdListDownloads and the file-download-*
// events.
func BrclientdContentGet(ctx context.Context, uidHex, fidHex string, maxCostAtoms uint64) error {
	return brclientdPostJSON(ctx, "/content/get", map[string]any{
		"uid":            uidHex,
		"fid":            fidHex,
		"max_cost_atoms": maxCostAtoms,
	})
}

// BrclientdContentFile opens a streaming GET against brclientd's /content/file
// for a fully-downloaded shared file. The caller owns resp.Body and must close
// it. uidHex may be empty to match the file from any peer.
func BrclientdContentFile(ctx context.Context, uidHex, fidHex string) (*http.Response, error) {
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	// An absent uid stays absent rather than becoming uid=.
	q := map[string]string{"fid": fidHex}
	if uidHex != "" {
		q["uid"] = uidHex
	}
	endpoint, err := brclientdEndpoint(statusPort, "/content/file", q)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd /content/file: %w", err)
	}
	return resp, nil
}

// BrclientdPostEmbedData opens a streaming GET against brclientd's
// /posts/embed-data for the inline payload of one post embed. The caller
// owns resp.Body and must close it.
func BrclientdPostEmbedData(ctx context.Context, uidHex, pidHex string, index int) (*http.Response, error) {
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	endpoint, err := brclientdEndpoint(statusPort, "/posts/embed-data", map[string]string{
		"uid":   uidHex,
		"pid":   pidHex,
		"index": strconv.Itoa(index),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd /posts/embed-data: %w", err)
	}
	return resp, nil
}

// BrclientdBackup opens a streaming GET against brclientd's /backup status
// endpoint, which serves a full-state tarball produced by BR's client.Backup
// (consistent snapshot under a clientdb read transaction). The caller owns
// resp.Body and must close it, and should bound total time via ctx.
func BrclientdBackup(ctx context.Context) (*http.Response, error) {
	cli, err := brclientdBackupClient()
	if err != nil {
		return nil, err
	}
	url, err := brclientdEndpoint(statusPort, "/backup", nil)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd /backup: %w", err)
	}
	return resp, nil
}

// BrclientdRestoreBackup streams a backup tarball to brclientd's pre-setup
// /restore-backup endpoint (same port as /create-identity, served only while
// the daemon is in the needs-identity stage). On HTTP 204 the daemon stages
// the tarball and restarts to extract it.
func BrclientdRestoreBackup(ctx context.Context, body io.Reader) error {
	cli, err := brclientdStreamClient()
	if err != nil {
		return err
	}
	url, err := brclientdEndpoint(setupPort, "/restore-backup", nil)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/gzip")
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("brclientd /restore-backup: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("brclientd /restore-backup: HTTP %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// BrclientdRates returns the current exchange rates as {dcr_usd, btc_usd,
// source, updated_at}. brclientd serves BR's built-in rate, falling back to
// Kraken's DCR/USD when BR has none.
func BrclientdRates(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/rates", nil)
}

// BrclientdStoreMode returns the node's resource-hosting mode {enabled,
// pay_type, account, ship_charge}: static pages or a simplestore.
func BrclientdStoreMode(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/store/mode", nil)
}

// BrclientdSetStoreMode flips the node between pages and store hosting. body is
// {enabled, pay_type, account, ship_charge}.
func BrclientdSetStoreMode(ctx context.Context, body any) (json.RawMessage, error) {
	return brclientdPostJSONRaw(ctx, "/store/mode", body)
}

// BrclientdStoreProducts returns the storefront's product catalog.
func BrclientdStoreProducts(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/store/products", nil)
}

// BrclientdSaveStoreProduct upserts a product (body is the product object).
func BrclientdSaveStoreProduct(ctx context.Context, body any) error {
	return brclientdPostJSON(ctx, "/store/products", body)
}

// BrclientdDeleteStoreProduct removes a product by SKU.
func BrclientdDeleteStoreProduct(ctx context.Context, sku string) error {
	return brclientdPostJSON(ctx, "/store/products/delete", map[string]string{"sku": sku})
}

// BrclientdUploadStoreFile streams a file to brclientd's /store/files/upload,
// stored under the store dir at relPath, for products to reference via
// sendfilename (digital downloads). Returns {path}.
func BrclientdUploadStoreFile(ctx context.Context, relPath, filename, mime string, overwrite bool, body io.Reader) (json.RawMessage, error) {
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	var fields [][2]string
	if relPath != "" {
		fields = append(fields, [2]string{"path", relPath})
	}
	if overwrite {
		fields = append(fields, [2]string{"overwrite", "true"})
	}
	return brclientdUpload(ctx, cli, "/store/files/upload", fields, filename, mime, body)
}

// BrclientdListStoreFiles lists the user-managed media files under the store dir
// (cover images, banner, digital-download goods). Returns {files:[{path,size,...}]}.
func BrclientdListStoreFiles(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/store/files/list", nil)
}

// BrclientdDeleteStoreFile removes one media file under the store dir.
func BrclientdDeleteStoreFile(ctx context.Context, path string) error {
	return brclientdPostJSON(ctx, "/store/files/delete", map[string]string{"path": path})
}

// BrclientdGetStoreFile fetches one store file's bytes (for preview/download),
// returning the body and its Content-Type.
func BrclientdGetStoreFile(ctx context.Context, path string) ([]byte, string, error) {
	cli, err := brclientdClient()
	if err != nil {
		return nil, "", err
	}
	u, err := brclientdEndpoint(statusPort, "/store/files/get", map[string]string{"path": path})
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build get-file request: %w", err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("brclientd /store/files/get: %w", err)
	}
	defer resp.Body.Close()
	body, err := readBrclientdBody(resp, "/store/files/get", 256<<20)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("brclientd /store/files/get: HTTP %d: %s", resp.StatusCode, body)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

// BrclientdStoreTemplates lists the storefront's *.tmpl files.
func BrclientdStoreTemplates(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/store/templates", nil)
}

// BrclientdStoreTemplateFile returns one template's raw content.
func BrclientdStoreTemplateFile(ctx context.Context, name string) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/store/templates/file", map[string]string{"name": name})
}

// BrclientdSaveStoreTemplate writes (creates or overwrites) a template. body is
// {name, content}.
func BrclientdSaveStoreTemplate(ctx context.Context, body any) error {
	return brclientdPostJSON(ctx, "/store/templates/save", body)
}

// BrclientdDeleteStoreTemplate removes a template by name.
func BrclientdDeleteStoreTemplate(ctx context.Context, name string) error {
	return brclientdPostJSON(ctx, "/store/templates/delete", map[string]string{"name": name})
}

// BrclientdStoreOrders returns all storefront orders (across customers).
func BrclientdStoreOrders(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/store/orders", nil)
}

// BrclientdSetStoreOrderStatus updates one order's status. status is one of
// placed/paid/shipped/completed/canceled.
func BrclientdSetStoreOrderStatus(ctx context.Context, uid string, id uint64, status string) error {
	return brclientdPostJSON(ctx, "/store/orders/status", map[string]any{
		"uid":    uid,
		"id":     id,
		"status": status,
	})
}

// BrclientdAddStoreOrderComment appends a merchant comment to an order (and
// brclientd DMs the buyer).
func BrclientdAddStoreOrderComment(ctx context.Context, uid string, id uint64, comment string) error {
	return brclientdPostJSON(ctx, "/store/orders/comment", map[string]any{
		"uid":     uid,
		"id":      id,
		"comment": comment,
	})
}

// BrclientdStatsOverview returns the compact summary shown on the Stats
// landing page (hero counters + top contacts + connection-health).
func BrclientdStatsOverview(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/stats/overview", nil)
}

// BrclientdStatsPayments returns the per-user payment table with
// per-user prefix breakdowns and RMQ RTT quantiles.
func BrclientdStatsPayments(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/stats/payments", nil)
}

// BrclientdStatsNetwork returns server policy + connection metadata + RMQ
// quantile histogram.
func BrclientdStatsNetwork(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/stats/network", nil)
}

// BrclientdStatsContacts returns per-contact metadata + ratchet debug info.
func BrclientdStatsContacts(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/stats/contacts", nil)
}

// BrclientdStatsPosts returns authored-post engagement aggregates + sub
// counts.
func BrclientdStatsPosts(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/stats/posts", nil)
}

// ---- RTDT realtime-voice control plane ----------------------------------
//
// All wrappers below are thin pass-throughs over brclientd's /rtdt/sessions
// routes. Audio (the binary WebSocket) is handled by a separate dashboard
// proxy handler in Phase 3, not via this client.

// BrclientdRTDTList returns the list of RTDT sessions known locally.
func BrclientdRTDTList(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/rtdt/sessions", nil)
}

// BrclientdRTDTCreate creates a new RTDT session with the given capacity.
func BrclientdRTDTCreate(ctx context.Context, size uint16, description string) (json.RawMessage, error) {
	return brclientdPostJSONRaw(ctx, "/rtdt/sessions/create", map[string]any{
		"size":        size,
		"description": description,
	})
}

// BrclientdRTDTCreateInstant creates an instant 1:1 (or N:1) call invite +
// auto-join in one shot.
func BrclientdRTDTCreateInstant(ctx context.Context, uids []string) (json.RawMessage, error) {
	return brclientdPostJSONRaw(ctx, "/rtdt/sessions/create-instant", map[string]any{
		"uids": uids,
	})
}

// BrclientdRTDTInvite invites users to an existing RTDT session.
func BrclientdRTDTInvite(ctx context.Context, rv ShortIDHex, uids []string, asPublisher bool) error {
	return brclientdPostJSONID(ctx, "/rtdt/sessions", rv, "/invite", map[string]any{
		"uids":         uids,
		"as_publisher": asPublisher,
	})
}

// BrclientdRTDTAccept accepts a pending invite to an RTDT session.
func BrclientdRTDTAccept(ctx context.Context, rv ShortIDHex, inviter string, asPublisher bool) error {
	return brclientdPostJSONID(ctx, "/rtdt/sessions", rv, "/accept", map[string]any{
		"inviter":      inviter,
		"as_publisher": asPublisher,
	})
}

// BrclientdRTDTJoin connects the live UDP audio for an accepted session.
func BrclientdRTDTJoin(ctx context.Context, rv ShortIDHex) error {
	return brclientdPostJSONID(ctx, "/rtdt/sessions", rv, "/join", map[string]any{})
}

// BrclientdRTDTLeave leaves an RTDT session (member action).
func BrclientdRTDTLeave(ctx context.Context, rv ShortIDHex) error {
	return brclientdPostJSONID(ctx, "/rtdt/sessions", rv, "/leave", map[string]any{})
}

// BrclientdRTDTDissolve tears down an RTDT session (owner only).
func BrclientdRTDTDissolve(ctx context.Context, rv ShortIDHex) error {
	return brclientdPostJSONID(ctx, "/rtdt/sessions", rv, "/dissolve", map[string]any{})
}

// BrclientdRTDTKick removes a peer from the live audio session.
func BrclientdRTDTKick(ctx context.Context, rv ShortIDHex, peerID uint32, banSeconds int64) error {
	return brclientdPostJSONID(ctx, "/rtdt/sessions", rv, "/kick", map[string]any{
		"peer_id":     peerID,
		"ban_seconds": banSeconds,
	})
}

// BrclientdRTDTRemove removes a member from the session metadata.
func BrclientdRTDTRemove(ctx context.Context, rv ShortIDHex, uid, reason string) error {
	return brclientdPostJSONID(ctx, "/rtdt/sessions", rv, "/remove", map[string]any{
		"uid":    uid,
		"reason": reason,
	})
}

// BrclientdRTDTRotateCookies invalidates current appointment cookies.
func BrclientdRTDTRotateCookies(ctx context.Context, rv ShortIDHex) error {
	return brclientdPostJSONID(ctx, "/rtdt/sessions", rv, "/rotate-cookies", map[string]any{})
}

// ---- GC (group-chat) control plane --------------------------------------
//
// All wrappers are thin pass-throughs over brclientd's /gc routes. The
// summary endpoints (List, Detail) return raw JSON; the mutator endpoints
// either return raw JSON (Create) or expect 204 No Content.

func BrclientdGCList(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/gc", nil)
}

func BrclientdGCCreate(ctx context.Context, name string) (json.RawMessage, error) {
	return brclientdPostJSONRaw(ctx, "/gc/create", map[string]any{"name": name})
}

func BrclientdGCInvitesList(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/gc/invites", nil)
}

func BrclientdGCInvitesAccept(ctx context.Context, iid uint64) error {
	return brclientdPostJSON(ctx, "/gc/invites/accept", map[string]any{"iid": iid})
}

func BrclientdGCDetail(ctx context.Context, gcid ShortIDHex) (json.RawMessage, error) {
	return brclientdGetRawID(ctx, "/gc", gcid, "", nil)
}

func BrclientdGCInvite(ctx context.Context, gcid ShortIDHex, uid string) error {
	return brclientdPostJSONID(ctx, "/gc", gcid, "/invite", map[string]any{"uid": uid})
}

func BrclientdGCMessage(ctx context.Context, gcid ShortIDHex, message string, mode int) error {
	return brclientdPostJSONID(ctx, "/gc", gcid, "/message", map[string]any{
		"message": message,
		"mode":    mode,
	})
}

func BrclientdGCHistory(ctx context.Context, gcid ShortIDHex, page, pageSize int) (json.RawMessage, error) {
	q := map[string]string{}
	if page > 0 {
		q["page"] = strconv.Itoa(page)
	}
	if pageSize > 0 {
		q["page_size"] = strconv.Itoa(pageSize)
	}
	return brclientdGetRawID(ctx, "/gc", gcid, "/history", q)
}

// BrclientdGCClearHistory permanently deletes the local scrollback (and inline
// media) for a group chat. Wraps brclientd's /gc/{gcid}/history/clear, which
// removes the on-disk message log(s) + embeds for the gcid. Membership and the
// group itself remain; only the local copy is wiped. Irreversible.
func BrclientdGCClearHistory(ctx context.Context, gcid ShortIDHex) error {
	return brclientdPostJSONID(ctx, "/gc", gcid, "/history/clear", map[string]any{})
}

func BrclientdGCPart(ctx context.Context, gcid ShortIDHex, reason string) error {
	return brclientdPostJSONID(ctx, "/gc", gcid, "/part", map[string]any{"reason": reason})
}

func BrclientdGCKill(ctx context.Context, gcid ShortIDHex, reason string) error {
	return brclientdPostJSONID(ctx, "/gc", gcid, "/kill", map[string]any{"reason": reason})
}

func BrclientdGCKick(ctx context.Context, gcid ShortIDHex, uid, reason string) error {
	return brclientdPostJSONID(ctx, "/gc", gcid, "/kick", map[string]any{
		"uid":    uid,
		"reason": reason,
	})
}

func BrclientdGCBlock(ctx context.Context, gcid ShortIDHex, uid string) error {
	return brclientdPostJSONID(ctx, "/gc", gcid, "/block", map[string]any{"uid": uid})
}

func BrclientdGCUnblock(ctx context.Context, gcid ShortIDHex, uid string) error {
	return brclientdPostJSONID(ctx, "/gc", gcid, "/unblock", map[string]any{"uid": uid})
}

func BrclientdGCModifyAdmins(ctx context.Context, gcid ShortIDHex, extraAdmins []string, reason string) error {
	return brclientdPostJSONID(ctx, "/gc", gcid, "/admins", map[string]any{
		"extra_admins": extraAdmins,
		"reason":       reason,
	})
}

func BrclientdGCModifyOwner(ctx context.Context, gcid ShortIDHex, newOwner, reason string) error {
	return brclientdPostJSONID(ctx, "/gc", gcid, "/owner", map[string]any{
		"new_owner": newOwner,
		"reason":    reason,
	})
}

func BrclientdGCUpgrade(ctx context.Context, gcid ShortIDHex, newVersion uint8) error {
	return brclientdPostJSONID(ctx, "/gc", gcid, "/upgrade", map[string]any{"new_version": newVersion})
}

func BrclientdGCAlias(ctx context.Context, gcid ShortIDHex, alias string) error {
	return brclientdPostJSONID(ctx, "/gc", gcid, "/alias", map[string]any{"alias": alias})
}

func BrclientdGCResendList(ctx context.Context, gcid ShortIDHex, uid string) error {
	body := map[string]any{}
	if uid != "" {
		body["uid"] = uid
	}
	return brclientdPostJSONID(ctx, "/gc", gcid, "/resend-list", body)
}

// BrclientdPostsFeed returns the raw JSON body of brclientd's /posts/feed.
// Each entry is a PostSummary; the caller decodes as needed.
func BrclientdPostsFeed(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/posts/feed", nil)
}

// BrclientdPostComments returns the comment status updates for a post.
func BrclientdPostComments(ctx context.Context, uidHex, pidHex string) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/posts/comments", map[string]string{
		"uid": uidHex,
		"pid": pidHex,
	})
}

// BrclientdPostComment publishes a new comment on a remote user's post.
// Returns the comment identifier on success.
func BrclientdPostComment(ctx context.Context, uidHex, pidHex, comment, parent string) (string, error) {
	raw, err := brclientdPostJSONRaw(ctx, "/posts/comment", map[string]string{
		"uid":     uidHex,
		"pid":     pidHex,
		"comment": comment,
		"parent":  parent,
	})
	if err != nil {
		return "", err
	}
	var out struct {
		Identifier string `json:"identifier"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return out.Identifier, nil
}

// BrclientdPostReceiveReceipts returns the receive receipts recorded for one
// of the local user's own posts. Maps to brclientd's GET
// /posts/receivereceipts; posts authored by others return an empty list.
func BrclientdPostReceiveReceipts(ctx context.Context, pidHex string) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/posts/receivereceipts", map[string]string{
		"pid": pidHex,
	})
}

// BrclientdRelayPost relays a known post to one user (toUidHex set) or to
// all of the local client's post subscribers (toUidHex empty).
func BrclientdRelayPost(ctx context.Context, uidHex, pidHex, toUidHex string) error {
	return brclientdPostJSON(ctx, "/posts/relay", map[string]string{
		"uid":    uidHex,
		"pid":    pidHex,
		"to_uid": toUidHex,
	})
}

// BrclientdPostCommentReceipts returns the receive receipts recorded for
// the comments on one of the local user's own posts, grouped by the
// comment's status id. Maps to brclientd's GET /posts/comment-receivereceipts.
func BrclientdPostCommentReceipts(ctx context.Context, pidHex string) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/posts/comment-receivereceipts", map[string]string{
		"pid": pidHex,
	})
}

// BrclientdPostHearts returns the current heart count + whether the local
// identity hearted this post. Maps to brclientd's GET /posts/hearts.
func BrclientdPostHearts(ctx context.Context, uidHex, pidHex string) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/posts/hearts", map[string]string{
		"uid": uidHex,
		"pid": pidHex,
	})
}

// BrclientdPostHeart toggles the local identity's heart on a remote post.
func BrclientdPostHeart(ctx context.Context, uidHex, pidHex string, heart bool) error {
	return brclientdPostJSON(ctx, "/posts/heart", map[string]any{
		"uid":   uidHex,
		"pid":   pidHex,
		"heart": heart,
	})
}

// BrclientdPostBody fetches the full PostMetadata for a single post.
// Returns the raw JSON envelope so the caller can pull out attributes
// (e.g. the markdown body under the "main" key) without taking a hard
// dependency on the BR rpc.PostMetadata type.
func BrclientdPostBody(ctx context.Context, uidHex, pidHex string) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/posts/body", map[string]string{
		"uid": uidHex,
		"pid": pidHex,
	})
}

// brclientdGetRaw issues a GET to brclientd's status server and returns
// the response body as a json.RawMessage. Mirrors brclientdPostJSON but
// for GET-shaped endpoints.
func brclientdGetRaw(ctx context.Context, path brPath, query map[string]string) (json.RawMessage, error) {
	return brclientdGetRawLimit(ctx, path, query, 8<<20)
}

// brclientdGetRawLimit is brclientdGetRaw with an explicit body bound, for
// endpoints whose replies are much smaller or much larger than the default.
func brclientdGetRawLimit(ctx context.Context, path brPath, query map[string]string, limit int64) (json.RawMessage, error) {
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	endpoint, err := brclientdEndpoint(statusPort, path, query)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("brclientd %s: HTTP %d: %s", path, resp.StatusCode, body)
	}
	body, err := readBrclientdBody(resp, path, limit)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

// BrclientdTipUser calls PaymentsService.TipUser on the configured
// brclientd instance. user is a nick or 64-hex identity; dcrAmount is the
// tip amount in DCR; maxAttempts is the per-tip retry budget. BR fires
// OnTipAttemptProgress on the sender side per attempt; we surface the
// terminal outcome via the notifications stream and don't wait for it here.
func BrclientdTipUser(ctx context.Context, user string, dcrAmount float64, maxAttempts int32) error {
	return brclientdPostJSON(ctx, "/tip", map[string]any{
		"user":        user,
		"dcrAmount":   dcrAmount,
		"maxAttempts": maxAttempts,
	})
}

// BrclientdAcceptSuggestion accepts an incoming KX suggestion by asking the
// mediator to introduce us to the target. Wraps brclientd's
// /contacts/accept-suggestion which calls client.RequestMediateIdentity.
func BrclientdAcceptSuggestion(ctx context.Context, mediatorHex, targetHex string) error {
	return brclientdPostJSON(ctx, "/contacts/accept-suggestion", map[string]string{
		"mediator": mediatorHex,
		"target":   targetHex,
	})
}

// BrclientdNotifEvent matches the {type, timestamp, payload} envelope
// brclientd writes to /notifications. payload shape is event-specific.
type BrclientdNotifEvent struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// notifIdleTimeout is how long the /notifications stream may stay silent
// before the connection is presumed dead and redialed. brclientd writes a
// keepalive every 30 seconds, so this allows three missed heartbeats.
const notifIdleTimeout = 90 * time.Second

// BrclientdStreamNotifications opens a long-lived GET against brclientd's
// /notifications JSONL endpoint and invokes onEvent per decoded line,
// keepalives included. Returns when ctx is cancelled or the stream errors.
// Used by the dashboard to forward brclientd-side events (e.g.
// OnKXSuggested) into the existing browser-WS event bus.
func BrclientdStreamNotifications(ctx context.Context, onEvent func(BrclientdNotifEvent)) error {
	cli, err := brclientdStreamClient()
	if err != nil {
		return err
	}
	url, err := brclientdEndpoint(statusPort, "/notifications", nil)
	if err != nil {
		return err
	}
	// The stream client carries no overall timeout, so a silently dead
	// connection would otherwise block Decode forever. The watchdog cancels
	// the request when nothing arrives within notifIdleTimeout; any decoded
	// frame, keepalives included, rearms it.
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	watchdog := time.AfterFunc(notifIdleTimeout, cancel)
	defer watchdog.Stop()
	req, err := http.NewRequestWithContext(sctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("dial brclientd /notifications: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("brclientd /notifications: HTTP %d: %s", resp.StatusCode, body)
	}
	dec := json.NewDecoder(resp.Body)
	for {
		var evt BrclientdNotifEvent
		if err := dec.Decode(&evt); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if sctx.Err() != nil {
				return fmt.Errorf("no traffic on /notifications for %s", notifIdleTimeout)
			}
			return fmt.Errorf("decode notif: %w", err)
		}
		watchdog.Reset(notifIdleTimeout)
		onEvent(evt)
	}
}

// brclientdPostJSON issues a POST with a JSON body to brclientd's status
// server and expects a 204 No Content reply. Used by per-contact action
// endpoints that share the same fire-and-forget shape.
func brclientdPostJSON(ctx context.Context, path brPath, body any) error {
	cli, err := brclientdClient()
	if err != nil {
		return err
	}
	url, err := brclientdEndpoint(statusPort, path, nil)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("brclientd %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("brclientd %s: HTTP %d: %s", path, resp.StatusCode, buf)
	}
	return nil
}

// brclientdPostJSONRaw is the variant of brclientdPostJSON used when the
// endpoint returns a JSON body (e.g. /rtdt/sessions/create returns the new
// session summary). Accepts 200 OK with body.
func brclientdPostJSONRaw(ctx context.Context, path brPath, body any) (json.RawMessage, error) {
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	return brclientdDoPostJSONRaw(ctx, cli, path, body, brclientdControlRespLimit)
}

// readBrclientdBody reads a response body bounded to limit. An oversized
// SUCCESS body is an error rather than a silent truncation, which would surface
// later as a confusing JSON parse failure. An oversized error body is truncated,
// since it only becomes text in an error message.
func readBrclientdBody(resp *http.Response, path brPath, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("brclientd %s: read body: %w", path, err)
	}
	if int64(len(body)) > limit {
		if resp.StatusCode == http.StatusOK {
			return nil, fmt.Errorf("brclientd %s: response exceeds %d bytes", path, limit)
		}
		body = body[:limit]
	}
	return body, nil
}

const (
	// brclientdControlRespLimit bounds the control endpoints, which all return
	// small JSON summaries.
	brclientdControlRespLimit = 1 << 20 // 1 MiB

	// brclientdPageRespLimit bounds a page fetch. Bison Relay only fulfils a
	// resource reply that fits one message payload, 1 MiB on the current
	// protocol version and 10 MiB on the next, so this cannot reject a page a
	// peer could legitimately serve.
	brclientdPageRespLimit = 16 << 20 // 16 MiB
)

// brclientdDoPostJSONRaw issues the POST with a caller-supplied client so the
// caller can pick a timeout policy that fits the endpoint (e.g. the no-deadline
// pages client for an unbounded /pages/fetch transfer). limit bounds the
// response body.
func brclientdDoPostJSONRaw(ctx context.Context, cli *http.Client, path brPath, body any, limit int64) (json.RawMessage, error) {
	url, err := brclientdEndpoint(statusPort, path, nil)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("brclientd %s: HTTP %d: %s", path, resp.StatusCode, buf)
	}
	out, err := readBrclientdBody(resp, path, limit)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BrclientdPagesFetch fetches a single page (resource) from brclientd,
// blocking until the reply lands. body: {uid, path, session_id?,
// parent_page?, data?, async_target_id?}. Returns the raw {session_id,
// page_id, parent_page, status, meta, markdown, async_target_id} JSON.
func BrclientdPagesFetch(ctx context.Context, body any) (json.RawMessage, error) {
	cli, err := brclientdPagesClient()
	if err != nil {
		return nil, err
	}
	return brclientdDoPostJSONRaw(ctx, cli, "/pages/fetch", body, brclientdPageRespLimit)
}

// BrclientdPagesLocalList lists the markdown pages this node hosts.
func BrclientdPagesLocalList(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/pages/local", nil)
}

// BrclientdPagesLocalFile returns the raw markdown of one hosted page.
func BrclientdPagesLocalFile(ctx context.Context, name string) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/pages/local/file", map[string]string{"name": name})
}

// BrclientdPagesLocalSave creates or overwrites one hosted page. body:
// {name, content}.
func BrclientdPagesLocalSave(ctx context.Context, body any) error {
	return brclientdPostJSON(ctx, "/pages/local/save", body)
}

// BrclientdPagesLocalDelete removes one hosted page. body: {name}.
func BrclientdPagesLocalDelete(ctx context.Context, body any) error {
	return brclientdPostJSON(ctx, "/pages/local/delete", body)
}

// BrclientdAcceptInvite hands a previously-shared OOB invite blob to
// brclientd's /invites/accept status endpoint. inviteBytes is base64-encoded.
func BrclientdAcceptInvite(ctx context.Context, inviteBytesB64 string) (json.RawMessage, error) {
	if err := brclientdPostJSON(ctx, "/invites/accept", map[string]any{
		"inviteBytes": inviteBytesB64,
	}); err != nil {
		return nil, err
	}
	return json.RawMessage("{}"), nil
}

// BrclientdHistoryPM reads paginated PM history from brclientd's
// /history/pm endpoint. UID is the hex-encoded zkidentity peer ID. The
// dashboard does not cache messages locally - brclientd's BR clientdb is
// the source of truth and this is a passthrough.
func BrclientdHistoryPM(ctx context.Context, uid string, page, pageSize int) (json.RawMessage, error) {
	return brclientdGetRawLimit(ctx, "/history/pm", map[string]string{
		"uid":       uid,
		"page":      strconv.Itoa(page),
		"page_size": strconv.Itoa(pageSize),
	}, 4<<20)
}

// BrclientdStatus calls brclientd's /status HTTP endpoint over mTLS and
// returns the daemon's JSON verbatim, so a field a newer daemon adds
// survives the proxy. The status server is on a separate port (default
// 7677) from clientrpc; both reuse the same cert triplet.
func BrclientdStatus(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/status", nil)
}

// brclientdBuild returns the cached client, building it lazily so the cert
// pair may appear after dashboard startup. Zero durations mean no bound,
// matching http.Client.
func brclientdBuild(cache **http.Client, timeout, headerTimeout time.Duration) (*http.Client, error) {
	brclientdClientMu.Lock()
	defer brclientdClientMu.Unlock()
	if *cache != nil {
		return *cache, nil
	}
	tlsCfg, err := loadBrclientdTLS(BrclientdCfg)
	if err != nil {
		rpccLog.Warnf("brclientd certs not yet available: %v (will retry on next call)", err)
		return nil, err
	}
	*cache = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:       tlsCfg,
			ResponseHeaderTimeout: headerTimeout,
		},
		Timeout: timeout,
		// brclientd registers both /gc and /gc/, so it never redirects on
		// purpose; ServeMux's path-cleaning 307 replays the method and body at
		// another route. ErrUseLastResponse surfaces it as a non-2xx rather
		// than as a transport error.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return *cache, nil
}

func brclientdClient() (*http.Client, error) {
	// Most calls return well under a second. PaymentsService.TipUser can
	// legitimately take ~10s on the first call after startup and a few seconds
	// per attempt thereafter; 90s is the worst-case ceiling to surface.
	return brclientdBuild(&brclientdHTTPClient, 90*time.Second, 60*time.Second)
}

// brclientdStreamClient is the variant for transfers whose body can outlast
// the 90s ceiling of the shared client (backup tarballs). Only the response
// headers are deadlined; the body streams for as long as it takes.
func brclientdStreamClient() (*http.Client, error) {
	return brclientdBuild(&brclientdStreamHTTPClient, 0, 60*time.Second)
}

// brclientdBackupClient is the variant for /backup: brclientd builds the
// complete tarball before sending response headers, so the header deadline
// must cover the whole build (up to 5 GiB of state), not just a roundtrip.
// Callers bound total time through the request context instead.
func brclientdBackupClient() (*http.Client, error) {
	return brclientdBuild(&brclientdBackupHTTPClient, 0, 15*time.Minute)
}

// brclientdPagesClient is the variant for /pages/fetch. A page fetched over the
// relay has unbounded transfer size and time, and brclientd buffers the whole
// reply before sending headers, so neither an overall timeout nor a
// response-header deadline can apply without cutting off a legitimate transfer.
// Total time is bounded by the request context (the originating connection)
// instead, so a navigated-away fetch is cancelled rather than timed out.
func brclientdPagesClient() (*http.Client, error) {
	return brclientdBuild(&brclientdPagesHTTPClient, 0, 0)
}

// BrclientdRTDTAudioDial returns the pinned mTLS chain and the complete wss://
// URL for one session's audio socket, so the RTDT audio proxy can bridge
// browser <-> brclientd frames without re-implementing cert pinning. Returns a
// finished URL rather than a base, so the caller has nothing to append to.
func BrclientdRTDTAudioDial(rv ShortIDHex) (*tls.Config, string, error) {
	path, err := brclientdRoute("/rtdt/sessions", rv, "/audio")
	if err != nil {
		return nil, "", err
	}
	endpoint, err := brclientdWSEndpoint(statusPort, path)
	if err != nil {
		return nil, "", err
	}
	tlsCfg, err := loadBrclientdTLS(brclientdConfig())
	if err != nil {
		return nil, "", err
	}
	return tlsCfg, endpoint, nil
}

func loadBrclientdTLS(cfg BrclientdConfig) (*tls.Config, error) {
	serverPEM, err := os.ReadFile(cfg.ServerCertPath)
	if err != nil {
		return nil, fmt.Errorf("read brclientd server cert: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(serverPEM) {
		return nil, fmt.Errorf("parse brclientd server cert at %s", cfg.ServerCertPath)
	}
	clientCert, err := tls.LoadX509KeyPair(cfg.ClientCertPath, cfg.ClientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load brclientd client cert: %w", err)
	}
	// brclientd's server cert SANs are localhost + 127.0.0.1 + the
	// container's auto-generated hostname. The dashboard dials by service
	// name (e.g. "brclientd"), so we authenticate the peer leaf against the
	// pinned pool via VerifyPeerCertificate (see tlspin.go) and skip only the
	// hostname check, matching the dcrlnd pattern. InsecureSkipVerify disables
	// only Go's built-in chain+hostname check; the callback still authenticates.
	return &tls.Config{
		RootCAs:               pool,
		Certificates:          []tls.Certificate{clientCert},
		MinVersion:            tls.VersionTLS12,
		InsecureSkipVerify:    true,
		VerifyPeerCertificate: pinnedLeafVerifier(pool),
	}, nil
}

// --- BR-MCP client bridge (proxied to brclientd's mcpclient endpoints) ---

// BrclientdMCPSettings fetches the BR-MCP client settings.
func BrclientdMCPSettings(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/settings/mcpclient", nil)
}

// BrclientdMCPApplySettings persists new BR-MCP client settings and returns
// the applied state.
func BrclientdMCPApplySettings(ctx context.Context, body any) (json.RawMessage, error) {
	return brclientdPostJSONRaw(ctx, "/settings/mcpclient", body)
}

// BrclientdMCPPending lists payments awaiting user approval.
func BrclientdMCPPending(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/mcp/pending", nil)
}

// BrclientdMCPResolvePending approves or denies one pending payment.
func BrclientdMCPResolvePending(ctx context.Context, id string, approve bool) error {
	return brclientdPostJSON(ctx, "/mcp/pending/resolve", map[string]any{
		"id": id, "approve": approve,
	})
}

// BrclientdMCPSpend returns the BR-MCP spend log and rolling-day total.
func BrclientdMCPSpend(ctx context.Context) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/mcp/spend", nil)
}
