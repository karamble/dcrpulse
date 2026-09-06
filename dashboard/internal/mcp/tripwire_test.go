// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"errors"
	"strings"
	"testing"
)

func TestIsOverLimit(t *testing.T) {
	for _, e := range []error{errPerTxExceeded, errDailyExceeded} {
		if !isOverLimit(e) {
			t.Errorf("%v should trip the tripwire", e)
		}
	}
	// Wrong-account / non-allowlisted / no-grant / expired / bad-amount are
	// denials but must NOT trip the tripwire (likelier honest mistakes).
	for _, e := range []error{errAccountNotGranted, errAddrNotAllowed, errNoGrant, errGrantExpired, errBadAmount} {
		if isOverLimit(e) {
			t.Errorf("%v should not trip the tripwire", e)
		}
	}
}

// withSaveAgents swaps the roster writer so the failure branch is reachable
// without an unwritable data volume, and restores it afterwards.
func withSaveAgents(t *testing.T, fn func() error) {
	t.Helper()
	prev := saveAgentsFn
	saveAgentsFn = fn
	t.Cleanup(func() { saveAgentsFn = prev })
}

// registerTestAgent puts an agent in the process-wide registry and takes it out
// again, since reg is global.
func registerTestAgent(t *testing.T, id, name string) {
	t.Helper()
	reg.addToken(id, name, id+"-token")
	t.Cleanup(func() { reg.remove(id) })
}

// The block is in force in this process either way, but if it never reaches the
// file a restart accepts the token again. That has to be said somewhere rather
// than swallowed.
func TestBlockAndPersistReportsAFailedSave(t *testing.T) {
	const agentID = "block-save-fails"
	sb := captureMCPLog(t)
	registerTestAgent(t, agentID, "unlucky")
	withSaveAgents(t, func() error { return errors.New("read-only file system") })

	blockAndPersist(agentID)

	if a := reg.agent(agentID); a == nil || !a.blocked.Load() {
		t.Fatal("the agent is not blocked in this process, which must happen regardless of the save")
	}
	got := sb.String()
	if !strings.Contains(got, agentID) {
		t.Errorf("the failure does not name the agent: %q", got)
	}
	if !strings.Contains(got, "read-only file system") {
		t.Errorf("the failure does not carry the reason: %q", got)
	}
}

func TestBlockAndPersistIsQuietWhenSaved(t *testing.T) {
	const agentID = "block-save-ok"
	sb := captureMCPLog(t)
	registerTestAgent(t, agentID, "fine")
	withSaveAgents(t, func() error { return nil })

	blockAndPersist(agentID)

	if got := sb.String(); strings.Contains(got, agentID) {
		t.Errorf("a successful save still complained: %q", got)
	}
	if a := reg.agent(agentID); a == nil || !a.blocked.Load() {
		t.Error("the agent was not blocked")
	}
}

// auditRowsFor picks this test's rows out of the process-wide ring, which other
// tests in the package have already written to.
func auditRowsFor(agentID string) []AuditEntry {
	var out []AuditEntry
	for _, e := range AuditLog(auditMax) {
		if e.AgentID == agentID {
			out = append(out, e)
		}
	}
	return out
}

// The tripwire's call sites each record their own row naming the tool that
// tripped, so the shared helper must not add one or every trip is recorded twice.
func TestTripwireDoesNotRecordItsOwnRow(t *testing.T) {
	const agentID = "trip-no-double"
	useTempAuditFile(t)
	registerTestAgent(t, agentID, "tripper")
	withSaveAgents(t, func() error { return nil })

	if !tripwire(agentID, errPerTxExceeded) {
		t.Fatal("the tripwire did not fire on an over-cap error")
	}
	if got := len(auditRowsFor(agentID)); got != 0 {
		t.Errorf("the tripwire wrote %d audit rows of its own; its call sites already record one", got)
	}
}

// An operator's explicit freeze is the one block with no tool call behind it, so
// without a row of its own it leaves no trace in the trail the operator goes to
// afterwards.
func TestFreezeRecordsAnAuditRow(t *testing.T) {
	const agentID = "freeze-row"
	useTempAuditFile(t)
	registerTestAgent(t, agentID, "frozen")
	withSaveAgents(t, func() error { return nil })

	freezeAgent(agentID)

	rows := auditRowsFor(agentID)
	if len(rows) != 1 {
		t.Fatalf("freezeAgent wrote %d audit rows, want exactly 1: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.AgentID != agentID || row.Agent != "frozen" {
		t.Errorf("the row does not name the frozen agent: %+v", row)
	}
	if row.Result != "blocked" {
		t.Errorf("Result = %q, want \"blocked\"", row.Result)
	}
	if !strings.Contains(row.Detail, "freeze") {
		t.Errorf("the row does not say it was an operator freeze: %q", row.Detail)
	}
}
