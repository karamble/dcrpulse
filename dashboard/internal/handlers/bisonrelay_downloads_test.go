// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestDownloadsListStaysInsideTheDownloadsRoot pins the handler's own
// containment check: the contact segment names a directory under the downloads
// root and nothing else. The nick regex admits "." and "..", and the router's
// default path cleaning is the only other thing standing in the way, so the
// vars are injected directly to prove the handler holds by itself.
func TestDownloadsListStaysInsideTheDownloadsRoot(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("BRCLIENTD_DATA_DIR", dataDir)

	downloads := filepath.Join(dataDir, "data", "mainnet", "downloads")
	if err := os.MkdirAll(filepath.Join(downloads, "alice"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(downloads, "alice", "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The marker one level above the root: an escaped listing would name it.
	if err := os.WriteFile(filepath.Join(dataDir, "data", "mainnet", "secret.json"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	get := func(contact string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/br/downloads/contact", nil)
		req = mux.SetURLVars(req, map[string]string{"contact": contact})
		rec := httptest.NewRecorder()
		BisonrelayDownloadsListHandler(rec, req)
		return rec
	}

	if rec := get("alice"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "a.txt") {
		t.Fatalf("alice answered %d %q, want 200 listing a.txt", rec.Code, rec.Body.String())
	}
	for _, contact := range []string{"..", "."} {
		rec := get(contact)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("contact %q answered %d, want 404", contact, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "secret.json") {
			// The mutation catch: without the containment check ".." lists the
			// directory above the downloads root.
			t.Fatalf("contact %q leaked a name from above the root: %q", contact, rec.Body.String())
		}
	}
}
