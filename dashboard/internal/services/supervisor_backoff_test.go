// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"testing"
	"time"
)

// A healthy run resets the sequence to the floor; a failing one doubles,
// clamped so the ceiling is never overshot. The literals here are fixtures
// independent of the constants under test.
func TestNextSupervisorBackoff(t *testing.T) {
	cases := []struct {
		name    string
		current time.Duration
		ranFor  time.Duration
		want    time.Duration
	}{
		{"healthy resets from ceiling", 60 * time.Second, 2 * time.Hour, 5 * time.Second},
		{"healthy resets from mid", 20 * time.Second, 31 * time.Second, 5 * time.Second},
		{"healthy exactly at boundary", 40 * time.Second, 30 * time.Second, 5 * time.Second},
		{"failing doubles", 5 * time.Second, time.Second, 10 * time.Second},
		{"failing doubles again", 10 * time.Second, 0, 20 * time.Second},
		{"clamped after doubling", 40 * time.Second, time.Second, 60 * time.Second},
		{"stays at ceiling", 60 * time.Second, time.Second, 60 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NextSupervisorBackoff(c.current, c.ranFor); got != c.want {
				t.Fatalf("NextSupervisorBackoff(%v, %v) = %v, want %v", c.current, c.ranFor, got, c.want)
			}
		})
	}
}
