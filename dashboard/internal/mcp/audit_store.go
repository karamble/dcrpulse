// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"dcrpulse/internal/config"
)

// The spend audit is mirrored to an append-only JSON-lines file so the trail
// survives restarts and can be exported. The in-memory ring (audit.go) still
// serves the live dashboard feed. Writes are best-effort: a failure never blocks
// or fails a spend. When the store is not initialized (e.g. in tests) auditPath
// is empty and the helpers no-op.
var (
	auditStoreMu sync.Mutex
	auditPath    string
)

// initAuditStore resolves the audit log path and ensures the file exists. Called
// once at startup.
func initAuditStore() {
	auditStoreMu.Lock()
	defer auditStoreMu.Unlock()
	auditPath = filepath.Join(config.AppDataDir, "mcp_audit.jsonl")
	if f, err := os.OpenFile(auditPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		_ = f.Close()
	}
}

// persistAudit appends one entry as a JSON line. Best-effort.
func persistAudit(e AuditEntry) {
	auditStoreMu.Lock()
	defer auditStoreMu.Unlock()
	if auditPath == "" {
		return
	}
	f, err := os.OpenFile(auditPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	if b, err := json.Marshal(e); err == nil {
		_, _ = f.Write(append(b, '\n'))
	}
}

// exportAudit reads the full persisted audit trail and returns it as a JSON
// array (newest entries last, in recorded order). Returns an empty array when
// nothing has been persisted.
func exportAudit() ([]byte, error) {
	auditStoreMu.Lock()
	path := auditPath
	auditStoreMu.Unlock()

	out := []AuditEntry{}
	if path != "" {
		if f, err := os.Open(path); err == nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for sc.Scan() {
				line := sc.Bytes()
				if len(line) == 0 {
					continue
				}
				var e AuditEntry
				if json.Unmarshal(line, &e) == nil {
					out = append(out, e)
				}
			}
		}
	}
	return json.MarshalIndent(out, "", "  ")
}
