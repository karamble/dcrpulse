// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"net/http"
	"time"

	"dcrpulse/pkg/bisonw"

	"github.com/gorilla/websocket"
)

// dexWSMsgLimit bounds what bisonw may send us on one frame. Unlike the Bison
// Relay ceilings this is ours, not a protocol maximum: DCRDEX defines none, and
// an order book snapshot is the largest thing that legitimately crosses.
const dexWSMsgLimit = 4 << 20

// DialDcrdexWS opens bisonw's /ws with the pinned TLS config, the Basic auth
// header and a read limit. pkg/bisonw deliberately owns no WebSocket
// dependency (see WSDialInfo), so the dialing lives here, where the three
// callers that used to build an identical dialer can share it.
func DialDcrdexWS(ctx context.Context, c *bisonw.Client) (*websocket.Conn, *http.Response, error) {
	tlsConfig, wsURL, basicAuth := c.WSDialInfo()
	dialer := &websocket.Dialer{TLSClientConfig: tlsConfig, HandshakeTimeout: 15 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, wsURL, http.Header{"Authorization": {basicAuth}})
	if err != nil {
		return nil, resp, err
	}
	conn.SetReadLimit(dexWSMsgLimit)
	return conn, resp, nil
}
