// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPersistedAgentAllowedIPsRoundTrip(t *testing.T) {
	r1 := newRegistry()
	hash := sha256.Sum256([]byte("tok"))
	r1.addAgentRecord("id1", "bot", hash, []string{"node"}, []string{"10.0.0.0/8"}, time.Now(), false)

	snap := r1.snapshot()
	if len(snap) != 1 || !reflect.DeepEqual(snap[0].AllowedIPs, []string{"10.0.0.0/8"}) {
		t.Fatalf("snapshot missing allowed IPs: %+v", snap)
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var recs []persistedAgent
	if err := json.Unmarshal(raw, &recs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	r2 := newRegistry()
	for _, rec := range recs {
		r2.addAgentRecord(rec.ID, rec.Name, hash, rec.Domains, rec.AllowedIPs, rec.CreatedAt, rec.Blocked)
	}
	a := r2.agents["id1"]
	if a == nil {
		t.Fatal("restored agent missing")
	}
	if !a.remoteAllowed("10.1.2.3:99") {
		t.Fatal("restored allowlist rejected an in-range remote")
	}
	if a.remoteAllowed("192.0.2.1:1") {
		t.Fatal("restored allowlist accepted an out-of-range remote")
	}
}

func TestPersistedAgentAllowedIPsBackCompat(t *testing.T) {
	// A record written before the field existed carries no allowedIps key and
	// must restore as unrestricted.
	var rec persistedAgent
	legacy := `{"id":"old","name":"old-bot","tokenHash":"ab","domains":["node"],"createdAt":"2026-01-01T00:00:00Z"}`
	if err := json.Unmarshal([]byte(legacy), &rec); err != nil {
		t.Fatalf("unmarshal legacy record: %v", err)
	}
	if rec.AllowedIPs != nil {
		t.Fatalf("legacy record should have nil AllowedIPs, got %v", rec.AllowedIPs)
	}
	r := newRegistry()
	r.addAgentRecord(rec.ID, rec.Name, sha256.Sum256([]byte("t")), rec.Domains, rec.AllowedIPs, rec.CreatedAt, rec.Blocked)
	if !r.agents["old"].remoteAllowed("203.0.113.5:1") {
		t.Fatal("legacy agent without allowed IPs must accept any remote")
	}

	// An unrestricted agent's snapshot omits the key entirely, keeping the
	// on-disk shape of pre-feature files unchanged.
	raw, err := json.Marshal(r.snapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if strings.Contains(string(raw), "allowedIps") {
		t.Fatalf("unrestricted snapshot should omit allowedIps: %s", raw)
	}
}

func TestPersistedAgentAllowedIPsDrift(t *testing.T) {
	// Unparseable entries can only come from a hand-edited config file; they
	// are dropped with a warning while the valid remainder stays enforced.
	sb := captureMCPLog(t)
	r := newRegistry()
	r.addAgentRecord("id1", "bot", sha256.Sum256([]byte("t")), []string{"node"},
		[]string{"nonsense", "10.0.0.1"}, time.Now(), false)
	a := r.agents["id1"]
	if got := a.allowedIPEntries(); !reflect.DeepEqual(got, []string{"10.0.0.1"}) {
		t.Fatalf("invalid entry not dropped: %v", got)
	}
	if !strings.Contains(sb.String(), "nonsense") {
		t.Fatalf("dropped entry not logged: %q", sb.String())
	}
	if !a.remoteAllowed("10.0.0.1:5") || a.remoteAllowed("10.0.0.2:5") {
		t.Fatal("remaining entry not enforced")
	}
}
