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

// DcrdexConfig holds the connection settings for the backend-only bisonw RPC
// server (the dcrdex container).
type DcrdexConfig struct {
	Host     string
	Port     string
	User     string
	Pass     string
	CertPath string
}

var (
	// DcrdexCfg is the resolved config. The client is built lazily because
	// bisonw generates its RPC TLS cert on first run, so the file may not
	// exist when the dashboard starts.
	DcrdexCfg DcrdexConfig

	dcrdexClient *bisonw.Client
	dcrdexMu     sync.Mutex
)

// InitDcrdexConfig records the bisonw RPC connection settings and resets any
// previously built client.
func InitDcrdexConfig(cfg DcrdexConfig) {
	dcrdexMu.Lock()
	defer dcrdexMu.Unlock()
	DcrdexCfg = cfg
	dcrdexClient = nil
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
