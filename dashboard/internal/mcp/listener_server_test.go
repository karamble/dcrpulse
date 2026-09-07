// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import "testing"

// The listener is bound on every interface inside the container, so a sibling
// that parks tokenless keep-alive connections must have them reaped, the way
// the dashboard's own server reaps its own.
func TestListenerReapsIdleConnections(t *testing.T) {
	srv := newListenerServer(nil)
	if srv.IdleTimeout <= 0 {
		t.Fatal("the MCP listener never closes an idle keep-alive connection")
	}
	if srv.ReadHeaderTimeout <= 0 {
		t.Fatal("the MCP listener waits forever for request headers")
	}
	if srv.ErrorLog == nil {
		t.Fatal("the MCP listener's own log lines go to stderr instead of the daemon log")
	}
}
