package gamingfunds

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
)

func TestAuthorityBackupRestoresCompleteLedger(t *testing.T) {
	source, scope, terms := testStore(t)
	deposit, err := source.Register(scope, terms, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	backup, err := source.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(t.TempDir(), "restored")
	if err = RestoreBackup(dir, backup); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	got, err := restored.Deposits(scope)
	if err != nil || len(got) != 1 || got[0].ID != deposit.ID || got[0].Terms.Recovery != terms.Recovery {
		t.Fatalf("restored deposit differs: %+v %v", got, err)
	}
	if key, err := restored.WalletKey(scope, terms.Table); err != nil || key.Public != terms.Recovery {
		t.Fatalf("restored wallet locator differs: %+v %v", key, err)
	}
	if _, err = os.Stat(filepath.Join(dir, "initialized")); err != nil {
		t.Fatal("restore did not preserve missing-ledger protection")
	}
}

func TestAuthorityBackupFailsClosed(t *testing.T) {
	source, _, _ := testStore(t)
	backup, err := source.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte(nil), backup...)
	if at := bytes.Index(tampered, []byte(`"stakeAtoms":1000000`)); at >= 0 {
		tampered[at+len(`"stakeAtoms":`)] = '9'
	} else {
		t.Fatal("backup fixture lacks table amount")
	}
	if err = RestoreBackup(filepath.Join(t.TempDir(), "tampered"), tampered); err == nil {
		t.Fatal("tampered backup restored")
	}
	if err = RestoreBackup(filepath.Join(t.TempDir(), "unknown"), append(append([]byte(nil), backup[:len(backup)-1]...), []byte(`,"unknown":true}`)...)); err == nil {
		t.Fatal("backup with unknown field restored")
	}

	existing, _, _ := testStore(t)
	dir := existing.dir
	if err = existing.Close(); err != nil {
		t.Fatal(err)
	}
	if err = RestoreBackup(dir, backup); err != ErrLedgerHasRecords {
		t.Fatalf("restore over a ledger with records: %v", err)
	}
}

func TestAuthorityBackupRestoresOverEmptyLedger(t *testing.T) {
	source, scope, terms := testStore(t)
	if _, err := source.Register(scope, terms, chaincfg.SimNetParams()); err != nil {
		t.Fatal(err)
	}
	backup, err := source.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	fresh, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = fresh.Close(); err != nil {
		t.Fatal(err)
	}
	if err = RestoreBackup(dir, backup); err != nil {
		t.Fatalf("restore over a fresh empty ledger: %v", err)
	}
	restored, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if got, err := restored.Deposits(scope); err != nil || len(got) != 1 {
		t.Fatalf("restored deposits = %+v %v", got, err)
	}
	scopes, err := BackupScopes(backup)
	if err != nil || len(scopes) != 1 || scopes[0] != scope {
		t.Fatalf("backup scopes = %+v %v", scopes, err)
	}
	keys, err := BackupKeys(backup)
	if err != nil || len(keys) != 1 || keys[0].Public != terms.Recovery {
		t.Fatalf("backup keys = %+v %v", keys, err)
	}
}
