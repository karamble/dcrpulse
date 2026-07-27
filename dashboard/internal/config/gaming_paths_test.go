// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import "testing"

// The sandbox's portal reads this exact path, and it is a separate program in a
// separate container built from a separate module - so nothing but agreement
// between two string literals makes the policy arrive.
//
// It did not agree once: this returned /control/control/gaming.json while the
// portal read /control/gaming.json, so the policy was written where nothing
// looked and no game could ever start. Neither side was wrong on its own, which
// is why it needs pinning here rather than being left to review.
const portalReadsGamingPolicyAt = "/control/gaming.json"

func TestTheGamingPolicyIsWrittenWhereTheSandboxReadsIt(t *testing.T) {
	if got := GamingSettingsPath(); got != portalReadsGamingPolicyAt {
		t.Fatalf("dashboard writes the gaming policy to %q, sandbox reads %q",
			got, portalReadsGamingPolicyAt)
	}
}

// The state file travels the other way, out of the sandbox, and has the same
// property: the portal writes /data/control-state.json on its own volume, which
// the dashboard mounts read-only somewhere else entirely.
func TestTheSandboxStateIsReadWhereThePortalWritesIt(t *testing.T) {
	if got, want := GamingStatePath(), "/gaming-data/control-state.json"; got != want {
		t.Fatalf("dashboard reads the sandbox state at %q, want %q", got, want)
	}
}
