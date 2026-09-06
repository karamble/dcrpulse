// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"testing"

	"dcrpulse/internal/types"
)

func boolp(b bool) *bool { return &b }
func intp(i int) *int    { return &i }

// live mirrors the local install: Tor fully on with a real dcrlnd hidden service.
var live = types.TorSettings{
	Enabled: true, Isolation: true, DcrdOnion: true,
	LnOnion: true, CircuitLimit: 32, Rev: 3,
}

// The tool exposes four of the six settings and must overlay them onto what is
// stored. lnOnion gates the Lightning node's inbound listener, so a write that
// rebuilds the struct takes the node off its hidden service.
func TestApplyTorSettingsNeverWritesLnOnion(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   torSetSettingsInput
	}{
		{name: "every field named", in: torSetSettingsInput{
			Enabled: boolp(false), Isolation: boolp(false),
			DcrdOnion: boolp(false), CircuitLimit: intp(64)}},
		{name: "nothing named", in: torSetSettingsInput{}},
		{name: "one field named", in: torSetSettingsInput{CircuitLimit: intp(64)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := applyTorSettings(live, tc.in)
			if !got.LnOnion {
				t.Error("lnOnion was cleared by a tool that does not expose it")
			}
			if got.Rev != live.Rev {
				t.Errorf("Rev = %d, want it carried from current (%d)", got.Rev, live.Rev)
			}
		})
	}
}

// An omitted field keeps its current value, so an agent that read settings,
// paused, then changed one thing cannot revert an operator change it never saw.
func TestApplyTorSettingsLeavesOmittedFieldsAlone(t *testing.T) {
	got, named := applyTorSettings(live, torSetSettingsInput{CircuitLimit: intp(64)})
	if got.CircuitLimit != 64 {
		t.Errorf("CircuitLimit = %d, want 64", got.CircuitLimit)
	}
	if !got.Enabled || !got.Isolation || !got.DcrdOnion {
		t.Errorf("an omitted field was overwritten: %+v", got)
	}
	if named != "circuitLimit=64" {
		t.Errorf("named = %q, want only the field that was set", named)
	}
}

func TestApplyTorSettingsAppliesNamedFields(t *testing.T) {
	off := types.TorSettings{LnOnion: true, CircuitLimit: 32, Rev: 1}
	got, named := applyTorSettings(off, torSetSettingsInput{
		Enabled: boolp(true), Isolation: boolp(true), DcrdOnion: boolp(true), CircuitLimit: intp(99),
	})
	if !got.Enabled || !got.Isolation || !got.DcrdOnion || got.CircuitLimit != 99 {
		t.Errorf("a named field was not applied: %+v", got)
	}
	if named == "" {
		t.Error("the audit detail must name what changed")
	}
}

// A named false must still be applied: omitted and false are different.
func TestApplyTorSettingsDistinguishesFalseFromOmitted(t *testing.T) {
	got, named := applyTorSettings(live, torSetSettingsInput{DcrdOnion: boolp(false)})
	if got.DcrdOnion {
		t.Error("an explicit false was ignored")
	}
	if !got.Enabled || !got.Isolation {
		t.Error("naming one field disturbed another")
	}
	if named != "dcrdOnion=false" {
		t.Errorf("named = %q, want dcrdOnion=false", named)
	}
}

// A write bumps Rev and relaunches every daemon, so the handler skips one that
// changes nothing. These pin the comparison that decision rests on.
func TestApplyTorSettingsComparesEqualWhenNothingChanges(t *testing.T) {
	if got, _ := applyTorSettings(live, torSetSettingsInput{}); got != live {
		t.Error("an empty input must compare equal, so the write is skipped")
	}
	same := torSetSettingsInput{Enabled: boolp(true), Isolation: boolp(true), DcrdOnion: boolp(true), CircuitLimit: intp(32)}
	if got, _ := applyTorSettings(live, same); got != live {
		t.Error("an input echoing current settings must compare equal")
	}
	for _, tc := range []struct {
		name string
		in   torSetSettingsInput
	}{
		{name: "enabled", in: torSetSettingsInput{Enabled: boolp(false)}},
		{name: "isolation", in: torSetSettingsInput{Isolation: boolp(false)}},
		{name: "dcrdOnion", in: torSetSettingsInput{DcrdOnion: boolp(false)}},
		{name: "circuitLimit", in: torSetSettingsInput{CircuitLimit: intp(64)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := applyTorSettings(live, tc.in); got == live {
				t.Error("a real change must compare unequal, or the write is wrongly skipped")
			}
		})
	}
}
