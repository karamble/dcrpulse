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
	"errors"
	"fmt"
	"io"
	"log"
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
	brclientdClientMu         sync.Mutex
)

// InitBrclientdConfig records the brclientd clientrpc connection settings.
// The HTTP client is built lazily on the first call so the dashboard can
// start before brclientd has issued its cert pair.
func InitBrclientdConfig(cfg BrclientdConfig) {
	brclientdClientMu.Lock()
	defer brclientdClientMu.Unlock()
	BrclientdCfg = cfg
	brclientdHTTPClient = nil
	brclientdStreamHTTPClient = nil
	brclientdBackupHTTPClient = nil
}

// UpdateBrclientdCerts repoints brclientd at a different wallet's identity certs
// and forces the HTTP client to rebuild on next use. Used on a wallet switch.
func UpdateBrclientdCerts(serverCertPath, clientCertPath, clientKeyPath string) {
	brclientdClientMu.Lock()
	defer brclientdClientMu.Unlock()
	BrclientdCfg.ServerCertPath = serverCertPath
	BrclientdCfg.ClientCertPath = clientCertPath
	BrclientdCfg.ClientKeyPath = clientKeyPath
	brclientdHTTPClient = nil
	brclientdStreamHTTPClient = nil
	brclientdBackupHTTPClient = nil
}

