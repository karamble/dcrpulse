// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/services"
)

// The ceiling has to be the same curve dcrlnd falls back to when no fee limit
// reaches it, or a reservation built on it does not describe what gets spent:
// 100% at and below 1000 atoms, 5% above.
func TestRoutingFeeCeilingMirrorsDcrlnd(t *testing.T) {
	for _, tc := range []struct {
		name   string
		amount int64
		want   int64
	}{
		{name: "zero reserves nothing", amount: 0, want: 0},
		{name: "one atom", amount: 1, want: 1},
		{name: "at the 100% boundary", amount: 1000, want: 1000},
		{name: "just past the boundary", amount: 1001, want: 50},
		{name: "one DCR", amount: 100_000_000, want: 5_000_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := services.RoutingFeeCeilingAtoms(tc.amount); got != tc.want {
				t.Errorf("RoutingFeeCeilingAtoms(%d) = %d, want %d", tc.amount, got, tc.want)
			}
		})
	}
}

// callTip drives br_tip_user for an agent granted perTx and daily, and reports
// what came back.
func callTip(t *testing.T, agentID string, perTx, daily int64, amountDCR float64) (*mcp.CallToolResult, error) {
	t.Helper()
	grants.set(agentID, GrantSpec{
		WriteScopes: []string{scopeLightning, scopeBR},
		PerTxAtoms:  perTx,
		DailyAtoms:  daily,
	}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	cs := connectTo(t, testAgent(agentID, "tipper", map[string]bool{"bisonrelay": true}))
	return cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "br_tip_user",
		Arguments: map[string]any{"uid": strings.Repeat("ff00", 16), "amountDcr": amountDCR},
	})
}

// The per-transaction cap bounds the amount the agent chose, and dcrlnd's fee
// ceiling on top counts against the daily cap only. A daily cap short of just
// the fee is refused without treating the agent as compromised.
func TestTipCapsTheAmountAndBudgetsTheFee(t *testing.T) {
	const amountDCR = 0.5
	const atoms = int64(50_000_000)
	ceiling := services.RoutingFeeCeilingAtoms(atoms)
	if ceiling <= 0 {
		t.Fatalf("ceiling for %d atoms is %d; this test would assert nothing", atoms, ceiling)
	}
	useTempAuditFile(t)

	t.Run("a cap of exactly the amount passes", func(t *testing.T) {
		const id = "tip-cap-exact"
		res, _ := callTip(t, id, atoms, atoms+ceiling, amountDCR)
		if got := resultText(res); strings.Contains(got, "per-transaction cap") || strings.Contains(got, "routing fee") {
			t.Fatalf("a cap of exactly the amount was refused: %q", got)
		}
		if info, ok := SpendGrantInfo(id); !ok || info.SpentAtoms != atoms+ceiling {
			t.Fatalf("grant %v, spent %d: want it kept with amount+ceiling %d reserved", ok, info.SpentAtoms, atoms+ceiling)
		}
	})

	t.Run("one atom over the cap trips", func(t *testing.T) {
		const id = "tip-cap-over"
		res, _ := callTip(t, id, atoms-1, 10*atoms, amountDCR)
		if got := resultText(res); !strings.Contains(got, "per-transaction cap") {
			t.Fatalf("refused for the wrong reason: %q", got)
		}
		if _, ok := SpendGrantInfo(id); ok {
			t.Fatal("an amount over the cap left the grant installed")
		}
	})

	t.Run("a daily cap short of the fee alone denies without tripping", func(t *testing.T) {
		const id = "tip-fee-short"
		res, _ := callTip(t, id, atoms, atoms+ceiling-1, amountDCR)
		if got := resultText(res); !strings.Contains(got, "routing fee") {
			t.Fatalf("refused for the wrong reason: %q", got)
		}
		if info, ok := SpendGrantInfo(id); !ok || info.SpentAtoms != 0 {
			t.Fatalf("grant %v, spent %d: a fee-only shortfall must leave the grant untouched", ok, info.SpentAtoms)
		}
	})
}

// Bison Relay's tip flow is asynchronous: once brclientd has the tip, the
// invoice request and the payment carry on by themselves, so an error here does
// not mean the tip went unpaid. Refunding would hand back a cap that may still
// be spent.
func TestTipKeepsTheReservationOnceHandedOver(t *testing.T) {
	const agentID = "tip-no-refund"
	const amountDCR = 0.5
	const atoms = int64(50_000_000)
	want := atoms + services.RoutingFeeCeilingAtoms(atoms)
	useTempAuditFile(t)

	res, err := callTip(t, agentID, 100_000_000, 100_000_000, amountDCR)
	if err == nil && !res.IsError {
		t.Fatal("the tip reached brclientd in a test; this asserts nothing")
	}

	info, ok := SpendGrantInfo(agentID)
	if !ok {
		t.Fatal("the grant is gone; the tripwire fired when it should not have")
	}
	if info.SpentAtoms != want {
		t.Errorf("SpentAtoms = %d, want %d: a tip already handed to brclientd must keep its reservation",
			info.SpentAtoms, want)
	}
	rows := AuditLog(1)
	if len(rows) != 1 || rows[0].Tool != "br_tip_user" {
		t.Fatalf("no br_tip_user audit row was written: %+v", rows)
	}
	if !strings.Contains(rows[0].Detail, "may still be paid") {
		t.Errorf("the audit detail does not record that the reservation was kept: %q", rows[0].Detail)
	}
}

