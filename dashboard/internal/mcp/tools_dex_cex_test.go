// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The name is what the credentials bind to upstream, where storing them
// replaces any existing entry for the same exchange. A blob that names none is
// refused rather than approved blind.
func TestCexConfigName(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
		wantErr         bool
	}{
		{name: "named", raw: `{"name":"Binance","apiKey":"k","apiSecret":"s"}`, want: "Binance"},
		{name: "padded", raw: `{"name":"  BinanceUS  "}`, want: "BinanceUS"},
		{name: "no name member", raw: `{"apiKey":"k"}`, wantErr: true},
		{name: "blank name", raw: `{"name":"   "}`, wantErr: true},
		{name: "not an object", raw: `nonsense`, wantErr: true},
		{name: "empty", raw: ``, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cexConfigName(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("cexConfigName(%q) = %q, want an error", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("cexConfigName(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("cexConfigName(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// withApprovalCapture turns oversight on and records what the operator was asked
// to approve, answering yes. Both seams are needed or the gated path is never
// reached: see the oversight contact tests.
func withApprovalCapture(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var asked []string

	prevCfg, prevSend := oversightSettings, sendApprovalPM
	oversightSettings = func() (bool, string) { return true, overseer }
	sendApprovalPM = func(_ context.Context, _, msg string) error {
		mu.Lock()
		asked = append(asked, msg)
		mu.Unlock()
		open, close := strings.Index(msg, "["), strings.Index(msg, "]")
		if open < 0 || close < open {
			return nil
		}
		go approvals.resolve(msg[open+1:close], approvalVerdict{approved: true})
		return nil
	}
	t.Cleanup(func() { oversightSettings, sendApprovalPM = prevCfg, prevSend })

	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), asked...)
	}
}

// Storing CEX credentials decides which exchange account a market-maker bot the
// operator starts later deposits into, and that send never passes a DCR cap. So
// it is approved rather than merely scoped, the prompt says which exchange, and
// the audit row records it.
func TestCexConfigWriteIsApprovedAndNamesTheExchange(t *testing.T) {
	const agentID = "dex-cex-approval"
	useTempAuditFile(t)
	asked := withApprovalCapture(t)
	grants.set(agentID, GrantSpec{WriteScopes: []string{scopeDex}}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	cs := connectTo(t, testAgent(agentID, "dex agent", map[string]bool{"dex": true}))
	// The DEX is locked in tests, so this fails after the gate. That is enough:
	// the approval and the audit row both happen before the daemon is reached.
	_, _ = cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "dex_mm_update_cex_config",
		Arguments: map[string]any{"config": `{"name":"Binance","apiKey":"k","apiSecret":"s"}`},
	})

	msgs := asked()
	if len(msgs) != 1 {
		t.Fatalf("the operator was asked %d times, want exactly 1: a plain scope check does not ask at all", len(msgs))
	}
	if !strings.Contains(msgs[0], "Binance") {
		t.Errorf("the approval does not say which exchange, so it cannot be judged: %q", msgs[0])
	}

	rows := AuditLog(1)
	if len(rows) != 1 {
		t.Fatalf("no audit row was written")
	}
	if rows[0].Tool != "dex_mm_update_cex_config" {
		t.Fatalf("unexpected audit row: %+v", rows[0])
	}
	if rows[0].Target != "Binance" {
		t.Errorf("audit target = %q, want Binance: the row must say which exchange changed", rows[0].Target)
	}
}

// A config naming no exchange cannot be approved meaningfully, so it is refused
// before the operator is asked.
func TestCexConfigWithoutAnExchangeIsRefused(t *testing.T) {
	const agentID = "dex-cex-unnamed"
	useTempAuditFile(t)
	asked := withApprovalCapture(t)
	grants.set(agentID, GrantSpec{WriteScopes: []string{scopeDex}}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	cs := connectTo(t, testAgent(agentID, "dex agent", map[string]bool{"dex": true}))
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "dex_mm_update_cex_config",
		Arguments: map[string]any{"config": `{"apiKey":"k","apiSecret":"s"}`},
	})
	if err == nil && !res.IsError {
		t.Fatal("a config naming no exchange was accepted")
	}
	if n := len(asked()); n != 0 {
		t.Errorf("the operator was asked %d times about a config that names no exchange", n)
	}
}
