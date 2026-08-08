// Copyright (c) 2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

// TestSafeResumeHeight covers the rule that decides whether a block the scan
// could not read becomes a permanent hole in the treasury history. The client
// resumes one above the returned height, so anything at or below it is rescanned.
func TestSafeResumeHeight(t *testing.T) {
	tests := []struct {
		name        string
		lastScanned int64
		failed      []int64
		want        int64
	}{{
		name:        "no failures resumes above the last block scanned",
		lastScanned: 1104480,
		failed:      nil,
		want:        1104480,
	}, {
		name:        "failures at several heights hold at the earliest",
		lastScanned: 1104480,
		failed:      []int64{553024, 552736, 553600},
		want:        552735,
	}, {
		name:        "a later success does not move the mark past an earlier failure",
		lastScanned: 1104480,
		failed:      []int64{552736},
		want:        552735,
	}, {
		name:        "a failure at the first block rescans everything",
		lastScanned: 1104480,
		failed:      []int64{552448},
		want:        552447,
	}, {
		name:        "every block failing holds at the first",
		lastScanned: 1104480,
		failed:      []int64{552448, 552736, 553024},
		want:        552447,
	}, {
		name:        "a failure at height 1 does not resume below genesis",
		lastScanned: 288,
		failed:      []int64{1},
		want:        0,
	}, {
		name:        "nothing scanned and nothing failed stays put",
		lastScanned: 0,
		failed:      nil,
		want:        0,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := safeResumeHeight(test.lastScanned, test.failed)
			if got != test.want {
				t.Fatalf("safeResumeHeight(%d, %v) = %d, want %d",
					test.lastScanned, test.failed, got, test.want)
			}
		})
	}
}