// BrclientdVersionResult is the wire shape returned by VersionService.Version.
type BrclientdVersionResult struct {
	AppName    string `json:"appName"`
	AppVersion string `json:"appVersion"`
	GoRuntime  string `json:"goRuntime"`
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

// BrclientdStatusResult is the wire shape served by brclientd's /status
// endpoint. Mirrors the JSON the daemon writes verbatim so the dashboard's
// /api/br/status handler can pass it through.
type BrclientdStatusResult struct {
	Stage           string `json:"stage"`
	Nick            string `json:"nick,omitempty"`
	ServerNode      string `json:"serverNode,omitempty"`
	RecommendedPeer string `json:"recommendedPeer,omitempty"`
	WalletCheckErr  string `json:"walletCheckErr,omitempty"`
	LastUpdated     string `json:"lastUpdated"`
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
	if BrclientdCfg.Host == "" || BrclientdCfg.Port == "" {
		return errors.New("brclientd: host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/create-identity", BrclientdCfg.Host, BrclientdCfg.Port)
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

// BrclientdSendFile uploads a file to brclientd's /files/send mTLS endpoint,
// which persists the bytes under UploadDir and dispatches them to BR's
// SendFile RPC. user can be a nick / alias / hex UID.
func BrclientdSendFile(ctx context.Context, user, filename, mime string, body io.Reader) (*BrclientdSendFileResult, error) {
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return nil, errors.New("brclientd: status host/port not configured")
	}

	pr, pw := io.Pipe()
	mp := multipart.NewWriter(pw)
	go func() {
		defer pw.Close()
		defer mp.Close()
		if err := mp.WriteField("user", user); err != nil {
			pw.CloseWithError(err)
			return
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

	url := fmt.Sprintf("https://%s:%s/files/send", BrclientdCfg.Host, BrclientdCfg.StatusPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		return nil, fmt.Errorf("build send-file request: %w", err)
	}
	req.Header.Set("Content-Type", mp.FormDataContentType())
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd /files/send: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read send-file response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brclientd /files/send: HTTP %d: %s", resp.StatusCode, respBody)
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
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return nil, errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/contacts", BrclientdCfg.Host, BrclientdCfg.StatusPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build contacts request: %w", err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd /contacts: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read contacts: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brclientd /contacts: HTTP %d: %s", resp.StatusCode, body)
	}
	return body, nil
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
	cli, err := brclientdClient()
	if err != nil {
		return err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/invites/redeem-key", BrclientdCfg.Host, BrclientdCfg.StatusPort)
	payload, _ := json.Marshal(map[string]string{"key": key})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("brclientd /invites/redeem-key: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("brclientd /invites/redeem-key: HTTP %d: %s", resp.StatusCode, body)
	}
	return nil
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

// BrclientdClearPMHistory permanently deletes the local PM history (and inline
// media) for a contact. Wraps brclientd's /history/pm/clear, which removes the
// on-disk message log(s) + embeds for the uid. The contact and ratchet remain;
// only the local copy is wiped. Irreversible.
func BrclientdClearPMHistory(ctx context.Context, uidHex string) error {
	return brclientdPostJSON(ctx, "/history/pm/clear", map[string]string{"uid": uidHex})
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
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return nil, errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/posts/new", BrclientdCfg.Host, BrclientdCfg.StatusPort)
	payload, err := json.Marshal(map[string]string{
		"post":  post,
		"descr": descr,
	})
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
		return nil, fmt.Errorf("brclientd /posts/new: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("brclientd /posts/new: HTTP %d: %s", resp.StatusCode, body)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return json.RawMessage(body), nil
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
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return nil, errors.New("brclientd: status host/port not configured")
	}
	pr, pw := io.Pipe()
	mp := multipart.NewWriter(pw)
	go func() {
		defer pw.Close()
		defer mp.Close()
		_ = mp.WriteField("cost_atoms", fmt.Sprintf("%d", costAtoms))
		if targetUIDHex != "" {
			_ = mp.WriteField("target_uid", targetUIDHex)
		}
		if descr != "" {
			_ = mp.WriteField("descr", descr)
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
	url := fmt.Sprintf("https://%s:%s/shared-files/add", BrclientdCfg.Host, BrclientdCfg.StatusPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		return nil, fmt.Errorf("build share-file request: %w", err)
	}
	req.Header.Set("Content-Type", mp.FormDataContentType())
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd /shared-files/add: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read share-file response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brclientd /shared-files/add: HTTP %d: %s", resp.StatusCode, respBody)
	}
	return json.RawMessage(respBody), nil
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

// BrclientdContentGet asks brclientd to start downloading a shared file (FID)
// from a remote user, as advertised by an --embed[download=<fid>,cost=,...]--
// tag. BR auto-pays any per-chunk cost the uploader set; progress is tracked
// via BrclientdListDownloads and the file-download-* events.
func BrclientdContentGet(ctx context.Context, uidHex, fidHex string) error {
	return brclientdPostJSON(ctx, "/content/get", map[string]string{
		"uid": uidHex,
		"fid": fidHex,
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
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return nil, errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/content/file?fid=%s", BrclientdCfg.Host, BrclientdCfg.StatusPort, fidHex)
	if uidHex != "" {
		url += "&uid=" + uidHex
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd /content/file: %w", err)
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
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return nil, errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/backup", BrclientdCfg.Host, BrclientdCfg.StatusPort)
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
	if BrclientdCfg.Host == "" || BrclientdCfg.Port == "" {
		return errors.New("brclientd: host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/restore-backup", BrclientdCfg.Host, BrclientdCfg.Port)
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
func BrclientdUploadStoreFile(ctx context.Context, relPath, filename, mime string, body io.Reader) (json.RawMessage, error) {
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return nil, errors.New("brclientd: status host/port not configured")
	}
	pr, pw := io.Pipe()
	mp := multipart.NewWriter(pw)
	go func() {
		defer pw.Close()
		defer mp.Close()
		if relPath != "" {
			_ = mp.WriteField("path", relPath)
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
	url := fmt.Sprintf("https://%s:%s/store/files/upload", BrclientdCfg.Host, BrclientdCfg.StatusPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		return nil, fmt.Errorf("build upload request: %w", err)
	}
	req.Header.Set("Content-Type", mp.FormDataContentType())
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd /store/files/upload: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brclientd /store/files/upload: HTTP %d: %s", resp.StatusCode, respBody)
	}
	return json.RawMessage(respBody), nil
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
func BrclientdRTDTInvite(ctx context.Context, rv string, uids []string, asPublisher bool) error {
	return brclientdPostJSON(ctx, "/rtdt/sessions/"+rv+"/invite", map[string]any{
		"uids":         uids,
		"as_publisher": asPublisher,
	})
}

// BrclientdRTDTAccept accepts a pending invite to an RTDT session.
func BrclientdRTDTAccept(ctx context.Context, rv, inviter string, asPublisher bool) error {
	return brclientdPostJSON(ctx, "/rtdt/sessions/"+rv+"/accept", map[string]any{
		"inviter":      inviter,
		"as_publisher": asPublisher,
	})
}

// BrclientdRTDTJoin connects the live UDP audio for an accepted session.
func BrclientdRTDTJoin(ctx context.Context, rv string) error {
	return brclientdPostJSON(ctx, "/rtdt/sessions/"+rv+"/join", map[string]any{})
}

// BrclientdRTDTLeave leaves an RTDT session (member action).
func BrclientdRTDTLeave(ctx context.Context, rv string) error {
	return brclientdPostJSON(ctx, "/rtdt/sessions/"+rv+"/leave", map[string]any{})
}

// BrclientdRTDTDissolve tears down an RTDT session (owner only).
func BrclientdRTDTDissolve(ctx context.Context, rv string) error {
	return brclientdPostJSON(ctx, "/rtdt/sessions/"+rv+"/dissolve", map[string]any{})
}

// BrclientdRTDTKick removes a peer from the live audio session.
func BrclientdRTDTKick(ctx context.Context, rv string, peerID uint32, banSeconds int64) error {
	return brclientdPostJSON(ctx, "/rtdt/sessions/"+rv+"/kick", map[string]any{
		"peer_id":     peerID,
		"ban_seconds": banSeconds,
	})
}

// BrclientdRTDTRemove removes a member from the session metadata.
func BrclientdRTDTRemove(ctx context.Context, rv, uid, reason string) error {
	return brclientdPostJSON(ctx, "/rtdt/sessions/"+rv+"/remove", map[string]any{
		"uid":    uid,
		"reason": reason,
	})
}

// BrclientdRTDTRotateCookies invalidates current appointment cookies.
func BrclientdRTDTRotateCookies(ctx context.Context, rv string) error {
	return brclientdPostJSON(ctx, "/rtdt/sessions/"+rv+"/rotate-cookies", map[string]any{})
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

func BrclientdGCDetail(ctx context.Context, gcid string) (json.RawMessage, error) {
	return brclientdGetRaw(ctx, "/gc/"+gcid, nil)
}

func BrclientdGCInvite(ctx context.Context, gcid, uid string) error {
	return brclientdPostJSON(ctx, "/gc/"+gcid+"/invite", map[string]any{"uid": uid})
}

func BrclientdGCMessage(ctx context.Context, gcid, message string, mode int) error {
	return brclientdPostJSON(ctx, "/gc/"+gcid+"/message", map[string]any{
		"message": message,
		"mode":    mode,
	})
}

func BrclientdGCHistory(ctx context.Context, gcid string, page, pageSize int) (json.RawMessage, error) {
	q := map[string]string{}
	if page > 0 {
		q["page"] = strconv.Itoa(page)
	}
	if pageSize > 0 {
		q["page_size"] = strconv.Itoa(pageSize)
	}
	return brclientdGetRaw(ctx, "/gc/"+gcid+"/history", q)
}

func BrclientdGCPart(ctx context.Context, gcid, reason string) error {
	return brclientdPostJSON(ctx, "/gc/"+gcid+"/part", map[string]any{"reason": reason})
}

func BrclientdGCKill(ctx context.Context, gcid, reason string) error {
	return brclientdPostJSON(ctx, "/gc/"+gcid+"/kill", map[string]any{"reason": reason})
}

func BrclientdGCKick(ctx context.Context, gcid, uid, reason string) error {
	return brclientdPostJSON(ctx, "/gc/"+gcid+"/kick", map[string]any{
		"uid":    uid,
		"reason": reason,
	})
}

func BrclientdGCBlock(ctx context.Context, gcid, uid string) error {
	return brclientdPostJSON(ctx, "/gc/"+gcid+"/block", map[string]any{"uid": uid})
}

func BrclientdGCUnblock(ctx context.Context, gcid, uid string) error {
	return brclientdPostJSON(ctx, "/gc/"+gcid+"/unblock", map[string]any{"uid": uid})
}

func BrclientdGCModifyAdmins(ctx context.Context, gcid string, extraAdmins []string, reason string) error {
	return brclientdPostJSON(ctx, "/gc/"+gcid+"/admins", map[string]any{
		"extra_admins": extraAdmins,
		"reason":       reason,
	})
}

func BrclientdGCModifyOwner(ctx context.Context, gcid, newOwner, reason string) error {
	return brclientdPostJSON(ctx, "/gc/"+gcid+"/owner", map[string]any{
		"new_owner": newOwner,
		"reason":    reason,
	})
}

func BrclientdGCUpgrade(ctx context.Context, gcid string, newVersion uint8) error {
	return brclientdPostJSON(ctx, "/gc/"+gcid+"/upgrade", map[string]any{"new_version": newVersion})
}

func BrclientdGCAlias(ctx context.Context, gcid, alias string) error {
	return brclientdPostJSON(ctx, "/gc/"+gcid+"/alias", map[string]any{"alias": alias})
}

func BrclientdGCResendList(ctx context.Context, gcid, uid string) error {
	body := map[string]any{}
	if uid != "" {
		body["uid"] = uid
	}
	return brclientdPostJSON(ctx, "/gc/"+gcid+"/resend-list", body)
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
	cli, err := brclientdClient()
	if err != nil {
		return "", err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return "", errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/posts/comment", BrclientdCfg.Host, BrclientdCfg.StatusPort)
	payload, err := json.Marshal(map[string]string{
		"uid":     uidHex,
		"pid":     pidHex,
		"comment": comment,
		"parent":  parent,
	})
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("brclientd /posts/comment: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", fmt.Errorf("brclientd /posts/comment: HTTP %d: %s", resp.StatusCode, body)
	}
	var out struct {
		Identifier string `json:"identifier"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return out.Identifier, nil
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
	cli, err := brclientdClient()
	if err != nil {
		return err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/posts/heart", BrclientdCfg.Host, BrclientdCfg.StatusPort)
	payload, err := json.Marshal(map[string]any{
		"uid":   uidHex,
		"pid":   pidHex,
		"heart": heart,
	})
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
		return fmt.Errorf("brclientd /posts/heart: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("brclientd /posts/heart: HTTP %d: %s", resp.StatusCode, body)
	}
	return nil
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
func brclientdGetRaw(ctx context.Context, path string, query map[string]string) (json.RawMessage, error) {
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return nil, errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s%s", BrclientdCfg.Host, BrclientdCfg.StatusPort, path)
	if len(query) > 0 {
		sep := "?"
		for k, v := range query {
			url += sep + k + "=" + v
			sep = "&"
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
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

// BrclientdStreamNotifications opens a long-lived GET against brclientd's
// /notifications JSONL endpoint and invokes onEvent per decoded line.
// Returns when ctx is cancelled or the stream errors. Used by the dashboard
// to forward brclientd-side events (e.g. OnKXSuggested) into the existing
// browser-WS event bus.
func BrclientdStreamNotifications(ctx context.Context, onEvent func(BrclientdNotifEvent)) error {
	cli, err := brclientdClient()
	if err != nil {
		return err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/notifications", BrclientdCfg.Host, BrclientdCfg.StatusPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
			return fmt.Errorf("decode notif: %w", err)
		}
		onEvent(evt)
	}
}

// brclientdPostJSON issues a POST with a JSON body to brclientd's status
// server and expects a 204 No Content reply. Used by per-contact action
// endpoints that share the same fire-and-forget shape.
func brclientdPostJSON(ctx context.Context, path string, body any) error {
	cli, err := brclientdClient()
	if err != nil {
		return err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s%s", BrclientdCfg.Host, BrclientdCfg.StatusPort, path)
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
func brclientdPostJSONRaw(ctx context.Context, path string, body any) (json.RawMessage, error) {
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return nil, errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s%s", BrclientdCfg.Host, BrclientdCfg.StatusPort, path)
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
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("brclientd %s: read body: %w", path, err)
	}
	return out, nil
}

// BrclientdPagesFetch fetches a single page (resource) from brclientd,
// blocking until the reply lands. body: {uid, path, session_id?,
// parent_page?, data?, async_target_id?}. Returns the raw {session_id,
// page_id, parent_page, status, meta, markdown, async_target_id} JSON.
func BrclientdPagesFetch(ctx context.Context, body any) (json.RawMessage, error) {
	return brclientdPostJSONRaw(ctx, "/pages/fetch", body)
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
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return nil, errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/history/pm?uid=%s&page=%d&page_size=%d",
		BrclientdCfg.Host, BrclientdCfg.StatusPort, uid, page, pageSize)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build history request: %w", err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd /history/pm: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brclientd /history/pm: HTTP %d: %s", resp.StatusCode, body)
	}
	return body, nil
}

// BrclientdStatus calls brclientd's /status HTTP endpoint over mTLS and
// returns the parsed snapshot. The status server is on a separate port
// (default 7677) from clientrpc; both reuse the same cert triplet.
func BrclientdStatus(ctx context.Context) (*BrclientdStatusResult, error) {
	cli, err := brclientdClient()
	if err != nil {
		return nil, err
	}
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return nil, errors.New("brclientd: status host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/status", BrclientdCfg.Host, BrclientdCfg.StatusPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build status request: %w", err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brclientd /status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("brclientd /status: HTTP %d: %s", resp.StatusCode, body)
	}
	var result BrclientdStatusResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}
	return &result, nil
}

// brclientdClient returns the cached HTTP client, building it lazily on the
// first call. Rebuilt on demand if the cert pair appears after dashboard
// startup, mirroring the dcrlnd pattern.
func brclientdClient() (*http.Client, error) {
	brclientdClientMu.Lock()
	defer brclientdClientMu.Unlock()
	if brclientdHTTPClient != nil {
		return brclientdHTTPClient, nil
	}
	tlsCfg, err := loadBrclientdTLS(BrclientdCfg)
	if err != nil {
		log.Printf("brclientd certs not yet available: %v (will retry on next call)", err)
		return nil, err
	}
	brclientdHTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:       tlsCfg,
			ResponseHeaderTimeout: 60 * time.Second,
		},
		// Most calls return well under a second. PaymentsService.TipUser
		// can legitimately take ~10s on the first call after startup
		// (BR waits for tipAttemptsRunning) and a few seconds on each
		// attempt thereafter while it fetches+pays an invoice. 90s is
		// the worst-case ceiling we want to surface to the user.
		Timeout: 90 * time.Second,
	}
	return brclientdHTTPClient, nil
}

// brclientdStreamClient is the variant for transfers whose body can outlast
// the 90s ceiling of the shared client (backup tarballs). Only the response
// headers are deadlined; the body streams for as long as it takes.
func brclientdStreamClient() (*http.Client, error) {
	brclientdClientMu.Lock()
	defer brclientdClientMu.Unlock()
	if brclientdStreamHTTPClient != nil {
		return brclientdStreamHTTPClient, nil
	}
	tlsCfg, err := loadBrclientdTLS(BrclientdCfg)
	if err != nil {
		log.Printf("brclientd certs not yet available: %v (will retry on next call)", err)
		return nil, err
	}
	brclientdStreamHTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:       tlsCfg,
			ResponseHeaderTimeout: 60 * time.Second,
		},
	}
	return brclientdStreamHTTPClient, nil
}

// brclientdBackupClient is the variant for /backup: brclientd builds the
// complete tarball before sending response headers, so the header deadline
// must cover the whole build (up to 5 GiB of state), not just a roundtrip.
// Callers bound total time through the request context instead.
func brclientdBackupClient() (*http.Client, error) {
	brclientdClientMu.Lock()
	defer brclientdClientMu.Unlock()
	if brclientdBackupHTTPClient != nil {
		return brclientdBackupHTTPClient, nil
	}
	tlsCfg, err := loadBrclientdTLS(BrclientdCfg)
	if err != nil {
		log.Printf("brclientd certs not yet available: %v (will retry on next call)", err)
		return nil, err
	}
	brclientdBackupHTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:       tlsCfg,
			ResponseHeaderTimeout: 15 * time.Minute,
		},
	}
	return brclientdBackupHTTPClient, nil
}

// BrclientdWSDialer returns a gorilla-websocket-compatible dialer plus the
// base wss:// URL for brclientd's status port, both preconfigured with the
// pinned mTLS chain. The RTDT audio proxy uses this to bridge browser <->
// brclientd binary frames without re-implementing cert pinning.
func BrclientdWSDialer() (tlsCfg *tls.Config, baseURL string, err error) {
	if BrclientdCfg.Host == "" || BrclientdCfg.StatusPort == "" {
		return nil, "", errors.New("brclientd: status host/port not configured")
	}
	tlsCfg, err = loadBrclientdTLS(BrclientdCfg)
	if err != nil {
		return nil, "", err
	}
	baseURL = fmt.Sprintf("wss://%s:%s", BrclientdCfg.Host, BrclientdCfg.StatusPort)
	return tlsCfg, baseURL, nil
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
	// name (e.g. "brclientd") so we authenticate via the pinned pool and
	// skip hostname verification, matching the dcrlnd pattern.
	return &tls.Config{
		RootCAs:            pool,
		Certificates:       []tls.Certificate{clientCert},
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}, nil
}
