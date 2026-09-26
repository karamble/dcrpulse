package gamingfunds

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func public(n byte) string {
	b := make([]byte, 32)
	b[31] = n
	return hex.EncodeToString(secp256k1.PrivKeyFromBytes(b).PubKey().SerializeCompressed())
}
func testStore(t *testing.T) (*Store, Scope, Terms) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	scope := Scope{Game: "stakewars", Network: "simnet", Wallet: "wallet-a", Account: 2}
	auth := TableAuthorization{Scope: scope, Table: "a1", StakeAtoms: 1000000, CSVBlocks: 16, AdmissionAtoms: 100000, AdmissionBlocks: 8, Seats: 2, Until: 100}
	if err = s.AuthorizeTable(auth); err != nil {
		t.Fatal(err)
	}
	if err = s.RegisterKey(WalletKey{Scope: scope, Table: "a1", Address: "wallet-address", Public: public(2)}); err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= 3; i++ {
		uid := fmt.Sprintf("%064x", i)
		if err = s.RecordParticipant(scope, "a1", "", uid, Participant{UID: uid, Key: public(byte(i)), Destination: fmt.Sprintf("destination%d", i), TermsHash: auth.TermsHash()}); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.BindSeats(scope, "a1", []string{public(2), public(3)}); err != nil {
		t.Fatal(err)
	}
	hash, err := s.RosterHash(scope, "a1")
	if err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= 3; i++ {
		if _, err = s.CommitRoster(scope, "a1", fmt.Sprintf("%064x", i), hash); err != nil {
			t.Fatal(err)
		}
	}
	terms := Terms{Version: Version, Game: scope.Game, Network: scope.Network, Account: scope.Account, Table: "a1", Kind: "stake", Atoms: auth.StakeAtoms, LockBlocks: auth.CSVBlocks, Identity: public(1), Recovery: public(2), Members: []string{public(3), public(2)}}
	return s, scope, terms
}
func TestWalletLocatorAndDescriptorSurviveRestart(t *testing.T) {
	s, scope, terms := testStore(t)
	dep, err := s.Register(scope, terms, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.VerifyRecovery(scope, dep.ID, chaincfg.SimNetParams()); err != nil {
		t.Fatal(err)
	}
	terms.Members[0], terms.Members[1] = strings.ToUpper(terms.Members[1]), strings.ToUpper(terms.Members[0])
	same, err := s.Register(scope, terms, chaincfg.SimNetParams())
	if err != nil || same.ID != dep.ID {
		t.Fatalf("duplicate descriptor: %v", err)
	}
	dir := s.dir
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	key, err := restored.WalletKey(scope, "a1")
	if err != nil || key.Public != terms.Recovery {
		t.Fatalf("lost locator: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "authority.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") {
		t.Fatal("private key store retained")
	}
	terms.Atoms++
	if _, err = restored.Register(scope, terms, chaincfg.SimNetParams()); err == nil {
		t.Fatal("changed terms accepted")
	}
	scope.Wallet = "foreign"
	if _, err = restored.PublicKey(scope, "a1"); err == nil {
		t.Fatal("foreign key exposed")
	}
}
func TestMissingAndCorruptLedgerFailClosed(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "corrupt", true: "missing"}[missing], func(t *testing.T) {
			s, _, _ := testStore(t)
			dir := s.dir
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			var err error
			if missing {
				err = os.Remove(filepath.Join(dir, "authority.json"))
			} else {
				err = os.WriteFile(filepath.Join(dir, "authority.json"), []byte("{}"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if other, err := Open(dir); err == nil {
				other.Close()
				t.Fatal("lost ledger silently recreated")
			}
		})
	}
}

func TestUnknownLedgerFieldsFailClosed(t *testing.T) {
	s, _, _ := testStore(t)
	dir := s.dir
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "authority.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw[:len(raw)-1], []byte(`,"unexpectedAuthority":true}`)...)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if other, err := Open(dir); err == nil {
		other.Close()
		t.Fatal("unknown authority field was ignored")
	}
}
func TestAcceptedTermsAndAuthorityRequired(t *testing.T) {
	s, scope, terms := testStore(t)
	for _, mutate := range []func(*Terms){func(d *Terms) { d.Atoms++ }, func(d *Terms) { d.LockBlocks++ }, func(d *Terms) { d.Recovery = public(3) }, func(d *Terms) { d.Kind = "forfeitbond" }, func(d *Terms) { d.Table = "not-accepted" }} {
		d := terms
		mutate(&d)
		if _, err := s.Register(scope, d, chaincfg.SimNetParams()); err == nil {
			t.Fatal("unapproved obligation accepted")
		}
	}
	if err := s.CloseTable(scope, "a1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register(scope, terms, chaincfg.SimNetParams()); err == nil {
		t.Fatal("closed table funded")
	}
}
