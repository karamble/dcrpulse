// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/pkg/bisonw"
)

// dexClient answers 503 and reports false when bisonw's RPC client is not up.
func dexClient(w http.ResponseWriter) (*bisonw.Client, bool) {
	client, err := rpc.DcrdexClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return nil, false
	}
	return client, true
}

// dexUnlockedClient gates an RPC action that needs bisonw's core logged in:
// 409 while no session is up, then 503 when the RPC client is not up. The
// dashboard holds no app password, so the webserver session is the unlock
// state; bisonw's core is shared by both servers, so that login satisfies the
// RPC routes too.
func dexUnlockedClient(w http.ResponseWriter) (*bisonw.Client, bool) {
	if !rpc.DcrdexUnlocked() {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return nil, false
	}
	return dexClient(w)
}

// dexWebSession gates an action that needs the unlocked session: 409 while no
// webserver session is up, then 503 when the web client is not up, in that
// order.
func dexWebSession(w http.ResponseWriter) (*bisonw.WebClient, bool) {
	if !rpc.DcrdexUnlocked() {
		http.Error(w, "DCRDEX is locked", http.StatusConflict)
		return nil, false
	}
	web, err := rpc.DcrdexWebClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return nil, false
	}
	return web, true
}

// dexCall gates the request, runs call under timeout and writes its result: a
// json.RawMessage from bisonw byte for byte, anything else as JSON.
func dexCall[C any](w http.ResponseWriter, r *http.Request, gate func(http.ResponseWriter) (C, bool),
	timeout time.Duration, call func(ctx context.Context, c C) (any, error)) {
	c, ok := gate(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	v, err := call(ctx, c)
	if err != nil {
		dexWriteErr(w, err)
		return
	}
	if raw, isRaw := v.(json.RawMessage); isRaw {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
		return
	}
	writeJSON(w, v)
}

// dexDo is dexCall for an action whose success is {"ok": true}.
func dexDo[C any](w http.ResponseWriter, r *http.Request, gate func(http.ResponseWriter) (C, bool),
	timeout time.Duration, act func(ctx context.Context, c C) error) {
	dexCall(w, r, gate, timeout, func(ctx context.Context, c C) (any, error) {
		if err := act(ctx, c); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, nil
	})
}
