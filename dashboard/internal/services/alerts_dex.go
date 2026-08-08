// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"dcrpulse/internal/alerts"
	"dcrpulse/internal/rpc"
	"dcrpulse/pkg/bisonw"

	"github.com/gorilla/websocket"
)

// StartDexWatcher owns a backend bisonw /ws subscription for trade events
// (the browser relay in handlers is tab-scoped, so the dashboard would
// otherwise be blind with no DEX tab open). Runs only while the DEX app pass
// is cached: an unlock implies bisonw is configured and initialized, so
// needs-init / needs-unlock deployments idle silently, and dial failures
// while unlocked are the dex_unreachable condition.
func StartDexWatcher(ctx context.Context) {
	go func() {
		backoff := 5 * time.Second
		for {
			if ctx.Err() != nil {
				return
			}
			if _, unlocked := rpc.DcrdexAppPass(); !unlocked || SyncPaused() {
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
				continue
			}
			client, err := rpc.DcrdexWSClient()
			if err == nil {
				err = watchDexNotifications(ctx, client)
				if ctx.Err() != nil {
					return
				}
			}
			if err != nil {
				alerts.Raise("dex_unreachable", "", err.Error())
				alrtLog.Warnf("dex watcher: %v (retrying in %s)", err, backoff)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 60*time.Second {
				backoff *= 2
			} else {
				backoff = 60 * time.Second
			}
		}
	}()
}

func watchDexNotifications(ctx context.Context, client *bisonw.Client) error {
	tlsConfig, wsURL, basicAuth := client.WSDialInfo()
	dialer := &websocket.Dialer{TLSClientConfig: tlsConfig, HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.Dial(wsURL, http.Header{"Authorization": {basicAuth}})
	if err != nil {
		return err
	}
	defer conn.Close()
	alerts.Resolve("dex_unreachable", "")

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		emitDexAlert(data)
	}
}

func emitDexAlert(data []byte) {
	var frame struct {
		Route   string `json:"route"`
		Payload struct {
			Type    string `json:"type"`
			Topic   string `json:"topic"`
			Subject string `json:"subject"`
			Details string `json:"details"`
			ID      string `json:"id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &frame); err != nil || frame.Route != "notify" {
		return
	}
	// OrderComplete is bisonw's success note for a fully executed trade
	// (dcrdex client/core/notification.go note taxonomy).
	if frame.Payload.Type != "order" || frame.Payload.Topic != "OrderComplete" {
		return
	}
	detail := frame.Payload.Details
	if detail == "" {
		detail = frame.Payload.Subject
	}
	dedupe := frame.Payload.ID
	if dedupe == "" {
		dedupe = frame.Payload.Topic + "|" + detail
	}
	alerts.Emit("dex_order_filled", detail, dedupe)
}
