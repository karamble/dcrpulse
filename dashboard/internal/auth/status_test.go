// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"dcrpulse/internal/auth"
	"dcrpulse/internal/handlers"
)

// The status probe is the one route a locked gate lets through, so it has to
// carry the lock and its reason or the UI shows a login screen that cannot work.
func TestAuthStatusReportsLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auth.PointAtForTest(path))
	if err := auth.Init(); err == nil {
		t.Fatal("Init() accepted an unparseable config")
	}

	rec := httptest.NewRecorder()
	handlers.AuthStatusHandler(rec, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
	var got struct {
		Locked     bool   `json:"locked"`
		LockReason string `json:"lockReason"`
		Enabled    bool   `json:"enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("status body: %v", err)
	}
	if !got.Locked {
		t.Fatal("status does not report the lock")
	}
	if got.LockReason == "" {
		t.Fatal("status does not say why the gate is locked")
	}
	if got.Enabled {
		t.Fatal("status reports enabled while locked; the UI would show a login screen")
	}
}
