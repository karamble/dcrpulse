package gamingfunds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
)

func depositNamed(t *testing.T, s *Store, id string) Deposit {
	t.Helper()
	all, err := s.AllDeposits()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range all {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("deposit %s missing", id)
	return Deposit{}
}

func TestOnlyASpentDepositIsArchived(t *testing.T) {
	s, scope, terms := testStore(t)
	dep, err := s.Register(scope, terms, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetDepositArchived(dep.ID, true); err == nil {
		t.Fatal("archived a deposit that still holds money")
	}
	if err := s.SetDepositArchived("nope", false); err == nil {
		t.Fatal("changed an unknown deposit")
	}
	if err := s.ObserveDeposit(scope, dep.ID, DepositObservation{SpendingTx: strings.Repeat("ab", 32), SpendConfirmations: 3}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDepositArchived(dep.ID, true); err != nil {
		t.Fatal(err)
	}
	if !depositNamed(t, s, dep.ID).Archived {
		t.Fatal("archive not recorded")
	}

	// The mark survives a restart and the record itself is untouched.
	dir := s.dir
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { again.Close() })
	got := depositNamed(t, again, dep.ID)
	if !got.Archived || got.State != "spent" || got.Script != dep.Script {
		t.Fatalf("after restart = %+v", got)
	}
	if err := again.SetDepositArchived(dep.ID, false); err != nil || depositNamed(t, again, dep.ID).Archived {
		t.Fatalf("restore = %v", err)
	}
}

func TestALedgerWithoutArchiveMarksLoads(t *testing.T) {
	s, scope, terms := testStore(t)
	dep, err := s.Register(scope, terms, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.dir, "authority.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"archived"`) {
		t.Fatal("an unarchived deposit wrote the field")
	}
	if got := depositNamed(t, s, dep.ID); got.Archived {
		t.Fatal("a fresh deposit is archived")
	}
}
