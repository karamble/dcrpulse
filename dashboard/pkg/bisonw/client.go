// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package bisonw is a dependency-free Go client for the DCRDEX "bisonw" client
// daemon RPC server (the same API the bwctl tool speaks). It exchanges the
// msgjson request/response envelope over HTTPS with HTTP Basic authentication.
//
// The package intentionally has no third-party dependencies (standard library
// only) so it can be reused outside dcrpulse. The msgjson envelope and the
// response payload shape are mirrored locally to match the wire format of
// decred.org/dcrdex/dex/msgjson.
package bisonw

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
)

// DefaultRPCPort is bisonw's mainnet RPC server port.
const DefaultRPCPort = "5757"

// msgjson message types (decred.org/dcrdex/dex/msgjson): Request = 1, Response = 2.
const (
	msgTypeRequest  = 1
	msgTypeResponse = 2
)

// Config holds the connection settings for a bisonw RPC server.
type Config struct {
	// Addr is the RPC server address as host:port.
	Addr string
	// User and Pass are bisonw's rpcuser/rpcpass (HTTP Basic auth).
	User string
	Pass string
	// Cert is the PEM-encoded RPC TLS certificate. CertPath, if set, takes
	// precedence and is read from disk.
	Cert     []byte
	CertPath string
	// ServerName overrides the TLS server name used to verify the RPC
	// certificate. If empty, the host portion of Addr is used. bisonw's
	// auto-generated cert lists its os.Hostname (e.g. the docker service name)
	// plus its interface IPs, so this normally matches Addr's host.
	ServerName string
}

// Client is a bisonw RPC client. It is safe for concurrent use.
type Client struct {
	cfg       Config
	url       string
	http      *http.Client
	tlsConfig *tls.Config
	nextID    uint64
}

// wireMessage mirrors msgjson.Message on the wire.
type wireMessage struct {
	Type    int             `json:"type"`
	Route   string          `json:"route,omitempty"`
	ID      uint64          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Sig     string          `json:"sig"`
}

// rawParams mirrors rpcserver.RawParams. bisonw's RPC routes take positional
// string arguments (Args) plus separate password arguments (PWArgs); the server
// requires this envelope (it is not typed JSON params).
type rawParams struct {
	PWArgs []string `json:"PWArgs"`
	Args   []string `json:"args"`
}

// responsePayload mirrors msgjson.ResponsePayload (the decoded payload of a
// response-type message).
type responsePayload struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

// RPCError is a structured error returned by the bisonw RPC server.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("bisonw rpc error %d: %s", e.Code, e.Message)
}

// New constructs a Client from cfg, loading and trusting the RPC TLS cert.
func New(cfg Config) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("bisonw: Addr is required")
	}
	pem := cfg.Cert
	if cfg.CertPath != "" {
		b, err := os.ReadFile(cfg.CertPath)
		if err != nil {
			return nil, fmt.Errorf("bisonw: read cert %q: %w", cfg.CertPath, err)
		}
		pem = b
	}
	if len(pem) == 0 {
		return nil, fmt.Errorf("bisonw: RPC TLS cert is required")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("bisonw: invalid RPC TLS cert")
	}
	u, err := url.Parse("https://" + cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("bisonw: bad Addr %q: %w", cfg.Addr, err)
	}
	serverName := cfg.ServerName
	if serverName == "" {
		serverName = u.Hostname()
	}
	tlsConfig := &tls.Config{RootCAs: pool, ServerName: serverName}
	return &Client{
		cfg:       cfg,
		url:       "https://" + cfg.Addr,
		tlsConfig: tlsConfig,
		http: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsConfig},
		},
	}, nil
}

// WSDialInfo returns the parameters for dialing bisonw's /ws endpoint: the
// pinned TLS config, the wss:// URL, and the HTTP Basic auth header value. The
// caller supplies its own WebSocket dialer so this package stays free of a
// WebSocket dependency.
func (c *Client) WSDialInfo() (tlsConfig *tls.Config, wsURL, basicAuth string) {
	auth := base64.StdEncoding.EncodeToString([]byte(c.cfg.User + ":" + c.cfg.Pass))
	return c.tlsConfig, "wss://" + c.cfg.Addr + "/ws", "Basic " + auth
}

// Call invokes an RPC route with the given password arguments and positional
// string arguments (either may be nil) and, if result is non-nil, JSON-
// unmarshals the successful result payload into it. A non-nil *RPCError is
// returned when the server reports an application error.
func (c *Client) Call(ctx context.Context, route string, pwArgs, args []string, result any) error {
	if pwArgs == nil {
		pwArgs = []string{}
	}
	if args == nil {
		args = []string{}
	}
	payload, err := json.Marshal(rawParams{PWArgs: pwArgs, Args: args})
	if err != nil {
		return fmt.Errorf("bisonw: marshal params for %q: %w", route, err)
	}
	body, err := json.Marshal(wireMessage{
		Type:    msgTypeRequest,
		Route:   route,
		ID:      atomic.AddUint64(&c.nextID, 1),
		Payload: json.RawMessage(payload),
	})
	if err != nil {
		return fmt.Errorf("bisonw: marshal request for %q: %w", route, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Close = true
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.cfg.User, c.cfg.Pass)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("bisonw: %s: %w", route, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("bisonw: %s: read response: %w", route, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(respBytes) == 0 {
			return fmt.Errorf("bisonw: %s: http %d", route, resp.StatusCode)
		}
		return fmt.Errorf("bisonw: %s: http %d: %s", route, resp.StatusCode, bytes.TrimSpace(respBytes))
	}

	var respMsg wireMessage
	if err := json.Unmarshal(respBytes, &respMsg); err != nil {
		return fmt.Errorf("bisonw: %s: decode response envelope: %w", route, err)
	}
	if respMsg.Type != msgTypeResponse {
		return fmt.Errorf("bisonw: %s: unexpected response type %d", route, respMsg.Type)
	}
	var rp responsePayload
	if err := json.Unmarshal(respMsg.Payload, &rp); err != nil {
		return fmt.Errorf("bisonw: %s: decode response payload: %w", route, err)
	}
	if rp.Error != nil {
		return rp.Error
	}
	if result != nil && len(rp.Result) > 0 {
		if err := json.Unmarshal(rp.Result, result); err != nil {
			return fmt.Errorf("bisonw: %s: decode result: %w", route, err)
		}
	}
	return nil
}
