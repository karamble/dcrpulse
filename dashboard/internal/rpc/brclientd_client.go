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
	brclientdClientMu   sync.Mutex
)

// InitBrclientdConfig records the brclientd clientrpc connection settings.
// The HTTP client is built lazily on the first call so the dashboard can
// start before brclientd has issued its cert pair.
func InitBrclientdConfig(cfg BrclientdConfig) {
	brclientdClientMu.Lock()
	defer brclientdClientMu.Unlock()
	BrclientdCfg = cfg
	brclientdHTTPClient = nil
}

// BrclientdVersionResult is the wire shape returned by VersionService.Version.
type BrclientdVersionResult struct {
	AppName    string `json:"appName"`
	AppVersion string `json:"appVersion"`
	GoRuntime  string `json:"goRuntime"`
}

// BrclientdVersion calls VersionService.Version on the configured brclientd
// instance and returns the appName / appVersion / goRuntime triple.
func BrclientdVersion(ctx context.Context) (*BrclientdVersionResult, error) {
	var result BrclientdVersionResult
	if err := brclientdCall(ctx, "VersionService.Version", struct{}{}, &result); err != nil {
		return nil, err
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

// BrclientdUserPublicIdentity calls ChatService.UserPublicIdentity over
// clientrpc and returns the raw JSON. Used by the dashboard to confirm
// the BR client core is operational and to render the local user's
// pubkey + nick on the BR overview.
func BrclientdUserPublicIdentity(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := brclientdCall(ctx, "ChatService.UserPublicIdentity", struct{}{}, &raw); err != nil {
		return nil, err
	}
	return raw, nil
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

// BrclientdSendPM sends a private message through ChatService.PM. `user`
// can be a nick, alias, or hex peer UID.
func BrclientdSendPM(ctx context.Context, user, msg string) error {
	params := map[string]any{
		"user": user,
		"msg":  map[string]any{"message": msg},
	}
	var unused json.RawMessage
	return brclientdCall(ctx, "ChatService.PM", params, &unused)
}

// BrclientdInviteResult bundles the two share-forms BR's WriteNewInvite
// produces: the binary OOB invite blob (base64 over the wire) and the
// bech32 brpik1 key that points at the same prepaid invite on the BR
// server. Sharing either gets a peer the same KX outcome.
type BrclientdInviteResult struct {
	InviteBytes string
	InviteKey   string
}

// BrclientdWriteNewInvite creates an OOB invite via ChatService.WriteNewInvite
// and returns both share-forms.
func BrclientdWriteNewInvite(ctx context.Context) (*BrclientdInviteResult, error) {
	var resp struct {
		InviteBytes string `json:"inviteBytes"`
		InviteKey   string `json:"inviteKey"`
	}
	if err := brclientdCall(ctx, "ChatService.WriteNewInvite", struct{}{}, &resp); err != nil {
		return nil, err
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

// BrclientdHandshake starts a 3-way handshake with the specified contact.
// Wraps brclientd's /contacts/handshake which calls client.Handshake.
func BrclientdHandshake(ctx context.Context, uidHex string) error {
	return brclientdPostJSON(ctx, "/contacts/handshake", map[string]string{"uid": uidHex})
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

// BrclientdTipUser calls PaymentsService.TipUser on the configured
// brclientd instance. user is a nick or 64-hex identity; dcrAmount is the
// tip amount in DCR; maxAttempts is the per-tip retry budget. BR fires
// OnTipAttemptProgress on the sender side per attempt; we surface the
// terminal outcome via the notifications stream and don't wait for it here.
func BrclientdTipUser(ctx context.Context, user string, dcrAmount float64, maxAttempts int32) error {
	params := map[string]any{
		"user":         user,
		"dcr_amount":   dcrAmount,
		"max_attempts": maxAttempts,
	}
	var unused struct{}
	return brclientdCall(ctx, "PaymentsService.TipUser", params, &unused)
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

// BrclientdAcceptInvite hands a previously-shared OOB invite blob to
// ChatService.AcceptInvite. inviteBytes is base64-encoded.
func BrclientdAcceptInvite(ctx context.Context, inviteBytesB64 string) (json.RawMessage, error) {
	params := map[string]any{"inviteBytes": inviteBytesB64}
	var raw json.RawMessage
	if err := brclientdCall(ctx, "ChatService.AcceptInvite", params, &raw); err != nil {
		return nil, err
	}
	return raw, nil
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

func brclientdCall(ctx context.Context, method string, params, result any) error {
	cli, err := brclientdClient()
	if err != nil {
		return err
	}

	if BrclientdCfg.Host == "" || BrclientdCfg.Port == "" {
		return errors.New("brclientd: host/port not configured")
	}
	url := fmt.Sprintf("https://%s:%s/index", BrclientdCfg.Host, BrclientdCfg.Port)

	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  method,
		"params":  params,
	})
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
		return fmt.Errorf("brclientd %s: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("brclientd %s: HTTP %d: %s", method, resp.StatusCode, body)
	}

	// brclientd's /index endpoint emits the JSON-RPC response followed by a
	// trailing close-frame ("Forbidden\n"), so we read exactly one JSON
	// value from the stream instead of buffering the whole body.
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("brclientd %s: code %d: %s", method, rpcResp.Error.Code, rpcResp.Error.Message)
	}
	if result != nil && len(rpcResp.Result) > 0 {
		if err := json.Unmarshal(rpcResp.Result, result); err != nil {
			return fmt.Errorf("decode result: %w", err)
		}
	}
	return nil
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
