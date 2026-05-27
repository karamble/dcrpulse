// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"fmt"
	"net"
	"sync"

	"dcrpulse/pkg/bisonw"
)

// DcrdexConfig holds the connection settings for the bisonw container: the RPC
// server (HTTP calls) and the webserver (the /ws live feed, which carries both
// the order book and the notification stream).
type DcrdexConfig struct {
	Host     string
	Port     string
	User     string
	Pass     string
	CertPath string
	// WSPort and WSCertPath address bisonw's webserver, whose /ws endpoint is
	// the only one that streams the notification feed. Its cert is a separate
	// file (web.cert) from the RPC cert.
	WSPort     string
	WSCertPath string
}

var (
	// DcrdexCfg is the resolved config. The clients are built lazily because
	// bisonw generates its TLS certs on first run, so the files may not exist
	// when the dashboard starts.
	DcrdexCfg DcrdexConfig

	dcrdexClient    *bisonw.Client
	dcrdexWSClient  *bisonw.Client
	dcrdexWebClient *bisonw.WebClient
	dcrdexMu        sync.Mutex
)

// InitDcrdexConfig records the bisonw RPC connection settings and resets any
// previously built client.
func InitDcrdexConfig(cfg DcrdexConfig) {
	dcrdexMu.Lock()
	defer dcrdexMu.Unlock()
	DcrdexCfg = cfg
	dcrdexClient = nil
	dcrdexWSClient = nil
	dcrdexWebClient = nil
}

// DcrdexClient returns the bisonw RPC client, constructing it on first use once
// the RPC cert is available.
func DcrdexClient() (*bisonw.Client, error) {
	dcrdexMu.Lock()
	defer dcrdexMu.Unlock()
	if dcrdexClient != nil {
		return dcrdexClient, nil
	}
	if DcrdexCfg.Host == "" || DcrdexCfg.Port == "" {
		return nil, fmt.Errorf("dcrdex: not configured")
	}
	c, err := bisonw.New(bisonw.Config{
		Addr:     net.JoinHostPort(DcrdexCfg.Host, DcrdexCfg.Port),
		User:     DcrdexCfg.User,
		Pass:     DcrdexCfg.Pass,
		CertPath: DcrdexCfg.CertPath,
	})
	if err != nil {
		return nil, err
	}
	dcrdexClient = c
	return c, nil
}

// DcrdexWSClient returns a client addressed to bisonw's webserver, whose /ws
// endpoint streams the order book and the notification feed. It is used only to
// derive the relay's dial info; the webserver /ws ignores auth, so the RPC
// credentials are passed only to satisfy the client constructor. Built lazily
// once web.cert is available.
func DcrdexWSClient() (*bisonw.Client, error) {
	dcrdexMu.Lock()
	defer dcrdexMu.Unlock()
	if dcrdexWSClient != nil {
		return dcrdexWSClient, nil
	}
	if DcrdexCfg.Host == "" || DcrdexCfg.WSPort == "" {
		return nil, fmt.Errorf("dcrdex: websocket not configured")
	}
	c, err := bisonw.New(bisonw.Config{
		Addr:       net.JoinHostPort(DcrdexCfg.Host, DcrdexCfg.WSPort),
		User:       DcrdexCfg.User,
		Pass:       DcrdexCfg.Pass,
		CertPath:   DcrdexCfg.WSCertPath,
		ServerName: DcrdexCfg.Host,
	})
	if err != nil {
		return nil, err
	}
	dcrdexWSClient = c
	return c, nil
}

// DcrdexWebClient returns a client for bisonw's webserver HTTP API (the
// market-maker routes), built lazily once web.cert is available. It shares the
// webserver address and pinned cert with the /ws relay client.
func DcrdexWebClient() (*bisonw.WebClient, error) {
	dcrdexMu.Lock()
	defer dcrdexMu.Unlock()
	if dcrdexWebClient != nil {
		return dcrdexWebClient, nil
	}
	if DcrdexCfg.Host == "" || DcrdexCfg.WSPort == "" {
		return nil, fmt.Errorf("dcrdex: webserver not configured")
	}
	c, err := bisonw.NewWebClient(bisonw.Config{
		Addr:       net.JoinHostPort(DcrdexCfg.Host, DcrdexCfg.WSPort),
		CertPath:   DcrdexCfg.WSCertPath,
		ServerName: DcrdexCfg.Host,
	})
	if err != nil {
		return nil, err
	}
	dcrdexWebClient = c
	return c, nil
}

// The bisonw app password is held only in process memory for the unlocked
// session and is never persisted to disk. It is cleared on lock and lost on
// dashboard restart, after which the user must unlock again.
var (
	dcrdexAppPass    string
	dcrdexAppPassSet bool
	dcrdexSessionMu  sync.Mutex
)

// SetDcrdexAppPass records the app password for the unlocked session.
func SetDcrdexAppPass(p string) {
	dcrdexSessionMu.Lock()
	defer dcrdexSessionMu.Unlock()
	dcrdexAppPass = p
	dcrdexAppPassSet = true
}

// DcrdexAppPass returns the in-memory app password and whether it is set.
func DcrdexAppPass() (string, bool) {
	dcrdexSessionMu.Lock()
	defer dcrdexSessionMu.Unlock()
	return dcrdexAppPass, dcrdexAppPassSet
}

// ClearDcrdexAppPass forgets the in-memory app password, locking the session.
func ClearDcrdexAppPass() {
	dcrdexSessionMu.Lock()
	defer dcrdexSessionMu.Unlock()
	dcrdexAppPass = ""
	dcrdexAppPassSet = false
}
