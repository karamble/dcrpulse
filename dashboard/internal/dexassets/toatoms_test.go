// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dexassets

import (
	"math"
	"testing"
)

// A conventional amount reaches bisonw as atoms. Before, a negative, NaN or
// huge value became a wrapped integer: a bond cap of -1 turned into
// -100000000, which bisonw reads as "leave unchanged".
func TestToAtoms(t *testing.T) {
	const eth = 60 // factor 1e9 in the catalog
	if ConvFactor(eth) != 1e9 {
		t.Fatalf("catalog ETH factor = %d, the test assumes 1e9", ConvFactor(eth))
	}
	good := []struct {
		name  string
		asset uint32
		v     float64
		want  uint64
	}{
		{"dcr", dcrAssetID, 1.5, 150_000_000},
		{"dcr zero", dcrAssetID, 0, 0},
		{"dcr rounds to the atom", dcrAssetID, 0.123456789, 12_345_679},
		{"eth uses its factor", eth, 1.5, 1_500_000_000},
		{"an unknown asset is read as dcr", 999_999, 2, 200_000_000},
	}
	for _, tt := range good {
		got, err := ToAtoms(tt.asset, tt.v)
		if err != nil || got != tt.want {
			t.Errorf("%s: ToAtoms(%d, %v) = %d, %v; want %d", tt.name, tt.asset, tt.v, got, err, tt.want)
		}
	}
	bad := []struct {
		name  string
		asset uint32
		v     float64
	}{
		{"dcr negative", dcrAssetID, -1},
		{"dcr NaN", dcrAssetID, math.NaN()},
		{"dcr infinity", dcrAssetID, math.Inf(1)},
		{"dcr past the supply", dcrAssetID, 3e7},
		{"eth negative", eth, -0.5},
		{"eth NaN", eth, math.NaN()},
		{"eth past int64", eth, 1e30},
	}
	for _, tt := range bad {
		if got, err := ToAtoms(tt.asset, tt.v); err == nil {
			t.Errorf("%s: ToAtoms(%d, %v) = %d, want an error", tt.name, tt.asset, tt.v, got)
		}
	}
}
