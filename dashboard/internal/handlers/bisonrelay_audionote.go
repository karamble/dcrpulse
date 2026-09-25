// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
)

// BisonrelayAudioNoteHandler sends a voice note as a PM. The browser records
// raw Opus packets; the container and the embed tag are built here so the
// message is byte for byte what bruig sends.
func BisonrelayAudioNoteHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		User       string `json:"user"`
		PacketsB64 string `json:"packets_b64"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	user, ok := brID(w, req.User, "user")
	if !ok {
		return
	}
	req.User = user
	blob, err := base64.StdEncoding.DecodeString(req.PacketsB64)
	if err != nil {
		http.Error(w, "packets_b64: "+err.Error(), http.StatusBadRequest)
		return
	}
	body, err := services.BuildAudioNoteBody(blob, time.Now())
	switch {
	case errors.Is(err, services.ErrAudioNoteTooLarge):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := rpc.BrclientdSendPM(r.Context(), req.User, body); err != nil {
		brWriteErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"body": body})
}
