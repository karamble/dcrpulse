package gamingcore

import (
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"

	"dcrpulse/internal/gamingfunds"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func restoreFixture(t *testing.T) ([]byte, gamingfunds.Scope) {
	t.Helper()
	src, err := gamingfunds.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	scope := gamingfunds.Scope{Game: "stakewars", Network: "mainnet", Wallet: "wallet-b", Account: 5}
	if err = src.AuthorizeTable(gamingfunds.TableAuthorization{Scope: scope, Table: "t7", StakeAtoms: 2500000, CSVBlocks: 16, AdmissionAtoms: 100000, AdmissionBlocks: 8, Seats: 2, Until: 100}); err != nil {
		t.Fatal(err)
	}
	priv := make([]byte, 32)
	priv[31] = 9
	pub := hex.EncodeToString(secp256k1.PrivKeyFromBytes(priv).PubKey().SerializeCompressed())
	if err = src.RegisterKey(gamingfunds.WalletKey{Scope: scope, Table: "t7", Address: "Dsrestored", Public: pub}); err != nil {
		t.Fatal(err)
	}
	raw, err := src.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	return raw, scope
}

func stubRestoreWallet(t *testing.T, br *Bridge, match, owned error) {
	t.Helper()
	br.restoreWalletMatches = func(context.Context, gamingfunds.Scope) error { return match }
	br.restoreKeyOwned = func(context.Context, gamingfunds.WalletKey) error { return owned }
	t.Cleanup(func() {
		br.financeStores.Lock()
		path := filepath.Join(br.dataDir, "financial-authority")
		if s := br.financeStores.stores[path]; s != nil {
			s.Close()
			delete(br.financeStores.stores, path)
		}
		br.financeStores.Unlock()
	})
}

func TestRestoreGamingLedgerReplacesOpenEmptyStore(t *testing.T) {
	br := newTestBridge(t)
	raw, scope := restoreFixture(t)
	stubRestoreWallet(t, br, nil, errors.New("not derived yet"))
	before, err := br.gamingFundsStore()
	if err != nil {
		t.Fatal(err)
	}
	unowned, err := br.RestoreGamingLedger(context.Background(), raw)
	if err != nil || unowned != 1 {
		t.Fatalf("restore = %d %v", unowned, err)
	}
	after, err := br.gamingFundsStore()
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Fatal("restore kept the store that held the empty ledger")
	}
	if got, err := after.AuthorizedTable(scope, "t7"); err != nil || got.StakeAtoms != 2500000 {
		t.Fatalf("restored table = %+v %v", got, err)
	}
}

func TestRestoreGamingLedgerRefusesAnotherWallet(t *testing.T) {
	br := newTestBridge(t)
	raw, scope := restoreFixture(t)
	stubRestoreWallet(t, br, errors.New("connect the original wallet"), nil)
	if _, err := br.RestoreGamingLedger(context.Background(), raw); !errors.Is(err, ErrGamingBackupWrongWallet) {
		t.Fatalf("restore from another wallet: %v", err)
	}
	store, err := br.gamingFundsStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AuthorizedTable(scope, "t7"); err == nil {
		t.Fatal("a refused backup was written")
	}
}

func TestRestoreGamingLedgerRefusesInvalidBackup(t *testing.T) {
	br := newTestBridge(t)
	stubRestoreWallet(t, br, nil, nil)
	if _, err := br.RestoreGamingLedger(context.Background(), []byte(`{"format":1}`)); !errors.Is(err, ErrGamingBackupInvalid) {
		t.Fatalf("invalid backup: %v", err)
	}
}
