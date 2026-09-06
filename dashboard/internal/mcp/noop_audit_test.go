// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/types"
)

// callGranted drives one write tool for an agent granted the scope it needs, and
// returns the audit row it wrote.
func callGranted(t *testing.T, agentID, domain, scope, tool string, args map[string]any) AuditEntry {
	t.Helper()
	grants.set(agentID, GrantSpec{WriteScopes: []string{scope}}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	cs := connectTo(t, testAgent(agentID, agentID, map[string]bool{domain: true}))
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args}); err != nil {
		t.Fatalf("calling %s: %v", tool, err)
	}
	rows := auditRowsFor(agentID)
	if len(rows) != 1 {
		t.Fatalf("%s wrote %d audit rows, want 1: %+v", tool, len(rows), rows)
	}
	return rows[0]
}

// An operator reading the feed must not see a successful action that never
// happened: the call was allowed, and it changed nothing.
func TestTorSetSettingsRecordsNoChange(t *testing.T) {
	useTempAuditFile(t)
	row := callGranted(t, "tor-noop", "tor", scopeTor, "tor_set_settings", map[string]any{})
	if row.Result != "unchanged" {
		t.Errorf("Result = %q, want \"unchanged\": the call changed nothing", row.Result)
	}
	if !strings.Contains(row.Detail, "no fields") {
		t.Errorf("the row does not say why nothing changed: %q", row.Detail)
	}
}

// The writer clamps an out-of-range circuit limit, so the row has to report the
// value that was persisted and not the one that was asked for.
func TestTorSettingsChangesReportsWhatWasWritten(t *testing.T) {
	base := types.TorSettings{Isolation: true, CircuitLimit: 64, Rev: 3}
	for _, tc := range []struct {
		name string
		next types.TorSettings
		want string
	}{{
		name: "a clamped limit reports the persisted value",
		next: types.TorSettings{Isolation: true, CircuitLimit: 32, Rev: 4},
		want: "circuitLimit=32",
	}, {
		name: "only the fields that moved are named",
		next: types.TorSettings{Enabled: true, Isolation: true, CircuitLimit: 64, Rev: 4},
		want: "enabled=true",
	}, {
		name: "several fields",
		next: types.TorSettings{Enabled: true, Isolation: false, CircuitLimit: 64, Rev: 4},
		want: "enabled=true isolation=false",
	}, {
		name: "a bumped revision alone is not a change",
		next: types.TorSettings{Isolation: true, CircuitLimit: 64, Rev: 4},
		want: "the request was clamped; nothing changed",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if got := torSettingsChanges(base, tc.next); got != tc.want {
				t.Errorf("torSettingsChanges = %q, want %q", got, tc.want)
			}
		})
	}
}

// Stopping something that was not running changes nothing, and the row has to
// say so rather than reporting a stop that did not happen.
func TestStopToolsRecordNoChange(t *testing.T) {
	useTempAuditFile(t)
	for _, tc := range []struct {
		agentID, domain, scope, tool, detail string
		args                                 map[string]any
	}{
		{
			agentID: "mixer-noop", domain: "privacy", scope: scopePrivacy,
			tool: "privacy_mixer_stop", detail: "mixer", args: map[string]any{},
		},
		{
			agentID: "buyer-noop", domain: "staking", scope: scopeStaking,
			tool: "staking_autobuyer_stop", detail: "ticket buyer", args: map[string]any{},
		},
		{
			agentID: "trickle-noop", domain: "governance", scope: scopeGovernance,
			tool: "governance_vote_trickle_stop", detail: "trickle run",
			args: map[string]any{"token": "0000000000000000"},
		},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			row := callGranted(t, tc.agentID, tc.domain, tc.scope, tc.tool, tc.args)
			if row.Result != "unchanged" {
				t.Errorf("Result = %q, want \"unchanged\": there was nothing to stop", row.Result)
			}
			if !strings.Contains(row.Detail, tc.detail) {
				t.Errorf("the row does not say what was not running: %q", row.Detail)
			}
		})
	}
}

// The new value must stay out of the operator's notification path: it is not a
// fund movement and not a block.
func TestUnchangedRowsAreNotNotified(t *testing.T) {
	for _, e := range []AuditEntry{
		{Agent: "a", Tool: "privacy_mixer_stop", Result: "unchanged"},
		{Agent: "a", Tool: "tor_set_settings", Result: "unchanged", AmountDCR: 1},
	} {
		if msg, want := spendNotice(e), ""; msg != want {
			t.Errorf("an unchanged row produced an operator message: %q", msg)
		}
	}
	// The two that must still be reported, so this is not asserting on a
	// function that reports nothing at all.
	if spendNotice(AuditEntry{Agent: "a", Tool: "wallet_send", Result: "ok", AmountDCR: 1}) == "" {
		t.Error("a completed spend is no longer reported to the operator")
	}
	if spendNotice(AuditEntry{Agent: "a", Tool: "wallet_send", Result: "blocked"}) == "" {
		t.Error("a block is no longer reported to the operator")
	}
}
