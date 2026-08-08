// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"math"
	"testing"
)

func TestLooksLikeCoinJoin(t *testing.T) {
	tests := []struct {
		name   string
		inputs int
		values []float64
		want   bool
	}{{
		name:   "three matching outputs",
		inputs: 3,
		values: []float64{1.5, 1.5, 1.5},
		want:   true,
	}, {
		name:   "matching outputs among others",
		inputs: 5,
		values: []float64{0.1, 2.25, 9.9, 2.25, 0.7, 2.25},
		want:   true,
	}, {
		name:   "only two match",
		inputs: 4,
		values: []float64{1.5, 1.5, 0.3, 0.9},
		want:   false,
	}, {
		name:   "too few inputs",
		inputs: 2,
		values: []float64{1.5, 1.5, 1.5},
		want:   false,
	}, {
		name:   "too few outputs",
		inputs: 9,
		values: []float64{1.5, 1.5},
		want:   false,
	}, {
		// 1234.56789012 * 1e8 is 123456789011.99998.
		name:   "amounts that do not scale cleanly",
		inputs: 3,
		values: []float64{1234.56789012, 1234.56789012, 1234.56789012},
		want:   true,
	}, {
		name:   "non-finite values cannot match and are skipped",
		inputs: 3,
		values: []float64{math.NaN(), math.Inf(1), math.Inf(-1)},
		want:   false,
	}, {
		name:   "no outputs",
		inputs: 3,
		values: nil,
		want:   false,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := looksLikeCoinJoin(test.inputs, test.values); got != test.want {
				t.Fatalf("looksLikeCoinJoin(%d, %v) = %v, want %v",
					test.inputs, test.values, got, test.want)
			}
		})
	}
}
