// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"sync"
	"time"
	"unicode/utf8"
)

// AuditEntry records one agent spend attempt (allowed, denied, or failed) for
// display in the dashboard. It is in-memory only and bounded.
type AuditEntry struct {
	Time      time.Time `json:"time"`
	AgentID   string    `json:"agentId"`
	Agent     string    `json:"agent"`
	Tool      string    `json:"tool"`
	Account   uint32    `json:"account"`
	AmountDCR float64   `json:"amountDcr"`
	Target    string    `json:"target,omitempty"` // recipient address, VSP host, etc.
	Result    string    `json:"result"`           // ok | denied | error
	Detail    string    `json:"detail,omitempty"` // txid on success, else the reason
}

const auditMax = 200

// Audit fields carry caller-supplied strings - an address, a VSP host, a uid, or
// an error text that quotes the agent's own input - so they are bounded before
// they reach the ring, the persisted trail, the audit resource and the operator's
// Bison Relay notification. The budgets are generous against real values: a
// Decred address is ~35 chars and a Bison Relay uid 64.
const (
	auditTargetMax   = 256
	auditDetailMax   = 1024
	auditTruncMarker = "...[truncated]"
)

// clampAuditField bounds s to max bytes, cutting on a rune boundary and marking
// the cut, so a clipped value is never mistaken for a complete one.
func clampAuditField(s string, max int) string {
	if len(s) <= max {
		return s
	}
	keep := max - len(auditTruncMarker)
	if keep < 0 {
		keep = 0
	}
	for keep > 0 && !utf8.RuneStart(s[keep]) {
		keep--
	}
	return s[:keep] + auditTruncMarker
}

type auditLog struct {
	mu      sync.Mutex
	entries []AuditEntry
}

func (l *auditLog) record(e AuditEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	l.entries = append(l.entries, e)
	if len(l.entries) > auditMax {
		l.entries = l.entries[len(l.entries)-auditMax:]
	}
}

// recent returns up to n entries, newest first.
func (l *auditLog) recent(n int) []AuditEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n <= 0 || n > len(l.entries) {
		n = len(l.entries)
	}
	out := make([]AuditEntry, n)
	for i := 0; i < n; i++ {
		out[i] = l.entries[len(l.entries)-1-i]
	}
	return out
}

var audit = &auditLog{}

var auditNotify = &coalescedNotifier{
	uri:    resAudit,
	notify: notifyResourceUpdated,
	ch:     make(chan struct{}, 1),
}

// AuditLog returns the most recent spend attempts (newest first) for the UI.
func AuditLog(n int) []AuditEntry { return audit.recent(n) }

// recordSpend logs a spend attempt by an agent: into the in-memory ring (live UI
// feed) and, best-effort, into the persisted append-only trail.
func recordSpend(a *agent, tool string, account uint32, amountDCR float64, target, result, detail string) {
	e := AuditEntry{
		Time:      time.Now(),
		AgentID:   a.id,
		Agent:     a.name,
		Tool:      tool,
		Account:   account,
		AmountDCR: amountDCR,
		Target:    clampAuditField(target, auditTargetMax),
		Result:    result,
		Detail:    clampAuditField(detail, auditDetailMax),
	}
	audit.record(e)
	persistAudit(e)
	// Off this goroutine: recordSpend runs after the transaction is away and
	// before the caller gets its id back, so an agent that has stopped reading
	// its audit feed must not be able to hold up that reply. Coalesced rather
	// than spawned, or a burst of rows is a burst of parked goroutines.
	auditNotify.signal()
	notifySpend(e)
}
