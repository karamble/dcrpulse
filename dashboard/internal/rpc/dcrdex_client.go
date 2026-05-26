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
