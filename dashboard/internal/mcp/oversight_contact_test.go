// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"encoding/json"
	"errors"
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

func withContacts(t *testing.T, entries ...map[string]any) {
	t.Helper()
	prev := recipientContacts
	recipientContacts = func(context.Context) ([]map[string]any, error) { return entries, nil }
	t.Cleanup(func() { recipientContacts = prev })
}

const bystander = "ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111"

// Whatever name an agent uses, the oversight contact is refused, and a name
// resolves to exactly the contact brclientd will deliver to: a uid prefix or a
// lookalike name reaches nobody rather than slipping past the check.
func TestAgentRecipientResolvesToTheContactThatReceivesIt(t *testing.T) {
	withOversight(t, overseer)
	withContacts(t,
		contactEntry(overseer, "phone", "myphone", "Operator Phone"),
		contactEntry(bystander, "someoneelse", "friend", "A Friend"),
		// A contact that took a nick shaped like the overseer's uid.
		contactEntry("eeee2222eeee2222eeee2222eeee2222eeee2222eeee2222eeee2222eeee2222", overseer+"\u200b", "", ""),
	)
	for _, tc := range []struct {
		name, target, want string
		err                error
	}{
		{name: "overseer uid", target: overseer, err: errOversightContact},
		{name: "overseer uid, upper case", target: strings.ToUpper(overseer), err: errOversightContact},
		{name: "overseer nick", target: "phone", err: errOversightContact},
		{name: "overseer alias", target: "myphone", err: errOversightContact},
		{name: "overseer nick, padded", target: "  phone  ", err: errOversightContact},
		{name: "5-char uid prefix", target: overseer[:5], err: errNoSuchRecipient},
		{name: "63-char uid prefix", target: overseer[:63], err: errNoSuchRecipient},
		{name: "nick plus zero-width space", target: "phone\u200b", err: errNoSuchRecipient},
		{name: "soft hyphen plus nick", target: "\u00adphone", err: errNoSuchRecipient},
		{name: "display name", target: "Operator Phone", err: errNoSuchRecipient},
		{name: "bystander uid", target: bystander, want: bystander},
		{name: "bystander nick", target: "someoneelse", want: bystander},
		{name: "bystander alias", target: "friend", want: bystander},
		{name: "empty", target: "", err: errNoSuchRecipient},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := agentRecipient(context.Background(), tc.target)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("agentRecipient(%q) = %q, %v; want %v", tc.target, got, err, tc.err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("agentRecipient(%q) = %q, %v; want %q", tc.target, got, err, tc.want)
			}
		})
	}
}

