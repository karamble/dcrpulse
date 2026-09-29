// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"fmt"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// hashString is the display form of a hash dcrwallet or dcrlnd sends in wire
// (little-endian) order, or "" when it is not 32 bytes.
func hashString(b []byte) string {
	h, err := chainhash.NewHash(b)
	if err != nil {
		return ""
	}
	return h.String()
}

// wireHash turns a displayed 64-hex-character hash into the wire order the
// daemons expect. NewHashFromStr pads short input, so the length is checked.
func wireHash(s string) ([]byte, error) {
	if len(s) != chainhash.MaxHashStringSize {
		return nil, fmt.Errorf("invalid hash %q", s)
	}
	h, err := chainhash.NewHashFromStr(s)
	if err != nil {
		return nil, fmt.Errorf("invalid hash %q: %w", s, err)
	}
	return h[:], nil
}
