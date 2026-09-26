package services

import (
	"encoding/hex"
	"fmt"
	"testing"

	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/gamingpb"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/karamble/dcrgaming-sdk/pkg/escrow"
	"github.com/karamble/dcrgaming-sdk/pkg/membership"
)

var rosterTestTerms = membership.Terms{
	Game: "stakewars", GameVer: 5, SID: "abcdef01", BuyInAtoms: 10_000_000, Seats: 2,
	CSVBlocks: 288, Until: 900, BondAtoms: 1_000_000, BondLockBlocks: 2016,
}

func testPriv(n byte) *secp256k1.PrivateKey {
	b := make([]byte, 32)
	b[31] = n
	return secp256k1.PrivKeyFromBytes(b)
}

// signedRoster builds a real two-seat roster with the SDK's own signing, and
// returns each seat's bridge (recovery) key.
func signedRoster(t *testing.T) ([]*gamingpb.SignedJoin, []*gamingpb.SignedCommit, []string) {
	t.Helper()
	var joins []*membership.Join
	var sessions []*secp256k1.PrivateKey
	var recoveries []string
	for seat := byte(0); seat < 2; seat++ {
		session, logKey, bond, bridge := testPriv(10+seat), testPriv(20+seat), testPriv(30+seat), testPriv(40+seat)
		script, err := escrow.BridgeBondScript(bond.PubKey().SerializeCompressed(), bridge.PubKey().SerializeCompressed(), rosterTestTerms.BondLockBlocks)
		if err != nil {
			t.Fatal(err)
		}
		j, err := membership.SignJoin(rosterTestTerms, membership.Credentials{
			Session: session, Log: logKey, Bond: bond,
			BondOutpoint: fmt.Sprintf("%064x:0", 50+int(seat)), BondScript: script,
		})
		if err != nil {
			t.Fatal(err)
		}
		joins = append(joins, j)
		sessions = append(sessions, session)
		recoveries = append(recoveries, hex.EncodeToString(bridge.PubKey().SerializeCompressed()))
	}
	keys := [][]byte{joins[0].Key, joins[1].Key}
	roster, err := membership.RosterHash(rosterTestTerms, keys)
	if err != nil {
		t.Fatal(err)
	}
	var wireJoins []*gamingpb.SignedJoin
	var wireCommits []*gamingpb.SignedCommit
	for i, j := range joins {
		wireJoins = append(wireJoins, &gamingpb.SignedJoin{Key: j.Key, LogKey: j.LogKey, Sig: j.Sig,
			BondOutpoint: j.Bond.Outpoint, BondScript: j.Bond.Script, BondPop: j.Bond.PoP})
		c, err := membership.SignCommit(rosterTestTerms, roster, sessions[i])
		if err != nil {
			t.Fatal(err)
		}
		wireCommits = append(wireCommits, &gamingpb.SignedCommit{Roster: c.Roster[:], Signer: c.Signer, Sig: c.Sig})
	}
	return wireJoins, wireCommits, recoveries
}