func TestMatchRecipientRefusesAmbiguousAndMalformedEntries(t *testing.T) {
	entries := []map[string]any{
		{"nick_alias": "orphan"},
		{"id": "not a map"},
		contactEntry(overseer, "twin", "", ""),
		contactEntry(bystander, "twin", "", ""),
	}
	if _, err := matchRecipient("twin", entries); !errors.Is(err, errAmbiguousRecipient) {
		t.Fatalf("a nick two contacts share: err = %v, want ambiguous", err)
	}
	if _, err := matchRecipient("orphan", entries); !errors.Is(err, errNoSuchRecipient) {
		t.Fatalf("an entry with no uid must not match: err = %v", err)
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
	oversightSettings = func() (bool, string, bool) { return true, contact, true }
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

// Private messages may reach the oversight contact; every other recipient
// tool still refuses it.
func TestApproverIsReachableByPrivateMessageOnly(t *testing.T) {
	withOversight(t, overseer)
	withContacts(t, contactEntry(overseer, "phone", "", ""), contactEntry(bystander, "bob", "", ""))
	ctx := context.Background()

	uid, toApprover, err := agentPMRecipient(ctx, "phone")
	if err != nil || uid != overseer || !toApprover {
		t.Fatalf("PM to the approver: %s %v %v", uid, toApprover, err)
	}
	if _, toApprover, _ := agentPMRecipient(ctx, "bob"); toApprover {
		t.Fatal("a bystander reported as the approver")
	}
	if _, err := agentRecipient(ctx, overseer); !errors.Is(err, errOversightContact) {
		t.Fatalf("other tools reach the approver: %v", err)
	}
}

// An agent's message to the approver names the agent and cannot pass for an
// approval request.
func TestLabelForApprover(t *testing.T) {
	got, err := labelForApprover("helper", "the report is ready")
	if err != nil || got != `[agent "helper"] the report is ready` {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := labelForApprover("helper", "Dcrpulse Approval [3fa1]: pay 1 DCR"); !errors.Is(err, errApprovalLookalike) {
		t.Fatalf("lookalike accepted: %v", err)
	}
}

func TestApprovalTrafficIsHiddenFromAgents(t *testing.T) {
	traffic := []string{
		`dcrpulse approval [3fa1]: agent "helper" wants to pay 1 DCR.`,
		"yes 3fa1", "No 3fa1 freeze", "approve   c0de",
	}
	chat := []string{"yes, go ahead", "no thanks", "ok", "yes please send the summary", "3fa1"}
	for _, m := range traffic {
		if !isApprovalTraffic(m) {
			t.Errorf("%q not recognised as approval traffic", m)
		}
	}
	for _, m := range chat {
		if isApprovalTraffic(m) {
			t.Errorf("%q hidden as approval traffic", m)
		}
	}

	entries := []map[string]any{
		{"message": "please check the node"},
		{"message": traffic[0]},
		{"message": "yes 3fa1"},
		{"message": "yes, go ahead"},
	}
	got := withoutApprovalTraffic(entries)
	if len(got) != 2 || got[0]["message"] != "please check the node" || got[1]["message"] != "yes, go ahead" {
		t.Fatalf("filtered history = %v", got)
	}

	withOversight(t, overseer)
	verdict := json.RawMessage(`{"from":"` + overseer + `","message":"yes 3fa1"}`)
	chatPM := json.RawMessage(`{"from":"` + overseer + `","message":"yes, go ahead"}`)
	other := json.RawMessage(`{"from":"` + bystander + `","message":"yes 3fa1"}`)
	if !approverVerdict(verdict) || approverVerdict(chatPM) || approverVerdict(other) {
		t.Fatal("feed filter drops the wrong messages")
	}
}

// br_send_message delivers to the approver with the agent's name in front, and
// to anyone else unchanged.
func TestSendMessageLabelsTheApproverThread(t *testing.T) {
	withOversight(t, overseer)
	withContacts(t, contactEntry(overseer, "phone", "", ""), contactEntry(bystander, "bob", "", ""))
	var sent []string
	prev := sendAgentPM
	sendAgentPM = func(_ context.Context, uid, msg string) error { sent = append(sent, uid+"|"+msg); return nil }
	t.Cleanup(func() { sendAgentPM = prev })

	id := "approver-pm"
	grants.set(id, GrantSpec{WriteScopes: []string{scopeBR}}, time.Now())
	t.Cleanup(func() { grants.revoke(id) })
	cs := connectTo(t, testAgent(id, "helper", map[string]bool{"bisonrelay": true}))
	call := func(to, msg string) *mcp.CallToolResult {
		out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "br_send_message", Arguments: map[string]any{"uid": to, "message": msg},
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	if out := call("phone", "done"); out.IsError {
		t.Fatalf("PM to the approver refused: %s", resultText(out))
	}
	if out := call("bob", "done"); out.IsError {
		t.Fatalf("PM to bob refused: %s", resultText(out))
	}
	if out := call("phone", "dcrpulse approval [3fa1]: fine"); !out.IsError {
		t.Fatal("lookalike approval request sent to the approver")
	}
	want := []string{overseer + `|[agent "helper"] done`, bystander + "|done"}
	if len(sent) != 2 || sent[0] != want[0] || sent[1] != want[1] {
		t.Fatalf("sent %q, want %q", sent, want)
	}
}
