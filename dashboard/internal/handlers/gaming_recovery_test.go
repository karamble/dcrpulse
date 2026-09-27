package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karamble/dcrgaming-sdk/pkg/gaming/bridge/funds"
)

func TestGamingLedgerBackupDownloadRestores(t *testing.T) {
	useTestGamingBridge(t, t.TempDir())

	rec := httptest.NewRecorder()
	BisonrelayGamingLedgerBackupHandler(rec, httptest.NewRequest(http.MethodGet, "/api/br/gaming/recovery/backup", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Content-Disposition") != "attachment" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", rec.Header())
	}
	if err := funds.RestoreBackup(filepath.Join(t.TempDir(), "restored"), rec.Body.Bytes()); err != nil {
		t.Fatalf("served backup does not restore: %v", err)
	}
}

func TestGamingLedgerRestoreRefusesLedgerWithRecords(t *testing.T) {
	useTestGamingBridge(t, t.TempDir())

	rec := httptest.NewRecorder()
	BisonrelayGamingLedgerBackupHandler(rec, httptest.NewRequest(http.MethodGet, "/api/br/gaming/recovery/backup", nil))
	backup := rec.Body.Bytes()

	rec = httptest.NewRecorder()
	BisonrelayGamingLedgerRestoreHandler(rec, httptest.NewRequest(http.MethodPost, "/api/br/gaming/recovery/restore", bytes.NewReader(backup)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"restored":true`) {
		t.Fatalf("restore onto empty ledger: %d %s", rec.Code, rec.Body)
	}

	populated := filepath.Join(t.TempDir(), "financial-authority")
	store, err := funds.Open(populated)
	if err != nil {
		t.Fatal(err)
	}
	scope := funds.Scope{Game: "orbitgolf", Network: "mainnet", Wallet: "wallet-c", Account: 3}
	if err = store.AuthorizeTable(funds.TableAuthorization{Scope: scope, Table: "g4", StakeAtoms: 700000, CSVBlocks: 16, AdmissionAtoms: 100000, AdmissionBlocks: 8, Seats: 2, Until: 100}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	useTestGamingBridge(t, filepath.Dir(populated))
	rec = httptest.NewRecorder()
	BisonrelayGamingLedgerRestoreHandler(rec, httptest.NewRequest(http.MethodPost, "/api/br/gaming/recovery/restore", bytes.NewReader(backup)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("restore over records: %d %s", rec.Code, rec.Body)
	}
}
