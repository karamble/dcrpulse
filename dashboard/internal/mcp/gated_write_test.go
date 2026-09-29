// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"errors"
	"strings"
	"testing"
	"time"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/rpc"
)

// Every outcome of a gated write leaves exactly one row, and the row says which
// of the three it was.
func TestGatedWriteRecordsOneRowPerOutcome(t *testing.T) {
	useTempAuditFile(t)
	errBody := errors.New("daemon said no")
	for _, tc := range []struct {
		name       string
		granted    bool
		do         func(target *string) (any, string, error)
		wantResult string
		wantTarget string
		wantDetail string
		wantErr    error
	}{{
		name:       "no grant: refused before the body runs",
		do:         func(*string) (any, string, error) { panic("the body ran without a grant") },
		wantResult: "denied",
		wantTarget: "t0",
	}, {
		name:       "the body fails",
		granted:    true,
		do:         func(*string) (any, string, error) { return nil, "", errBody },
		wantResult: "error",
		wantTarget: "t0",
		wantDetail: "daemon said no",
		wantErr:    errBody,
	}, {
		name:       "the body refuses by policy",
		granted:    true,
		do:         func(*string) (any, string, error) { return nil, "", denial{errNoSuchRecipient} },
		wantResult: "denied",
		wantTarget: "t0",
		wantDetail: errNoSuchRecipient.Error(),
		wantErr:    errNoSuchRecipient,
	}, {
		name:       "success carries the body's detail",
		granted:    true,
		do:         func(*string) (any, string, error) { return "out", "sent 3", nil },
		wantResult: "ok",
		wantTarget: "t0",
		wantDetail: "sent 3",
	}, {
		name:    "a resolved target reaches the row",
		granted: true,
		do: func(target *string) (any, string, error) {
			*target = "resolved"
			return "out", "", nil
		},
		wantResult: "ok",
		wantTarget: "resolved",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			id := "gated-" + strings.ReplaceAll(tc.name, " ", "-")
			if tc.granted {
				grants.set(id, GrantSpec{WriteScopes: []string{scopeBR}}, time.Now())
				t.Cleanup(func() { grants.revoke(id) })
			}
			target := "t0"
			out, err := gatedWrite(testAgent(id, id, nil), "br_test", scopeBR, &target,
				func() (any, string, error) { return tc.do(&target) })

			rows := auditRowsFor(id)
			if len(rows) != 1 {
				t.Fatalf("%d audit rows, want 1: %+v", len(rows), rows)
			}
			row := rows[0]
			if row.Tool != "br_test" || row.Result != tc.wantResult || row.Target != tc.wantTarget {
				t.Errorf("row = %s/%s/%s, want br_test/%s/%s", row.Tool, row.Result, row.Target, tc.wantResult, tc.wantTarget)
			}
			if tc.wantDetail != "" && row.Detail != tc.wantDetail {
				t.Errorf("detail = %q, want %q", row.Detail, tc.wantDetail)
			}
			if tc.wantResult == "ok" {
				if err != nil || out != "out" {
					t.Errorf("gatedWrite = %v, %v; want the body's result", out, err)
				}
				return
			}
			if err == nil || out != nil {
				t.Fatalf("gatedWrite = %v, %v; want a refusal", out, err)
			}
			if tc.wantErr != nil && (!errors.Is(err, tc.wantErr) || err.Error() != tc.wantErr.Error()) {
				t.Errorf("error = %v, want %v unchanged", err, tc.wantErr)
			}
		})
	}
}

// The tools record what they refused, including the failures that used to
// return without a row, and a refused recipient is named in the row.
func TestGatedToolsRecordRefusals(t *testing.T) {
	useTempAuditFile(t)
	withContacts(t)
	const gcid = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		name, domain, scope, tool string
		args                      map[string]any
		result, target, detail    string
	}{{
		name: "malformed rv", domain: "bisonrelay", scope: scopeBR, tool: "br_rtdt_join",
		args:   map[string]any{"rv": "zz"},
		result: "error", target: "zz", detail: rpc.ErrBadShortID.Error(),
	}, {
		name: "unknown message recipient", domain: "bisonrelay", scope: scopeBR, tool: "br_send_message",
		args:   map[string]any{"uid": "nobody", "message": "hi"},
		result: "denied", target: "nobody", detail: errNoSuchRecipient.Error(),
	}, {
		name: "unknown invitee", domain: "bisonrelay", scope: scopeBR, tool: "br_gc_invite",
		args:   map[string]any{"gcid": gcid, "uid": "nobody"},
		result: "denied", target: "nobody", detail: errNoSuchRecipient.Error(),
	}, {
		name: "locked DEX", domain: "dex", scope: scopeDex, tool: "dex_add_peer",
		args:   map[string]any{"assetId": 42, "address": "127.0.0.1:9108"},
		result: "error", target: "127.0.0.1:9108", detail: dexLocked().Error(),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			row := callGranted(t, "refusal-"+tc.tool, tc.domain, tc.scope, tc.tool, tc.args)
			if row.Result != tc.result || row.Target != tc.target || row.Detail != tc.detail {
				t.Errorf("row = %s/%q/%q, want %s/%q/%q", row.Result, row.Target, row.Detail, tc.result, tc.target, tc.detail)
			}
		})
	}
}

// A start refused by the allowance it shares with the dashboard is recorded as
// a refusal, before the DEX unlock is even looked at.
func TestRateLimitedDexWriteIsRecorded(t *testing.T) {
	useTempAuditFile(t)
	lim := middleware.DexRescan.Limiter()
	for lim.Allow() {
	}
	row := callGranted(t, "refusal-rescan", "dex", scopeDex, "dex_wallet_rescan", map[string]any{"assetId": 42})
	if row.Result != "denied" || !strings.Contains(row.Detail, "rate limit") || row.Target != "asset=42" {
		t.Errorf("row = %s/%q/%q, want a denied rate-limit row for asset=42", row.Result, row.Target, row.Detail)
	}
}
