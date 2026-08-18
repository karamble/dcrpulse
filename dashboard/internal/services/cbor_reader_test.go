// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

// The reader exists to reject hostile device files, so skip() must refuse an
// oversized string length the way text() and arrayHead() already do - a 64-bit
// length must never wrap int negative and walk pos off the buffer.
func TestCBORSkipBoundsHostileStringLengths(t *testing.T) {
	cases := []struct {
		name    string
		buf     []byte
		wantErr bool
		wantPos int
	}{
		{
			// 0x5b = byte string with 8-byte length; 2^63 wraps int negative.
			name:    "length 2^63",
			buf:     []byte{0x5b, 0x80, 0, 0, 0, 0, 0, 0, 0},
			wantErr: true,
		},
		{
			name:    "length 2^64-1",
			buf:     []byte{0x5b, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			wantErr: true,
		},
		{
			// 0x42 = 2-byte string with only 1 payload byte present.
			name:    "plain over-length",
			buf:     []byte{0x42, 0x01},
			wantErr: true,
		},
		{
			// 0x42 = 2-byte string, both bytes present: pos lands just past it.
			name:    "valid string skip",
			buf:     []byte{0x42, 0xaa, 0xbb},
			wantPos: 3,
		},
		{
			// 0x82 = 2-element array of small ints: the recursive path.
			name:    "valid array skip",
			buf:     []byte{0x82, 0x01, 0x02},
			wantPos: 3,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					// Mutation catch: the pre-fix int(v) conversion panics here.
					t.Fatalf("skip panicked: %v", p)
				}
			}()
			r := &cborReader{buf: c.buf}
			err := r.skip()
			if c.wantErr {
				if err == nil {
					t.Fatal("hostile length was not refused")
				}
			} else if err != nil {
				t.Fatalf("valid skip refused: %v", err)
			}
			if r.pos < 0 {
				t.Fatalf("pos went negative: %d", r.pos)
			}
			if !c.wantErr && r.pos != c.wantPos {
				t.Fatalf("pos = %d, want %d", r.pos, c.wantPos)
			}
			// The next read must fail or succeed cleanly, never index negatively.
			if _, _, err := r.head(); err == nil && r.pos < 0 {
				t.Fatal("head() after skip left a negative pos")
			}
		})
	}
}