// The tip flow is asynchronous, so how many attempts to make is not the
// caller's decision. Upstream's own clients hardcode one; the knob is not
// offered here at all.
func TestTipOffersNoAttemptsKnob(t *testing.T) {
	cs := connectTo(t, testAgent("tip-schema", "tipper", map[string]bool{"bisonrelay": true}))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tl := range res.Tools {
		if tl.Name != "br_tip_user" {
			continue
		}
		schema, merr := json.Marshal(tl.InputSchema)
		if merr != nil {
			t.Fatalf("marshal schema: %v", merr)
		}
		if !strings.Contains(strings.ToLower(string(schema)), "amountdcr") {
			t.Fatalf("the schema does not look like br_tip_user's, so this asserts nothing: %s", schema)
		}
		if strings.Contains(strings.ToLower(string(schema)), "maxattempts") {
			t.Errorf("br_tip_user still advertises an attempts knob: %s", schema)
		}
		return
	}
	t.Fatal("br_tip_user missing from the listing")
}

// callFeeTool drives a Lightning-paying tool for an agent granted perTx and daily.
func callFeeTool(t *testing.T, agentID string, perTx, daily int64, domain, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	grants.set(agentID, GrantSpec{
		WriteScopes: []string{scopeLightning, scopeBR},
		PerTxAtoms:  perTx,
		DailyAtoms:  daily,
	}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	cs := connectTo(t, testAgent(agentID, "payer", map[string]bool{domain: true}))
	res, _ := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	return res
}

// The tip is not the only tool that pays over Lightning: the liquidity fee and a
// paid download carry a routing fee on top too, and each budgets it the same way.
// Asserted from both sides of the daily boundary, so the size of the reservation
// is what is being tested and not merely that a call failed.
func TestEveryLightningPayerReservesTheRoutingFee(t *testing.T) {
	const feeAtoms = int64(20_000_000) // 0.2 DCR
	ceiling := services.RoutingFeeCeilingAtoms(feeAtoms)
	if ceiling <= 0 {
		t.Fatalf("ceiling is %d; this test would assert nothing", ceiling)
	}

	for _, tc := range []struct {
		name, domain, tool string
		args               map[string]any
	}{
		{
			name: "ln_liquidity_request", domain: "lightning", tool: "ln_liquidity_request",
			args: map[string]any{"chanSizeDcr": 1.0, "approvedFeeDcr": 0.2},
		},
		{
			name: "br_content_get", domain: "bisonrelay", tool: "br_content_get",
			args: map[string]any{"uid": "ff00", "fid": "abcd", "maxCostAtoms": feeAtoms},
		},
	} {
		t.Run(tc.name+" a daily cap short of the fee is refused", func(t *testing.T) {
			id := tc.tool + "-short"
			res := callFeeTool(t, id, feeAtoms, feeAtoms+ceiling-1, tc.domain, tc.tool, tc.args)
			if got := resultText(res); !strings.Contains(got, "routing fee") {
				t.Fatalf("a daily cap that cannot cover the routing fee did not refuse: %q", got)
			}
			if _, ok := SpendGrantInfo(id); !ok {
				t.Fatal("a fee-only shortfall revoked the grant")
			}
		})
		t.Run(tc.name+" exactly enough gets past the caps", func(t *testing.T) {
			res := callFeeTool(t, tc.tool+"-exact", feeAtoms, feeAtoms+ceiling, tc.domain, tc.tool, tc.args)
			if got := resultText(res); strings.Contains(got, "per-transaction cap") || strings.Contains(got, "routing fee") {
				t.Fatalf("a grant of exactly fee+ceiling was refused, so the reservation is too large: %q", got)
			}
		})
	}
}

// Routing fees follow dcrlnd's default curve for every payment, so ln_pay offers
// no fee limit an agent could raise.
func TestLnPayOffersNoFeeLimit(t *testing.T) {
	cs := connectTo(t, testAgent("ln-pay-schema", "payer", map[string]bool{"lightning": true}))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tl := range res.Tools {
		if tl.Name != "ln_pay" {
			continue
		}
		schema, merr := json.Marshal(tl.InputSchema)
		if merr != nil {
			t.Fatalf("marshal schema: %v", merr)
		}
		if !strings.Contains(strings.ToLower(string(schema)), "payreq") {
			t.Fatalf("the schema does not look like ln_pay's, so this asserts nothing: %s", schema)
		}
		if strings.Contains(strings.ToLower(string(schema)), "feelimit") {
			t.Errorf("ln_pay still advertises a fee limit: %s", schema)
		}
		return
	}
	t.Fatal("ln_pay missing from the listing")
}
