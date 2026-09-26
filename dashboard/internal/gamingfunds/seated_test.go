package gamingfunds

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func seatedStore(t *testing.T) (*Store, Scope, TableAuthorization) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	scope := Scope{Game: "stakewars", Network: "simnet", Wallet: "wallet-a"}
	auth := TableAuthorization{Scope: scope, Table: "a1", StakeAtoms: 1000000, CSVBlocks: 16, AdmissionAtoms: 100000, AdmissionBlocks: 8, Seats: 2, Until: 100}
	if err = s.AuthorizeTable(auth); err != nil {
		t.Fatal(err)
	}
	return s, scope, auth
}

func announce(s *Store, scope Scope, auth TableAuthorization, uid byte, key byte) error {
	id := fmt.Sprintf("%064x", uid)
	return s.RecordParticipant(scope, "a1", "", id, Participant{UID: id, Key: public(key), Destination: fmt.Sprintf("dest%d", key), TermsHash: auth.TermsHash()})
}

func peerUIDs(t *testing.T, s *Store, scope Scope) map[string]string {
	t.Helper()
	got, err := s.Participants(scope, "a1")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, p := range got {
		out[p.UID] = p.Key
	}
	return out
}

func TestOnlySeatedKeysJoinTheRoster(t *testing.T) {
	s, scope, auth := seatedStore(t)
	// A lurker announces first, then both seated players.
	for _, a := range [][2]byte{{9, 9}, {2, 2}, {3, 3}} {
		if err := announce(s, scope, auth, a[0], a[1]); err != nil {
			t.Fatal(err)
		}
	}
	if got := peerUIDs(t, s, scope); len(got) != 0 {
		t.Fatalf("roster filled before seats were bound: %v", got)
	}
	if h, _ := s.RosterHash(scope, "a1"); h != "" {
		t.Fatal("roster hash before seats were bound")
	}
	if err := s.BindSeats(scope, "a1", []string{public(3), public(2)}); err != nil {
		t.Fatal(err)
	}
	got := peerUIDs(t, s, scope)
	if len(got) != 2 || got[fmt.Sprintf("%064x", 2)] != public(2) || got[fmt.Sprintf("%064x", 3)] != public(3) {
		t.Fatalf("roster = %v", got)
	}
	if h, _ := s.RosterHash(scope, "a1"); h == "" {
		t.Fatal("no roster hash once seated players are in")
	}
	if err := announce(s, scope, auth, 8, 8); err == nil {
		t.Fatal("an unseated key was admitted after binding")
	}
}

func TestALateSeatedAnnouncementIsAdmitted(t *testing.T) {
	s, scope, auth := seatedStore(t)
	if err := announce(s, scope, auth, 2, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.BindSeats(scope, "a1", []string{public(2), public(3)}); err != nil {
		t.Fatal(err)
	}
	if got := peerUIDs(t, s, scope); len(got) != 1 {
		t.Fatalf("roster = %v", got)
	}
	if err := announce(s, scope, auth, 8, 8); err == nil {
		t.Fatal("an unseated key took the free slot")
	}
	if err := announce(s, scope, auth, 3, 3); err != nil {
		t.Fatal(err)
	}
	if got := peerUIDs(t, s, scope); len(got) != 2 {
		t.Fatalf("roster = %v", got)
	}
}

func TestTheEarlierAnnouncerOfAKeyKeepsIt(t *testing.T) {
	s, scope, auth := seatedStore(t)
	for _, a := range [][2]byte{{2, 2}, {7, 2}, {3, 3}} {
		if err := announce(s, scope, auth, a[0], a[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.BindSeats(scope, "a1", []string{public(2), public(3)}); err != nil {
		t.Fatal(err)
	}
	got := peerUIDs(t, s, scope)
	if _, copied := got[fmt.Sprintf("%064x", 7)]; copied || len(got) != 2 {
		t.Fatalf("roster = %v", got)
	}
}

func TestSeatsBindOnce(t *testing.T) {
	s, scope, _ := seatedStore(t)
	if err := s.BindSeats(scope, "a1", []string{public(2)}); err == nil {
		t.Fatal("bound a roster that does not fill the table")
	}
	if err := s.BindSeats(scope, "a1", []string{public(2), public(2)}); err == nil {
		t.Fatal("bound the same key twice")
	}
	if err := s.BindSeats(scope, "a1", []string{public(2), public(3)}); err != nil {
		t.Fatal(err)
	}
	if err := s.BindSeats(scope, "a1", []string{public(3), public(2)}); err != nil {
		t.Fatalf("the same roster again: %v", err)
	}
	if err := s.BindSeats(scope, "a1", []string{public(2), public(4)}); err == nil {
		t.Fatal("a different roster replaced the bound one")
	}
}

func TestALedgerWithoutSeatsStillLoads(t *testing.T) {
	s, scope, _ := seatedStore(t)
	path := filepath.Join(s.dir, "authority.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	delete(generic, "seated")
	delete(generic, "candidates")
	raw, _ = json.Marshal(generic)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.BindSeats(scope, "a1", []string{public(2), public(3)}); err != nil {
		t.Fatalf("old ledger: %v", err)
	}
}

func TestAKeyProofIsWrittenOnce(t *testing.T) {
	s, scope, _ := seatedStore(t)
	if got, err := s.KeyProof(scope, "a1"); err != nil || got != "" {
		t.Fatalf("proof before any = %q, %v", got, err)
	}
	if err := s.SaveKeyProof(scope, "a1", "zz"); err == nil {
		t.Fatal("stored a proof that is not hex")
	}
	if err := s.SaveKeyProof(scope, "a2", "0102"); err == nil {
		t.Fatal("stored a proof for an unknown table")
	}
	if err := s.SaveKeyProof(scope, "a1", "0102"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveKeyProof(scope, "a1", "0102"); err != nil {
		t.Fatalf("the same proof again: %v", err)
	}
	if err := s.SaveKeyProof(scope, "a1", "0304"); err == nil {
		t.Fatal("a second proof replaced the first")
	}
	if got, _ := s.KeyProof(scope, "a1"); got != "0102" {
		t.Fatalf("proof = %q", got)
	}
}
