package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/services"
)

func TestGamingLedgerBackupDownloadRestores(t *testing.T) {
	old := services.GamingStateDir
	services.GamingStateDir = t.TempDir()
	t.Cleanup(func() { services.GamingStateDir = old })

	rec := httptest.NewRecorder()
	BisonrelayGamingLedgerBackupHandler(rec, httptest.NewRequest(http.MethodGet, "/api/br/gaming/recovery/backup", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Content-Disposition") != "attachment" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", rec.Header())
	}
	if err := gamingfunds.RestoreBackup(filepath.Join(t.TempDir(), "restored"), rec.Body.Bytes()); err != nil {
		t.Fatalf("served backup does not restore: %v", err)
	}
}
