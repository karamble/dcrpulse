// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// rpcClient is a minimal dcrd JSON-RPC client. It is stateless: an Umbrel widget
// refresh is what triggers new data, so every request makes fresh calls and
// nothing is cached.
type rpcClient struct {
	url  string
	auth string
	http *http.Client
}

func newRPCClient(host, port, user, pass, certPath string) (*rpcClient, error) {
	pem, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read rpc cert %s: %w", certPath, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("parse rpc cert %s", certPath)
	}
	// Pin dcrd's self-signed cert via the loaded pool. The hostname is not checked
	// because dcrd does not list the compose service name as a SAN; chaining to the
	// pinned cert is the security boundary.
	tlsCfg := &tls.Config{
		RootCAs:            pool,
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("no peer certificate")
			}
			_, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{Roots: pool})
			return err
		},
	}
	return &rpcClient{
		url:  fmt.Sprintf("https://%s:%s", host, port),
		auth: "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)),
		http: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}, nil
}

func (c *rpcClient) call(method string, params ...interface{}) (json.RawMessage, error) {
	if params == nil {
		params = []interface{}{}
	}
	reqBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "1.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.auth)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read body: %w", method, err)
	}
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("%s: decode: %w", method, err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("%s: %s", method, out.Error.Message)
	}
	return out.Result, nil
}
