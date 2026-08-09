// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"fmt"
	"os"

	"github.com/decred/dcrd/rpcclient/v8"
)

// DcrdNotifyClient is a second dcrd client in WebSocket mode, used only for
// block-connected notifications (dcrd has no gRPC; the main DcrdClient runs in
// HTTP POST mode, which cannot receive notifications). It pushes a callback on
// each new block so the node sync progress can update without polling.
var DcrdNotifyClient *rpcclient.Client

// InitDcrdNotifyClient connects a websocket dcrd client and subscribes to block
// notifications, invoking onBlock for each connected block. It re-subscribes on
// every (re)connection so progress keeps flowing across dcrd restarts.
func InitDcrdNotifyClient(config Config, onBlock func()) error {
	var certs []byte
	var err error
	if config.RPCCert != "" {
		certs, err = os.ReadFile(config.RPCCert)
		if err != nil {
			return fmt.Errorf("failed to read RPC certificate: %v", err)
		}
	}

	// Closes over the local, not the package var: the callback runs on a
	// goroutine rpcclient starts, and only Connect below starts it.
	var client *rpcclient.Client

	ntfnHandlers := &rpcclient.NotificationHandlers{
		OnClientConnected: func() {
			// Fires on the initial connect and on every reconnect; (re)register
			// for block notifications so the subscription survives dcrd restarts.
			if err := client.NotifyBlocks(context.Background()); err != nil {
				rpccLog.Warnf("dcrd notify: NotifyBlocks failed: %v", err)
				return
			}
			rpccLog.Info("dcrd notify: subscribed to block notifications")
		},
		OnBlockConnected: func(_ []byte, _ [][]byte) {
			if onBlock != nil {
				onBlock()
			}
		},
	}

	connCfg := &rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", config.RPCHost, config.RPCPort),
		Endpoint:     "ws",
		User:         config.RPCUser,
		Pass:         config.RPCPassword,
		HTTPPostMode: false, // websocket mode is required for notifications
		DisableTLS:   config.RPCCert == "",
		Certificates: certs,
		// Connect separately below. New would otherwise dial and fire
		// OnClientConnected before returning, so the callback could run before
		// the client it needs is assigned.
		DisableConnectOnNew: true,
	}

	client, err = rpcclient.New(connCfg, ntfnHandlers)
	if err != nil {
		return fmt.Errorf("failed to create dcrd notify client: %v", err)
	}
	DcrdNotifyClient = client

	// On a cold stack start dcrd is not listening for minutes, so retry. Connect
	// blocks while it backs off, hence the goroutine.
	go func() {
		if err := client.Connect(context.Background(), true); err != nil {
			rpccLog.Warnf("dcrd notify: gave up connecting: %v", err)
		}
	}()
	return nil
}
