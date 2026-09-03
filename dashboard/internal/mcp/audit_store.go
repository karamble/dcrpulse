// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"dcrpulse/internal/config"
)

// auditLineMax bounds one persisted JSON line. Entries are already clamped when
// they are recorded, so this only guards a future unbounded field: a line longer
// than this cannot be read back, so it is dropped rather than written.
const auditLineMax = 64 * 1024

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
		if len(b) > auditLineMax {
			mcpLog.Warnf("Dropping an oversized audit entry for %s (%d bytes)",
				sanitizeLogField(e.Tool), len(b))
			return
		}
		_, _ = f.Write(append(b, '\n'))
	}
}

// exportAudit reads the full persisted audit trail and returns it as a JSON
// array (newest entries last, in recorded order). Returns an empty array when
// nothing has been persisted. A line that cannot be read is skipped and counted
// rather than ending the read, so one bad entry never hides the ones after it.
func exportAudit() ([]byte, error) {
	auditStoreMu.Lock()
	path := auditPath
	auditStoreMu.Unlock()

	out := []AuditEntry{}
	if path != "" {
		if f, err := os.Open(path); err == nil {
			defer f.Close()
			skipped := 0
			r := bufio.NewReader(f)
			for {
				line, tooLong, err := readAuditLine(r)
				switch {
				case tooLong:
					skipped++
				case len(line) == 0:
					// blank line: nothing to decode
				default:
					var e AuditEntry
					if json.Unmarshal(line, &e) == nil {
						out = append(out, e)
					} else {
						skipped++
					}
				}
				if err != nil {
					if !errors.Is(err, io.EOF) {
						mcpLog.Warnf("Audit export stopped early: %v", err)
					}
					break
				}
			}
			if skipped > 0 {
				mcpLog.Warnf("Audit export skipped %d unreadable line(s); the exported trail is incomplete", skipped)
			}
		}
	}
	return json.MarshalIndent(out, "", "  ")
}

// readAuditLine reads one line, reporting tooLong when it exceeds auditLineMax.
// An over-long line is consumed and discarded without being buffered, so the
// reader stays aligned on the next entry instead of stopping at the bad one.
func readAuditLine(r *bufio.Reader) (line []byte, tooLong bool, err error) {
	for {
		chunk, more, err := r.ReadLine()
		if err != nil {
			return line, tooLong, err
		}
		if !tooLong {
			if len(line)+len(chunk) > auditLineMax {
				tooLong, line = true, nil
			} else {
				line = append(line, chunk...)
			}
		}
		if !more {
			return line, tooLong, nil
		}
	}
}
