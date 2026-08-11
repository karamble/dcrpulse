// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package auth

import (
	"bytes"
	"testing"
)

// TestSecretRotationInvalidatesSessions proves the mechanism Change relies on:
// rotating the session secret invalidates previously minted tokens.
func TestSecretRotationInvalidatesSessions(t *testing.T) {
	mu.Lock()
	prevSecret, prevEnabled, prevHash := secret, enabled, hash
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		secret, enabled, hash = prevSecret, prevEnabled, prevHash
		mu.Unlock()
	})

	mu.Lock()
	enabled = true
	secret = bytes.Repeat([]byte{0x01}, 32)
	mu.Unlock()

	tok, err := MintSession()
	if err != nil {
		t.Fatalf("MintSession() error: %v", err)
	}
	if !ValidSession(tok) {
		t.Fatal("ValidSession(tok) = false, want true before rotation")
	}

	mu.Lock()
	secret = bytes.Repeat([]byte{0x02}, 32)
	mu.Unlock()

	if ValidSession(tok) {
		t.Error("ValidSession(tok) = true, want false after secret rotation")
	}
	fresh, err := MintSession()
	if err != nil {
		t.Fatalf("MintSession() after rotation error: %v", err)
	}
	if !ValidSession(fresh) {
		t.Error("ValidSession(fresh) = false, want true after rotation")
	}
}
