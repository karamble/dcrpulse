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
	"net/http"
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
			ResponseHeaderTimeout: 10 * time.Second,
		},
		Timeout: 15 * time.Second,
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
