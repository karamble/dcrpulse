// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "sync"

// DexBondPost is the state of the most recent bond post to a DEX host. bisonw's
// postbond returns only after broadcast, so the dashboard runs it in the
// background; both spenders (the HTTP handler and the MCP tool) record here
// that a post is in flight, so neither can start a second one for the host.
type DexBondPost struct {
	Phase string `json:"phase"` // submitting | broadcast | error | none
	Error string `json:"error,omitempty"`
}

var dexBondPosts = struct {
	sync.Mutex
	m map[string]DexBondPost
}{m: map[string]DexBondPost{}}

// BeginDexBondPost marks a post to host as in flight. It reports false, and
// changes nothing, while one is already in flight.
func BeginDexBondPost(host string) bool {
	dexBondPosts.Lock()
	defer dexBondPosts.Unlock()
	if dexBondPosts.m[host].Phase == "submitting" {
		return false
	}
	dexBondPosts.m[host] = DexBondPost{Phase: "submitting"}
	return true
}

// EndDexBondPost records how the in-flight post to host finished.
func EndDexBondPost(host string, err error) {
	s := DexBondPost{Phase: "broadcast"}
	if err != nil {
		s = DexBondPost{Phase: "error", Error: err.Error()}
	}
	dexBondPosts.Lock()
	dexBondPosts.m[host] = s
	dexBondPosts.Unlock()
}

// DexBondPostState reports the most recent post to host; Phase is "none" when
// there has never been one.
func DexBondPostState(host string) DexBondPost {
	dexBondPosts.Lock()
	defer dexBondPosts.Unlock()
	if s, ok := dexBondPosts.m[host]; ok {
		return s
	}
	return DexBondPost{Phase: "none"}
}
