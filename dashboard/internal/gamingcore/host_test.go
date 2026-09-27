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
	ctx := context.Background()

	br := New("/srv/bridge", Host{})
	if br.dataDir != "/srv/bridge" {
		t.Fatalf("data directory = %q", br.dataDir)
	}
	if br.hostOperator().Protected() {
		t.Fatal("an absent operator counted as protected")
	}
	if br.hostNode() != nil {
		t.Fatal("an absent node answered")
	}
	if _, err := br.currentNetwork(ctx); !errors.Is(err, ErrGamingChainUnavailable) {
		t.Fatalf("network without a node = %v", err)
	}
	if err := br.hostRelay().SendGroupMessage(ctx, [32]byte{}, "x"); !errors.Is(err, ErrNotSent) {
		t.Fatalf("a send with no relay = %v, want not sent", err)
	}
	if _, err := br.hostWallet().Construct(ctx, 0, "addr", 1); err == nil {
		t.Fatal("an absent wallet built a transaction")
	}
	if err := br.hostWallet().WithUnlockedAccount(ctx, 0, nil, func() error { return nil }); err == nil {
		t.Fatal("an absent wallet unlocked an account")
	}
}
