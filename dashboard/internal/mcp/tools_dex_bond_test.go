// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"math"
	"testing"

	"dcrpulse/pkg/bisonw"
)

// dcrAsset is the default bond asset, and the one whose 1e8 scale the field
// name assumes.
var dcrAsset = uint32(bisonw.AssetDCR)

func ptr(f float64) *float64 { return &f }

// The field is conventional and SetBondOptions takes atoms, so the conversion is
// the whole point. With the default bond asset that scale is DCR's: 1 DCR is 1e8
// atoms, and the value is a ceiling on what auto-renewal may lock without any
// further approval.
func TestBondCeilingAtoms(t *testing.T) {
	tests := []struct {
		name string
		in   *float64
		want int
	}{
		// nil is "leave unchanged", which SetBondOptions spells -1.
		{"nil leaves the option unchanged", nil, -1},
		// Zero means "reset to the server default", NOT the unchanged
		// sentinel. If these two ever collapse, a reset silently does nothing.
		{"zero resets to the server default", ptr(0), 0},
		{"half a DCR", ptr(0.5), 50_000_000},
		{"one DCR", ptr(1), 100_000_000},
		{"fifty DCR", ptr(50), 5_000_000_000},
		{"one atom", ptr(0.00000001), 1},
	}
	for _, tt := range tests {
		got, err := bondCeilingAtoms(tt.in, dcrAsset)
		if err != nil {
			t.Errorf("%s: bondCeilingAtoms() err = %v, want nil", tt.name, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s: bondCeilingAtoms() = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestBondCeilingAtomsRejectsBadValues(t *testing.T) {
	for name, in := range map[string]*float64{
		"negative": ptr(-1),
		"NaN":      ptr(math.NaN()),
		"infinity": ptr(math.Inf(1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := bondCeilingAtoms(in, dcrAsset); err == nil {
				t.Errorf("bondCeilingAtoms(%v) err = nil, want an error", *in)
			}
		})
	}
}

// maxBondedDcr must stay optional: omitting it is how an agent leaves the
// existing ceiling alone.
func TestSetBondOptionsInputOptionalFields(t *testing.T) {
	req := requiredSet[dexSetBondOptionsInput](t)
	if !req["host"] {
		t.Error("host is not required, want required")
	}
	for _, f := range []string{"maxBondedDcr", "targetTier", "bondAssetId", "penaltyComps"} {
		if req[f] {
			t.Errorf("%s is marked required, want optional (omit = leave unchanged)", f)
		}
	}
}

// The ceiling is denominated in the BOND ASSET, which bondAssetId may set to
// something other than DCR, so the conversion must follow the asset's own scale
// rather than assuming 1e8. This is what the HTTP handler for the same field
// already does.
func TestBondCeilingAtomsUsesTheBondAssetScale(t *testing.T) {
	const btc = uint32(0) // dcrdex asset 0 is BTC
	got, err := bondCeilingAtoms(ptr(1), btc)
	if err != nil {
		t.Fatalf("bondCeilingAtoms() err = %v", err)
	}
	want := int(dexConvToAtoms(1, btc))
	if got != want {
		t.Errorf("bondCeilingAtoms(1, btc) = %d, want %d (the bond asset's factor, not DCR's)", got, want)
	}
}
