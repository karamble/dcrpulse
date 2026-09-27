// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingcore

import (
	"context"
	"errors"
	"testing"
)

// A part the host left out is unavailable, never trusted: above all an absent
// operator is not a protected one, because the bridge serves no game while the
// operator is unprotected.
func TestAPartTheHostLeftOutIsUnavailable(t *testing.T) {
	prevHost, prevDir := host, GamingStateDir
	t.Cleanup(func() { host, GamingStateDir = prevHost, prevDir })
	networkMu.Lock()
	networkName = ""
	networkMu.Unlock()
	ctx := context.Background()

	Configure("/srv/bridge", Host{})
	if GamingStateDir != "/srv/bridge" {
		t.Fatalf("data directory = %q", GamingStateDir)
	}
	if hostOperator().Protected() {
		t.Fatal("an absent operator counted as protected")
	}
	if hostNode() != nil {
		t.Fatal("an absent node answered")
	}
	if _, err := currentNetwork(ctx); !errors.Is(err, ErrGamingChainUnavailable) {
		t.Fatalf("network without a node = %v", err)
	}
	if err := hostRelay().SendGroupMessage(ctx, [32]byte{}, "x"); !errors.Is(err, ErrNotSent) {
		t.Fatalf("a send with no relay = %v, want not sent", err)
	}
	if _, err := hostWallet().Construct(ctx, 0, "addr", 1); err == nil {
		t.Fatal("an absent wallet built a transaction")
	}
	if err := hostWallet().WithUnlockedAccount(ctx, 0, nil, func() error { return nil }); err == nil {
		t.Fatal("an absent wallet unlocked an account")
	}
}
