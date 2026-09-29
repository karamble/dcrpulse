// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// Daemons send hashes byte-reversed from how they are displayed; a hash of
// the wrong length used to be reversed and passed on anyway.
func TestHashByteOrder(t *testing.T) {
	// Decred's mainnet genesis block, as displayed and in wire order.
	const display = "298e5cc3d985bfe7f81dc135f360abe089edd4396b86d2de66b0cef42b21d980"
	wire, _ := hex.DecodeString("80d9212bf4ceb066ded2866b39d4ed89e0ab60f335c11df8e7bf85d9c35c8e29")

	if got := hashString(wire); got != display {
		t.Fatalf("hashString = %s, want %s", got, display)
	}
	got, err := wireHash(display)
	if err != nil || !bytes.Equal(got, wire) {
		t.Fatalf("wireHash = %x, %v; want %x", got, err, wire)
	}

	for _, n := range []int{0, 31, 33} {
		if s := hashString(make([]byte, n)); s != "" {
			t.Errorf("hashString of %d bytes = %q, want empty", n, s)
		}
	}
	for _, s := range []string{"", display[:63], display + "0", strings.Repeat("zz", 32)} {
		if b, err := wireHash(s); err == nil {
			t.Errorf("wireHash(%q) = %x, want an error", s, b)
		}
	}
}
