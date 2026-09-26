package services

import (
	"context"
	"encoding/hex"
	"fmt"

	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/gamingpb"

	"github.com/karamble/dcrgaming-sdk/pkg/escrow"
	"github.com/karamble/dcrgaming-sdk/pkg/membership"
)

// BindGamingRoster takes a seated table's roster from its game, verifies every
// join and commit itself, and binds the table's financial roster to the seats'
// bridge keys: the recovery key each join's bond names.
func BindGamingRoster(ctx context.Context, game string, req *gamingpb.BindRosterRequest) error {
	rt := req.GetTerms()
	if rt == nil {
		return fmt.Errorf("roster without terms")
	}
	terms := membership.Terms{
		Game: rt.GetGame(), GameVer: int(rt.GetGameVersion()), SID: rt.GetSid(),
		BuyInAtoms: rt.GetBuyinAtoms(), Seats: rt.GetSeats(), CSVBlocks: rt.GetCsvBlocks(),
		Until: rt.GetUntil(), BondAtoms: rt.GetBondAtoms(), BondLockBlocks: rt.GetBondLockBlocks(),
		AccuseFeeAtoms: rt.GetAccuseFeeAtoms(), ForfeitBondAtoms: rt.GetForfeitBondAtoms(),
	}
	if terms.Game != game {
		return fmt.Errorf("roster is for another game")
	}
	scope, err := gamingFinancialScope(ctx, game)
	if err != nil {
		return err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return err
	}
	accepted, err := store.AuthorizedTable(scope, terms.SID)
	if err != nil {
		return err
	}
	if !rosterTermsMatch(terms, accepted) {
		return fmt.Errorf("roster terms differ from the accepted table")
	}
	keys, err := verifiedSeatKeys(terms, req.GetJoins(), req.GetCommits())
	if err != nil {
		return err
	}
	own, err := store.WalletKey(scope, terms.SID)
	if err != nil {
		return err
	}
	if !containsString(keys, own.Public) {
		return fmt.Errorf("this bridge's key holds no seat at the table")
	}
	if err := store.BindSeats(scope, terms.SID, keys); err != nil {
		return err
	}
	return announceGamingAuthority(ctx, scope, terms.SID)
}

// rosterTermsMatch reports whether a roster's terms are the table the operator
// accepted.
func rosterTermsMatch(terms membership.Terms, accepted gamingfunds.TableAuthorization) bool {
	return terms.SID == accepted.Table && terms.Game == accepted.Scope.Game &&
		terms.BuyInAtoms == uint64(accepted.StakeAtoms) && terms.Seats == accepted.Seats &&
		terms.CSVBlocks == accepted.CSVBlocks && terms.Until == accepted.Until &&
		terms.BondAtoms == uint64(accepted.AdmissionAtoms) && terms.BondLockBlocks == accepted.AdmissionBlocks
}

// verifiedSeatKeys checks a full, unanimously committed roster and returns each
// seat's bridge financial key.
func verifiedSeatKeys(terms membership.Terms, joins []*gamingpb.SignedJoin, commits []*gamingpb.SignedCommit) ([]string, error) {
	if len(joins) != int(terms.Seats) {
		return nil, fmt.Errorf("roster has %d joins for %d seats", len(joins), terms.Seats)
	}
	sessions := make([][]byte, 0, len(joins))
	seen := map[string]bool{}
	var keys []string
	for _, j := range joins {
		join := &membership.Join{Key: j.GetKey(), LogKey: j.GetLogKey(), Sig: j.GetSig(),
			Bond: membership.Bond{Outpoint: j.GetBondOutpoint(), Script: j.GetBondScript(), PoP: j.GetBondPop()}}
		if err := join.Verify(terms); err != nil {
			return nil, fmt.Errorf("join: %w", err)
		}
		id := hex.EncodeToString(join.Key)
		if seen[id] {
			return nil, fmt.Errorf("a seat joined twice")
		}
		seen[id] = true
		sessions = append(sessions, join.Key)
		bond, err := escrow.ParseBond(join.Bond.Script)
		if err != nil {
			return nil, fmt.Errorf("join bond: %w", err)
		}
		keys = append(keys, hex.EncodeToString(bond.Recovery))
	}
	roster, err := membership.RosterHash(terms, sessions)
	if err != nil {
		return nil, err
	}
	committed := map[string]bool{}
	for _, c := range commits {
		commit := &membership.Commit{Signer: c.GetSigner(), Sig: c.GetSig()}
		if len(c.GetRoster()) != len(commit.Roster) {
			return nil, fmt.Errorf("commit roster is not a hash")
		}
		copy(commit.Roster[:], c.GetRoster())
		if commit.Roster != roster {
			return nil, fmt.Errorf("a commit names another roster")
		}
		if err := commit.Verify(terms); err != nil {
			return nil, fmt.Errorf("commit: %w", err)
		}
		committed[hex.EncodeToString(commit.Signer)] = true
	}
	for id := range seen {
		if !committed[id] {
			return nil, fmt.Errorf("a seat has not committed to the roster")
		}
	}
	return keys, nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
