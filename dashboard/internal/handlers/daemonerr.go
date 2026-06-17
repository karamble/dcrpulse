// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"dcrpulse/internal/services"
)

// respondDaemonError writes the HTTP response for a failed daemon call. When the
// error looks like the daemon being unreachable (down, still starting, or
// running a startup database upgrade), it returns 503 with a friendly,
// log-derived message so the UI can show a "starting" / "database upgrade in
// progress" state instead of a raw 500. Genuine RPC errors from a responsive
// daemon are returned as 500 unchanged, matching prior behavior.
func respondDaemonError(w http.ResponseWriter, r *http.Request, component services.LogComponent, err error) {
	if services.IsDaemonUnreachable(err) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		hint := services.DaemonStartupHint(ctx, component)
		if hint.Detail != "" {
			log.Printf("%s unreachable (%s): %s", component, hint.State, hint.Detail)
		}
		http.Error(w, hint.Message, http.StatusServiceUnavailable)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// respondUpstreamError writes the HTTP response for a failed call to an upstream
// daemon the dashboard proxies over HTTP/gRPC (bisonw, brclientd). A connectivity
// failure (daemon down or still starting) becomes a friendly 503 with a
// log-derived message; any other (app-level) error stays a 502 with the raw text,
// matching prior behavior and keeping it debuggable.
func respondUpstreamError(w http.ResponseWriter, component services.LogComponent, err error) {
	if services.IsDaemonUnreachable(err) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		http.Error(w, services.DaemonStartupHint(ctx, component).Message, http.StatusServiceUnavailable)
		return
	}
	http.Error(w, err.Error(), http.StatusBadGateway)
}

// dexWriteErr and brWriteErr are component-bound shortcuts for respondUpstreamError
// so the many bisonw / brclientd handler call sites need no extra import.
func dexWriteErr(w http.ResponseWriter, err error) {
	respondUpstreamError(w, services.LogComponentDcrdex, err)
}

func brWriteErr(w http.ResponseWriter, err error) {
	respondUpstreamError(w, services.LogComponentBrclientd, err)
}
