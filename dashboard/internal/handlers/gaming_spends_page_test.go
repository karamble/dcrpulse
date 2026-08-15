// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dcrpulse/internal/services"
)

// seedSpendFile writes a spend log the way the service persists one, at the
// path the redirected state directory makes it read from.
func seedSpendFile(t *testing.T, spends []services.GamingSpend) {
	t.Helper()
	dir := t.TempDir()
	orig := services.GamingStateDir
	services.GamingStateDir = dir
	t.Cleanup(func() { services.GamingStateDir = orig })
	blob, err := json.Marshal(map[string]any{"spends": spends})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gaming-spends.json"), blob, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// Everything still in flight always arrives whole - it is what the approvals
// panel and the tab's count exist for - while decided history comes a page at
// a time, with the total beside it and the server's own day figure per game,
// so the console never re-derives the number the cap is enforced against.
func TestSpendHistoryComesAPageAtATime(t *testing.T) {
	now := time.Now().Unix()
	var spends []services.GamingSpend
	spends = append(spends,
		services.GamingSpend{ID: "p1", Game: "poker", State: services.GamingSpendPending,
			AmountAtoms: 100, RequestedAt: now - 1, ExpiresAt: now + 300},
		services.GamingSpend{ID: "p2", Game: "poker", State: services.GamingSpendPending,
			AmountAtoms: 200, RequestedAt: now - 2, ExpiresAt: now + 300},
		services.GamingSpend{ID: "b1", Game: "poker", State: services.GamingSpendPublishing,
			AmountAtoms: 400, RequestedAt: now - 3, ExpiresAt: now + 300},
	)
	for i := 0; i < 25; i++ {
		spends = append(spends, services.GamingSpend{
			ID: string(rune('a'+i)) + "-done", Game: "poker", State: services.GamingSpendDenied,
			AmountAtoms: 1, RequestedAt: now - int64(100+i), DecidedAt: now - int64(50+i),
		})
	}
	seedSpendFile(t, spends)

	req := httptest.NewRequest("GET", "/br/gaming/spends?page=2&pageSize=10", nil)
	rec := httptest.NewRecorder()
	BisonrelayGamingSpendsHandler(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Pending      []services.GamingSpend `json:"pending"`
		Decided      []services.GamingSpend `json:"decided"`
		DecidedTotal int                    `json:"decidedTotal"`
		Page         int                    `json:"page"`
		UsedToday    map[string]int64       `json:"usedToday"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got.Pending) != 3 {
		t.Fatalf("pending came %d entries, want all 3 including the one being paid", len(got.Pending))
	}
	if len(got.Decided) != 10 || got.DecidedTotal != 25 || got.Page != 2 {
		t.Fatalf("page 2 of 10 came %d rows of %d total, page %d", len(got.Decided), got.DecidedTotal, got.Page)
	}
	for _, s := range got.Decided {
		if s.State == services.GamingSpendPending || s.State == services.GamingSpendPublishing {
			t.Fatalf("history holds %q, which is still in flight", s.State)
		}
	}
	// 100 + 200 waiting, 400 being paid; the 25 denied never count.
	if got.UsedToday["poker"] != 700 {
		t.Fatalf("the day figure came %d, want 700", got.UsedToday["poker"])
	}

	// A page size beyond the clamp is cut to it, not served.
	req = httptest.NewRequest("GET", "/br/gaming/spends?page=1&pageSize=1000", nil)
	rec = httptest.NewRecorder()
	BisonrelayGamingSpendsHandler(rec, req)
	var clamped struct {
		PageSize int `json:"pageSize"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &clamped); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if clamped.PageSize != 100 {
		t.Fatalf("a thousand-row page was served as %d, want the clamp of 100", clamped.PageSize)
	}
}
