// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"sync"
	"time"
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
		Target:    target,
		Result:    result,
		Detail:    detail,
	}
	audit.record(e)
	persistAudit(e)
	notifyResourceUpdated(resAudit)
	notifySpend(e)
}
