// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

func TestShortChanID(t *testing.T) {
	tests := []struct {
		name string
		id   uint64
		want string
	}{
		// Real channels from the mainnet node. The output index is the part a
		// JSON number used to round away, so these are the cases that matter.
		{"real channel, output 1", 1208174162926501889, "1098828x10x1"},
		{"real channel, output 0", 1208182959019327488, "1098836x7x0"},
		{"real channel, high tx index", 1208519409578016769, "1099142x16x1"},

		// A channel carries no ID until it confirms; omitempty then keeps the
		// field off the wire rather than shipping a meaningless 0x0x0.
		{"unconfirmed", 0, ""},

		{"first output of the first tx in block 1", 1<<40 | 0<<16 | 0, "1x0x0"},
		{"field maxima", uint64(1<<24-1)<<40 | uint64(1<<24-1)<<16 | uint64(1<<16-1),
			"16777215x16777215x65535"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortChanID(tc.id); got != tc.want {
				t.Errorf("shortChanID(%d) = %q, want %q", tc.id, got, tc.want)
			}
		})
	}
}
