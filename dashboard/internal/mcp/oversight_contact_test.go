// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const overseer = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"

func contactEntry(uid, nick, alias, name string) map[string]any {
	return map[string]any{
		"id":         map[string]any{"identity": uid, "nick": nick, "name": name},
		"nick_alias": alias,
	}
}

// The BR tools take a nick, alias or hex uid and let brclientd resolve it, so a
// refusal that only knows the stored hex is walked around by passing the nick.
func TestNamesOversightContactCoversEveryName(t *testing.T) {
	entries := []map[string]any{
		contactEntry(overseer, "phone", "myphone", "Operator Phone"),
		contactEntry("ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111", "someoneelse", "friend", "A Friend"),
	}
	for _, tc := range []struct {
		name   string
		target string
		want   bool
	}{
		{name: "hex uid", target: overseer, want: true},
		{name: "hex uid, different case", target: "A1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F90", want: true},
		{name: "nick", target: "phone", want: true},
		{name: "local alias", target: "myphone", want: true},
		{name: "display name", target: "Operator Phone", want: true},
		{name: "nick with padding", target: "  phone  ", want: true},
		{name: "another contact's uid", target: "ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111"},
		{name: "another contact's nick", target: "someoneelse"},
		{name: "another contact's alias", target: "friend"},
		{name: "unknown name", target: "nobody"},
		{name: "empty", target: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := namesOversightContact(tc.target, overseer, entries); got != tc.want {
				t.Errorf("namesOversightContact(%q) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}

// A contact whose entry carries no nick or alias must still be matched by uid,
// and empty fields must never match an empty-ish target.
func TestNamesOversightContactHandlesSparseEntries(t *testing.T) {
	entries := []map[string]any{contactEntry(overseer, "", "", "")}
	if !namesOversightContact(overseer, overseer, entries) {
		t.Error("the uid must match even when the contact has no names")
	}
	if namesOversightContact("", overseer, entries) {
		t.Error("an empty target must not match empty name fields")
	}
	if namesOversightContact("   ", overseer, entries) {
		t.Error("a blank target must not match empty name fields")
	}
}

func TestNamesOversightContactIgnoresMalformedEntries(t *testing.T) {
	entries := []map[string]any{
		{"nick_alias": "orphan"},
		{"id": "not a map"},
		contactEntry(overseer, "phone", "", ""),
	}
	if !namesOversightContact("phone", overseer, entries) {
		t.Error("a well-formed entry must still match past malformed ones")
	}
	if namesOversightContact("orphan", overseer, entries) {
		t.Error("an entry with no uid must not match the contact")
	}
}

// withOversight turns oversight on for a fixed contact and stands in for the
// operator, approving whatever is asked. Both seams are needed: without the
// first, oversight reads as off in tests and every refusal is a silent no-op;
// without the second, every gated tool stops at the approval message and never
// reaches its own body.
func withOversight(t *testing.T, contact string) {
	t.Helper()
	prevCfg, prevSend := oversightSettings, sendApprovalPM
	oversightSettings = func() (bool, string) { return true, contact }
	sendApprovalPM = func(_ context.Context, _, msg string) error {
		open, close := strings.Index(msg, "["), strings.Index(msg, "]")
		if open < 0 || close < open {
			return fmt.Errorf("no approval id in %q", msg)
		}
		id := msg[open+1 : close]
		// The operator says yes, from another goroutine: gateApproval is about
		// to block on the reply and this send is what unblocks it.
		go approvals.resolve(id, approvalVerdict{approved: true})
		return nil
	}
	t.Cleanup(func() { oversightSettings, sendApprovalPM = prevCfg, prevSend })
}

// br_tip_user reserves the tip against the caps before it checks the recipient,
// so refusing the oversight contact has to hand the reservation back. Otherwise
// the tip spends nothing and still burns the daily cap, repeatably.
func TestTipRefundsWhenTheOversightContactIsRefused(t *testing.T) {
	const agentID = "tip-oversight-refund"
	withOversight(t, overseer)
	grants.set(agentID, GrantSpec{
		WriteScopes: []string{scopeLightning},
		PerTxAtoms:  dcrAtoms, DailyAtoms: dcrAtoms,
	}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	if info, ok := SpendGrantInfo(agentID); !ok || info.SpentAtoms != 0 {
		t.Fatalf("the grant did not install clean: ok=%v spent=%d", ok, info.SpentAtoms)
	}

	cs := connectTo(t, testAgent(agentID, "tipper", map[string]bool{"bisonrelay": true}))
	// The hex uid matches the contact directly, so the refusal lands before the
	// brclientd contact lookup and this needs no daemon.
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "br_tip_user",
		Arguments: map[string]any{"uid": overseer, "amountDcr": 0.5},
	})
	if err == nil && !res.IsError {
		t.Fatal("tipping the oversight contact was allowed")
	}
	// Assert the refusal specifically. Any error would otherwise do, and with
	// oversight reading as off the tool falls through to brclientd, fails there
	// for want of a daemon, and refunds on that path instead.
	if got := resultText(res); !strings.Contains(got, "approval requests") {
		t.Fatalf("the call failed for the wrong reason, so this asserts nothing: %q", got)
	}

	info, ok := SpendGrantInfo(agentID)
	if !ok {
		t.Fatal("the grant is gone; the tripwire fired when it should not have")
	}
	if info.SpentAtoms != 0 {
		t.Errorf("SpentAtoms = %d, want 0: the refusal spent nothing but kept the reservation",
			info.SpentAtoms)
	}
}
