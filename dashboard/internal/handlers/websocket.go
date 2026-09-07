// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http"

	"dcrpulse/internal/middleware"

	"github.com/decred/slog"
	"github.com/gorilla/websocket"
)

// gorilla enforces a read limit only when one is set, so an upgrader without
// one buffers a frame of any size and a single oversized message can exhaust
// the process. Every browser socket is opened here so a new one cannot be added
// without a bound.
const (
	// wsBrowserMsgLimit bounds what the browser may send on a control or event
	// socket. The largest legitimate message is the Lightning payment request
	// read once by LightningSendPaymentHandler; uploads go over HTTP multipart
	// and are capped there.
	wsBrowserMsgLimit = 64 << 10

	// wsDexMsgLimit bounds the bisonw relay in both directions. Unlike the
	// Bison Relay limits this is a ceiling of our own choosing, not a protocol
	// maximum: DCRDEX defines none, and an order book snapshot is the largest
	// thing that legitimately crosses.
	wsDexMsgLimit = 4 << 20
)

// upgradeWS turns an HTTP request into a browser WebSocket with an origin check
// and a read limit, logging and answering the request itself on failure. A
// false return means the caller should simply return.
func upgradeWS(w http.ResponseWriter, r *http.Request, log slog.Logger, label string, limit int64) (*websocket.Conn, bool) {
	upgrader := websocket.Upgrader{CheckOrigin: middleware.SameOriginWS}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade %s WebSocket: %v", label, err)
		return nil, false
	}
	conn.SetReadLimit(limit)
	return conn, true
}
