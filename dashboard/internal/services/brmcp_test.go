// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"encoding/json"
	"strings"
	"testing"
)

// The settings decoder converts atoms to DCR, turns absent lists into empty
// ones so they marshal as [], and passes the reply-only last_denied through.
func TestDecodeBRMCPSettings(t *testing.T) {
	raw := json.RawMessage(`{
		"enabled": true,
		"token": "secret",
		"mode": "approval",
		"per_call_cap_atoms": 100000000,
		"per_day_cap_atoms": 250000000,
		"allowed_bots": null,
		"allowed_ips": ["10.0.0.7"],
		"approval_timeout_secs": 120,
		"tip_wait_secs": 30,
		"last_denied": {"ip": "9.9.9.9", "at": "2026-09-01T10:00:00Z"}
	}`)
	got, err := DecodeBRMCPSettings(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Enabled || got.Token != "secret" || got.Mode != "approval" {
		t.Errorf("scalar fields = %+v", got)
	}
	if got.PerCallCapDcr != 1 {
		t.Errorf("PerCallCapDcr = %v, want 1", got.PerCallCapDcr)
	}
	if got.PerDayCapDcr != 2.5 {
		t.Errorf("PerDayCapDcr = %v, want 2.5", got.PerDayCapDcr)
	}
	if got.AllowedBots == nil || len(got.AllowedBots) != 0 {
		t.Errorf("AllowedBots = %#v, want empty non-nil", got.AllowedBots)
	}
	if len(got.AllowedIPs) != 1 || got.AllowedIPs[0] != "10.0.0.7" {
		t.Errorf("AllowedIPs = %#v", got.AllowedIPs)
	}
	if got.ApprovalTimeoutSecs != 120 || got.TipWaitSecs != 30 {
		t.Errorf("timeouts = %d/%d", got.ApprovalTimeoutSecs, got.TipWaitSecs)
	}
	if got.LastDenied == nil || got.LastDenied.IP != "9.9.9.9" || got.LastDenied.At != "2026-09-01T10:00:00Z" {
		t.Errorf("LastDenied = %+v", got.LastDenied)
	}

	if _, err := DecodeBRMCPSettings(json.RawMessage(`not json`)); err == nil || !strings.HasPrefix(err.Error(), "parse settings: ") {
		t.Errorf("garbage settings error = %v, want the parse settings prefix", err)
	}
}

func TestDecodeBRMCPPending(t *testing.T) {
	got, err := DecodeBRMCPPending(json.RawMessage(`{"pending":[
		{"id":"p1","bot":"aa","tool":"quote","atoms":100000000,"created":1700000000},
		{"id":"p2","bot":"bb","tool":"image","atoms":5000000,"created":1700000060}
	]}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "p1" || got[0].Bot != "aa" || got[0].Tool != "quote" || got[0].AmountDcr != 1 || got[0].Created != 1700000000 {
		t.Errorf("entry 0 = %+v", got[0])
	}
	if got[1].AmountDcr != 0.05 {
		t.Errorf("entry 1 AmountDcr = %v, want 0.05", got[1].AmountDcr)
	}

	empty, err := DecodeBRMCPPending(json.RawMessage(`{"pending":null}`))
	if err != nil {
		t.Fatalf("decode null: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("null pending = %#v, want empty non-nil", empty)
	}

	if _, err := DecodeBRMCPPending(json.RawMessage(`[]`)); err == nil || !strings.HasPrefix(err.Error(), "parse pending: ") {
		t.Errorf("garbage pending error = %v, want the parse pending prefix", err)
	}
}

func TestDecodeBRMCPSpend(t *testing.T) {
	got, err := DecodeBRMCPSpend(json.RawMessage(`{"entries":[
		{"ts":1700000000,"bot":"aa","tool":"quote","rail":"ln","atoms":100000000,"status":"ok","err":""},
		{"ts":1700000010,"bot":"aa","tool":"quote","rail":"ln","atoms":1,"status":"failed","err":"no route"}
	],"today_atoms":100000001}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("len = %d, want 2", len(got.Entries))
	}
	if e := got.Entries[0]; e.TS != 1700000000 || e.Rail != "ln" || e.AmountDcr != 1 || e.Status != "ok" || e.Err != "" {
		t.Errorf("entry 0 = %+v", e)
	}
	if e := got.Entries[1]; e.AmountDcr != 0.00000001 || e.Status != "failed" || e.Err != "no route" {
		t.Errorf("entry 1 = %+v", e)
	}
	if got.TodayDcr != 1.00000001 {
		t.Errorf("TodayDcr = %v, want 1.00000001", got.TodayDcr)
	}

	empty, err := DecodeBRMCPSpend(json.RawMessage(`{"entries":null,"today_atoms":0}`))
	if err != nil {
		t.Fatalf("decode null: %v", err)
	}
	if empty.Entries == nil || len(empty.Entries) != 0 {
		t.Errorf("null entries = %#v, want empty non-nil", empty.Entries)
	}
	if b, _ := json.Marshal(empty); string(b) != `{"entries":[],"todayDcr":0}` {
		t.Errorf("empty spend marshals as %s", b)
	}

	if _, err := DecodeBRMCPSpend(json.RawMessage(`{"entries":"nope"}`)); err == nil || !strings.HasPrefix(err.Error(), "parse spend: ") {
		t.Errorf("garbage spend error = %v, want the parse spend prefix", err)
	}
}
