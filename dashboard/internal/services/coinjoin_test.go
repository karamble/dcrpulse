// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"encoding/hex"
	"testing"

	"github.com/decred/dcrd/wire"
)

// coinJoinTestHex serializes a transaction with numIn inputs and one output per
// value; sameScript pays every output to one script.
func coinJoinTestHex(t *testing.T, numIn int, values []int64, sameScript bool) string {
	t.Helper()
	mtx := wire.NewMsgTx()
	for i := 0; i < numIn; i++ {
		mtx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: uint32(i)}, 0, nil))
	}
	for i, v := range values {
		script := []byte{0x76, byte(i)}
		if sameScript {
			script = []byte{0x76}
		}
		mtx.AddTxOut(wire.NewTxOut(v, script))
	}
	b, err := mtx.Bytes()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return hex.EncodeToString(b)
}

// mixTestHex is a CSPP mix shape: five inputs, five equal outputs.
func mixTestHex(t *testing.T) string {
	return coinJoinTestHex(t, 5, []int64{536870912, 536870912, 536870912, 536870912, 536870912}, false)
}

func TestIsCoinJoinHex(t *testing.T) {
	batch := []int64{5e8, 5e8, 5e8, 1e8, 2e8, 3e8, 4e8, 6e8, 7e8, 8e8, 9e8, 1234}
	tests := []struct {
		name string
		hex  string
		want bool
	}{
		{"a mix", mixTestHex(t), true},
		{"a batch payment with three equal amounts", coinJoinTestHex(t, 3, batch, false), false},
		{"three equal outputs to one script", coinJoinTestHex(t, 3, []int64{5e8, 5e8, 5e8}, true), false},
		{"too few inputs", coinJoinTestHex(t, 2, []int64{5e8, 5e8, 5e8}, false), false},
		{"not hex", "zz", false},
		{"not a transaction", "01", false},
		{"empty", "", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isCoinJoinHex(test.hex); got != test.want {
				t.Fatalf("isCoinJoinHex = %v, want %v", got, test.want)
			}
		})
	}
}
