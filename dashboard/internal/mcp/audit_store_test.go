// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// useTempAuditFile points the persisted trail at a file under t.TempDir() and
// restores the previous path afterwards.
func useTempAuditFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp_audit.jsonl")
	auditStoreMu.Lock()
	prev := auditPath
	auditPath = path
	auditStoreMu.Unlock()
	t.Cleanup(func() {
		auditStoreMu.Lock()
		auditPath = prev
		auditStoreMu.Unlock()
	})
	return path
}

// TestClampAuditField covers the bound applied to the caller-supplied audit
// fields: values within budget must survive byte-for-byte, and a longer one must
// be cut on a rune boundary and marked so it cannot pass for a complete value.
func TestClampAuditField(t *testing.T) {
	t.Run("values within budget are untouched", func(t *testing.T) {
		for _, s := range []string{"", "DsAddr123", strings.Repeat("a", auditTargetMax)} {
			if got := clampAuditField(s, auditTargetMax); got != s {
				t.Errorf("clampAuditField(%d chars) altered a value within budget", len(s))
			}
		}
	})

	t.Run("oversized values are cut and marked", func(t *testing.T) {
		got := clampAuditField(strings.Repeat("a", auditTargetMax*4), auditTargetMax)
		if len(got) > auditTargetMax {
			t.Fatalf("clamped length %d exceeds the %d budget", len(got), auditTargetMax)
		}
		if !strings.HasSuffix(got, auditTruncMarker) {
			t.Fatalf("clamped value is not marked as truncated: %q", got)
		}
	})

	t.Run("a cut lands on a rune boundary", func(t *testing.T) {
		// Three-byte runes guarantee the budget lands mid-rune.
		got := clampAuditField(strings.Repeat("世", auditTargetMax), auditTargetMax)
		if !utf8.ValidString(got) {
			t.Fatalf("clamped value is not valid UTF-8: %q", got)
		}
		if len(got) > auditTargetMax {
			t.Fatalf("clamped length %d exceeds the %d budget", len(got), auditTargetMax)
		}
	})
}

// TestRecordSpendBoundsFields is the wiring check: the clamp helper being correct
// is worthless if recordSpend does not apply it.
func TestRecordSpendBoundsFields(t *testing.T) {
	useTempAuditFile(t)
	huge := strings.Repeat("A", 2*1024*1024)
	recordSpend(testAgent("clamp-agent", "clamp", nil), "wallet_send", 0, 0, huge, "denied", huge)

	got := AuditLog(1)
	if len(got) != 1 {
		t.Fatalf("AuditLog returned %d entries, want 1", len(got))
	}
	if len(got[0].Target) > auditTargetMax {
		t.Errorf("Target kept %d bytes, want <= %d", len(got[0].Target), auditTargetMax)
	}
	if len(got[0].Detail) > auditDetailMax {
		t.Errorf("Detail kept %d bytes, want <= %d", len(got[0].Detail), auditDetailMax)
	}
}

// TestExportAuditRecoversPastUnreadableLine is the regression guard: an oversized
// line in the middle of the trail must not hide every entry recorded after it.
func TestExportAuditRecoversPastUnreadableLine(t *testing.T) {
	path := useTempAuditFile(t)

	first, _ := json.Marshal(AuditEntry{Tool: "first", Result: "ok"})
	second, _ := json.Marshal(AuditEntry{Tool: "second", Result: "ok"})
	// Must exceed the 1 MiB cap the old scanner-based reader used, not just the
	// current auditLineMax, or this passes against the very code it guards.
	poison := strings.Repeat("x", 2*1024*1024)
	body := string(first) + "\n" + poison + "\n" + string(second) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := exportAudit()
	if err != nil {
		t.Fatalf("exportAudit: %v", err)
	}
	var out []AuditEntry
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("export is not valid JSON: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("exported %d entries, want 2 (the entry after the bad line was dropped)", len(out))
	}
	if out[0].Tool != "first" || out[1].Tool != "second" {
		t.Fatalf("exported %q then %q, want first then second", out[0].Tool, out[1].Tool)
	}
}

// TestExportAuditRoundTrip checks the ordinary path still works after the
// recovery rewrite: entries come back in the order they were recorded.
func TestExportAuditRoundTrip(t *testing.T) {
	useTempAuditFile(t)
	persistAudit(AuditEntry{Tool: "one", Result: "ok"})
	persistAudit(AuditEntry{Tool: "two", Result: "denied"})

	data, err := exportAudit()
	if err != nil {
		t.Fatalf("exportAudit: %v", err)
	}
	var out []AuditEntry
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("export is not valid JSON: %v", err)
	}
	if len(out) != 2 || out[0].Tool != "one" || out[1].Tool != "two" {
		t.Fatalf("round trip returned %d entries in unexpected order", len(out))
	}
}
