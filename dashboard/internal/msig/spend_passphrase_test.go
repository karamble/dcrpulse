// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"bytes"
	"context"
	"testing"
)

// Both spend entry points take the passphrase as a slice they own, so it must
// be wiped on every return - including the earliest refusal, which is the path
// an unknown wallet or a wrong recipient count takes.
func TestSpendEntryPointsWipeThePassphrase(t *testing.T) {
	orig := networkSeam
	networkSeam = func(context.Context) (string, error) { return "simnet", nil }
	t.Cleanup(func() { networkSeam = orig })

	t.Run("ProposeSpend", func(t *testing.T) {
		pass := []byte{1, 2, 3, 4, 5, 6, 7, 8}
		_, err := ProposeSpend(context.Background(), "no-such-wallet",
			[]Recipient{{Address: "DsAlpha"}}, false, nil, "", 0, pass)
		if err == nil {
			t.Fatal("a proposal against an unknown wallet was accepted")
		}
		requireWiped(t, pass)
	})

	t.Run("SignIncomingProposal", func(t *testing.T) {
		pass := []byte{1, 2, 3, 4, 5, 6, 7, 8}
		err := SignIncomingProposal(context.Background(), "no-such-wallet", "deadbeef", pass)
		if err == nil {
			t.Fatal("a signature against an unknown wallet was accepted")
		}
		requireWiped(t, pass)
	})

}

func requireWiped(t *testing.T, b []byte) {
	t.Helper()
	if !bytes.Equal(b, make([]byte, len(b))) {
		t.Fatalf("passphrase left readable in memory: %v", b)
	}
}