func TestSeatKeysComeFromAVerifiedRoster(t *testing.T) {
	joins, commits, want := signedRoster(t)
	keys, err := verifiedSeatKeys(rosterTestTerms, joins, commits)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != want[0] || keys[1] != want[1] {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
}

func TestForgedOrIncompleteRostersAreRefused(t *testing.T) {
	cases := map[string]func(j []*gamingpb.SignedJoin, c []*gamingpb.SignedCommit) ([]*gamingpb.SignedJoin, []*gamingpb.SignedCommit){
		"join signature": func(j []*gamingpb.SignedJoin, c []*gamingpb.SignedCommit) ([]*gamingpb.SignedJoin, []*gamingpb.SignedCommit) {
			j[0].Sig = append([]byte(nil), j[1].Sig...)
			return j, c
		},
		"bond swapped": func(j []*gamingpb.SignedJoin, c []*gamingpb.SignedCommit) ([]*gamingpb.SignedJoin, []*gamingpb.SignedCommit) {
			j[0].BondScript = j[1].BondScript
			return j, c
		},
		"one join": func(j []*gamingpb.SignedJoin, c []*gamingpb.SignedCommit) ([]*gamingpb.SignedJoin, []*gamingpb.SignedCommit) {
			return j[:1], c
		},
		"same join twice": func(j []*gamingpb.SignedJoin, c []*gamingpb.SignedCommit) ([]*gamingpb.SignedJoin, []*gamingpb.SignedCommit) {
			return []*gamingpb.SignedJoin{j[0], j[0]}, c
		},
		"missing commit": func(j []*gamingpb.SignedJoin, c []*gamingpb.SignedCommit) ([]*gamingpb.SignedJoin, []*gamingpb.SignedCommit) {
			return j, c[:1]
		},
		"commit to another roster": func(j []*gamingpb.SignedJoin, c []*gamingpb.SignedCommit) ([]*gamingpb.SignedJoin, []*gamingpb.SignedCommit) {
			c[1].Roster = make([]byte, 32)
			return j, c
		},
		"commit signature": func(j []*gamingpb.SignedJoin, c []*gamingpb.SignedCommit) ([]*gamingpb.SignedJoin, []*gamingpb.SignedCommit) {
			c[1].Sig = append([]byte(nil), c[0].Sig...)
			return j, c
		},
	}
	for name, mutate := range cases {
		joins, commits, _ := signedRoster(t)
		j, c := mutate(joins, commits)
		if _, err := verifiedSeatKeys(rosterTestTerms, j, c); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRosterTermsMustBeTheAcceptedTable(t *testing.T) {
	accepted := gamingfunds.TableAuthorization{
		Scope: gamingfunds.Scope{Game: "stakewars"}, Table: "abcdef01", StakeAtoms: 10_000_000, Seats: 2,
		CSVBlocks: 288, Until: 900, AdmissionAtoms: 1_000_000, AdmissionBlocks: 2016,
	}
	if !rosterTermsMatch(rosterTestTerms, accepted) {
		t.Fatal("matching terms refused")
	}
	for name, edit := range map[string]func(*membership.Terms){
		"game":    func(m *membership.Terms) { m.Game = "chess" },
		"sid":     func(m *membership.Terms) { m.SID = "abcdef02" },
		"buyin":   func(m *membership.Terms) { m.BuyInAtoms++ },
		"seats":   func(m *membership.Terms) { m.Seats = 3 },
		"csv":     func(m *membership.Terms) { m.CSVBlocks++ },
		"until":   func(m *membership.Terms) { m.Until++ },
		"bond":    func(m *membership.Terms) { m.BondAtoms++ },
		"bondcsv": func(m *membership.Terms) { m.BondLockBlocks++ },
	} {
		terms := rosterTestTerms
		edit(&terms)
		if rosterTermsMatch(terms, accepted) {
			t.Errorf("%s: different terms accepted", name)
		}
	}
}

// isolatedRoster signs n joins from the first seats and commits by every
// joined session over roster, so exactly one property is wrong at a time.
func isolatedRoster(t *testing.T, sessions []byte, roster func([][]byte) [32]byte) ([]*gamingpb.SignedJoin, []*gamingpb.SignedCommit) {
	t.Helper()
	var joins []*gamingpb.SignedJoin
	var keys [][]byte
	for _, seat := range sessions {
		session, logKey, bond, bridge := testPriv(10+seat), testPriv(20+seat), testPriv(30+seat), testPriv(40+seat)
		script, err := escrow.BridgeBondScript(bond.PubKey().SerializeCompressed(), bridge.PubKey().SerializeCompressed(), rosterTestTerms.BondLockBlocks)
		if err != nil {
			t.Fatal(err)
		}
		j, err := membership.SignJoin(rosterTestTerms, membership.Credentials{Session: session, Log: logKey, Bond: bond,
			BondOutpoint: fmt.Sprintf("%064x:0", 50+int(seat)), BondScript: script})
		if err != nil {
			t.Fatal(err)
		}
		joins = append(joins, &gamingpb.SignedJoin{Key: j.Key, LogKey: j.LogKey, Sig: j.Sig,
			BondOutpoint: j.Bond.Outpoint, BondScript: j.Bond.Script, BondPop: j.Bond.PoP})
		keys = append(keys, j.Key)
	}
	hash := roster(keys)
	var commits []*gamingpb.SignedCommit
	for _, seat := range sessions {
		c, err := membership.SignCommit(rosterTestTerms, hash, testPriv(10+seat))
		if err != nil {
			t.Fatal(err)
		}
		commits = append(commits, &gamingpb.SignedCommit{Roster: c.Roster[:], Signer: c.Signer, Sig: c.Sig})
	}
	return joins, commits
}

func TestEachRosterCheckStandsAlone(t *testing.T) {
	honest := func(keys [][]byte) [32]byte {
		h, err := membership.RosterHash(rosterTestTerms, keys)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if j, c := isolatedRoster(t, []byte{0}, honest); true {
		if _, err := verifiedSeatKeys(rosterTestTerms, j, c); err == nil {
			t.Error("a signed one-seat roster filled a two-seat table")
		}
	}
	if j, c := isolatedRoster(t, []byte{0, 0}, func(keys [][]byte) [32]byte {
		h, _ := membership.RosterHash(rosterTestTerms, keys)
		return h
	}); true {
		if _, err := verifiedSeatKeys(rosterTestTerms, j, c); err == nil {
			t.Error("one player took both seats")
		}
	}
	if j, c := isolatedRoster(t, []byte{0, 1}, func([][]byte) [32]byte { return [32]byte{1} }); true {
		if _, err := verifiedSeatKeys(rosterTestTerms, j, c); err == nil {
			t.Error("commits to another roster were accepted")
		}
	}
}
