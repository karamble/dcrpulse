// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/rpc"
)

var rtdtAudioBrowserUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     middleware.SameOriginWS,
}

// rtdtRVValid bounds the session rv to a URL-path-safe token before it is
// interpolated into the brclientd upstream URL. brclientd rv values are
// hex/base32-style tokens, so this rejects any path or query metacharacter.
var rtdtRVValid = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// BisonrelayRTDTAudioHandler bridges a browser WebSocket to brclientd's
// /rtdt/sessions/{rv}/audio. Binary frames are forwarded blindly in
// both directions; the wire framing is owned by brclientd + the browser.
// Ping/pong is handled on each leg independently so a slow consumer on
// one side doesn't keep the other alive.
func BisonrelayRTDTAudioHandler(w http.ResponseWriter, r *http.Request) {
	rv := mux.Vars(r)["rv"]
	if rv == "" {
		log.Printf("RTDT audio: missing rv in path %s", r.URL.Path)
		http.Error(w, "missing session rv", http.StatusBadRequest)
		return
	}
	if !rtdtRVValid.MatchString(rv) {
		log.Printf("RTDT audio: rejecting malformed rv %q", rv)
		http.Error(w, "invalid session rv", http.StatusBadRequest)
		return
	}
	log.Printf("RTDT audio: upgrade request rv=%s origin=%q", rv, r.Header.Get("Origin"))

	tlsCfg, baseURL, err := rpc.BrclientdWSDialer()
	if err != nil {
		log.Printf("RTDT audio: dialer config: %v", err)
		http.Error(w, "brclientd dialer: "+err.Error(), http.StatusInternalServerError)
		return
	}
	dialer := &websocket.Dialer{
		TLSClientConfig:  tlsCfg,
		HandshakeTimeout: 10 * time.Second,
	}

	// Dial brclientd first; if it refuses (e.g. 409 because the session
	// is not joined yet, or already attached in another tab) we surface
	// that to the browser BEFORE upgrading our side, so the React client
	// gets a clean HTTP error rather than a torn-down WS.
	upstreamURL := baseURL + "/rtdt/sessions/" + rv + "/audio"
	upstream, resp, err := dialer.Dial(upstreamURL, nil)
	if err != nil {
		if resp != nil {
			log.Printf("RTDT audio: brclientd dial rv=%s HTTP %d", rv, resp.StatusCode)
			http.Error(w, "brclientd /rtdt/audio: "+resp.Status, resp.StatusCode)
			return
		}
		log.Printf("RTDT audio: brclientd dial rv=%s err=%v", rv, err)
		http.Error(w, "brclientd dial: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer upstream.Close()
	log.Printf("RTDT audio: upstream WS open rv=%s", rv)

	browser, err := rtdtAudioBrowserUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("RTDT audio: browser upgrade rv=%s err=%v", rv, err)
		return
	}
	defer browser.Close()
	log.Printf("RTDT audio: browser WS upgraded rv=%s", rv)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var (
		brWriteMu sync.Mutex
		upWriteMu sync.Mutex
	)

	// Browser -> brclientd pump.
	go func() {
		defer cancel()
		for {
			mt, data, err := browser.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.BinaryMessage {
				continue
			}
			upWriteMu.Lock()
			_ = upstream.SetWriteDeadline(time.Now().Add(5 * time.Second))
			err = upstream.WriteMessage(websocket.BinaryMessage, data)
			upWriteMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	// brclientd -> browser pump.
	go func() {
		defer cancel()
		for {
			mt, data, err := upstream.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.BinaryMessage {
				continue
			}
			brWriteMu.Lock()
			_ = browser.SetWriteDeadline(time.Now().Add(5 * time.Second))
			err = browser.WriteMessage(websocket.BinaryMessage, data)
			brWriteMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	// Heartbeat to both sides every 30s. brclientd already pings its
	// own side; this ping is just to keep the browser <-> dashboard leg
	// alive when audio is silent (no binary frames between speech
	// bursts).
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			brWriteMu.Lock()
			_ = browser.SetWriteDeadline(time.Now().Add(5 * time.Second))
			err := browser.WriteMessage(websocket.PingMessage, nil)
			brWriteMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}
