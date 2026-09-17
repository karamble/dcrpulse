package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/gamingpb"
)

var financeStores struct {
	sync.Mutex
	stores map[string]*gamingfunds.Store
}
var financialTableID = regexp.MustCompile(`^[0-9a-f]{1,32}$`)

func gamingFundsStore() (*gamingfunds.Store, error) {
	financeStores.Lock()
	defer financeStores.Unlock()
	path := filepath.Join(GamingStateDir, "financial-authority")
	if financeStores.stores == nil {
		financeStores.stores = map[string]*gamingfunds.Store{}
	}
	if s := financeStores.stores[path]; s != nil {
		return s, nil
	}
	s, err := gamingfunds.Open(path)
	if err != nil {
		return nil, err
	}
	financeStores.stores[path] = s
	return s, nil
}
func gamingFinancialScope(ctx context.Context, game string) (gamingfunds.Scope, error) {
	var scope gamingfunds.Scope
	if !gamingGameRegistered(game) {
		return scope, ErrGamingGameNotRegistered
	}
	account, err := gamingAccountNumber(ctx, game)
	if err != nil {
		return scope, err
	}
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return scope, err
	}
	xpub, err := GetAccountExtendedPubKey(ctx, account)
	if err != nil {
		return scope, err
	}
	if xpub == "" {
		return scope, fmt.Errorf("wallet fingerprint unavailable")
	}
	fingerprint := sha256.Sum256([]byte(xpub))
	return gamingfunds.Scope{Game: game, Network: network, Wallet: hex.EncodeToString(fingerprint[:]), Account: account}, nil
}
func GamingFinancialKey(ctx context.Context, game, sid string) (string, error) {
	if sid != "" && !financialTableID.MatchString(sid) {
		return "", fmt.Errorf("invalid table identifier")
	}
	scope, err := gamingFinancialScope(ctx, game)
	if err != nil {
		return "", err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return "", err
	}
	if sid != "" {
		if _, err = store.AuthorizedTable(scope, sid); err != nil {
			return "", err
		}
	}
	pub, err := ensureGamingWalletKey(ctx, store, scope, sid)
	if err != nil {
		return "", err
	}
	if err = announceGamingAuthority(ctx, scope, sid); err != nil {
		return "", err
	}
	return pub, nil
}
func PrepareGamingDeposit(ctx context.Context, game string, req *gamingpb.PrepareDepositRequest) (*gamingpb.PreparedDeposit, error) {
	if req.GetSid() != "" && !financialTableID.MatchString(req.GetSid()) {
		return nil, fmt.Errorf("invalid table identifier")
	}
	scope, err := gamingFinancialScope(ctx, game)
	if err != nil {
		return nil, err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	if req.GetSid() != "" {
		accepted, err := store.AuthorizedTable(scope, req.GetSid())
		if err != nil {
			return nil, err
		}
		if req.GetKind() == "stake" && (accepted.StakeAtoms != req.GetAmountAtoms() || accepted.CSVBlocks != req.GetLockBlocks() || int(accepted.Seats) != len(req.GetMembers())) {
			return nil, fmt.Errorf("stake differs from the invitation approved in the dashboard")
		}
	}
	pub, err := ensureGamingWalletKey(ctx, store, scope, req.GetSid())
	if err != nil {
		return nil, err
	}
	terms := gamingfunds.Terms{Version: gamingfunds.Version, Game: game, Network: scope.Network, Account: scope.Account, Table: req.GetSid(), Kind: req.GetKind(), Atoms: req.GetAmountAtoms(), LockBlocks: req.GetLockBlocks(), Identity: req.GetIdentityKey(), Recovery: pub, Members: req.GetMembers()}
	params, err := chainParams(ctx)
	if err != nil {
		return nil, err
	}
	address, _, err := terms.Output(params)
	if err != nil {
		return nil, err
	}
	if _, err = checkSpendRequest(ReadGamingSettings(), game, address, terms.Atoms); err != nil {
		return nil, err
	}
	dep, err := store.Register(scope, terms, params)
	if err != nil {
		return nil, err
	}
	if err = ImportMsigScript(ctx, dep.Script, false, 0); err != nil {
		return nil, err
	}
	if err = store.VerifyRecovery(scope, dep.ID, params); err != nil {
		return nil, err
	}
	return &gamingpb.PreparedDeposit{Id: dep.ID, Address: dep.Address, RedeemScript: dep.Script, PkScript: dep.PkScript, RecoveryKey: pub}, nil
}

// verifiedGamingPayment cannot be supplied over HTTP/gRPC. It is created only
// after the bridge reads its own immutable ledger and verifies the transaction.
type verifiedGamingPayment struct {
	Deposit  gamingfunds.Deposit
	Unsigned string
	FeeAtoms int64
}

func verifyGamingDeposit(ctx context.Context, game, id string) (gamingfunds.Deposit, error) {
	var empty gamingfunds.Deposit
	scope, err := gamingFinancialScope(ctx, game)
	if err != nil {
		return empty, err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return empty, err
	}
	rows, err := store.Deposits(scope)
	if err != nil {
		return empty, err
	}
	for _, dep := range rows {
		if dep.ID == id {
			if dep.Closed || dep.Outpoint != "" || dep.FundingTx != "" {
				return empty, fmt.Errorf("deposit is closed or already funded")
			}
			accepted, err := store.AuthorizedTable(scope, dep.Terms.Table)
			if err != nil {
				return empty, err
			}
			if dep.Terms.Kind == "seatbond" {
				tip, err := GamingChainTipNow(ctx)
				if err != nil {
					return empty, err
				}
				if tip.Height <= 0 || tip.Height > int64(accepted.Until) {
					return empty, fmt.Errorf("admission deadline passed or chain unavailable")
				}
			}
			params, err := chainParams(ctx)
			if err != nil {
				return empty, err
			}
			address, pk, err := dep.Terms.Output(params)
			if err != nil || address != dep.Address || hex.EncodeToString(pk) != dep.PkScript {
				return empty, fmt.Errorf("stored financial descriptor is inconsistent")
			}
			script, err := dep.Terms.Script()
			if err != nil || hex.EncodeToString(script) != dep.Script {
				return empty, fmt.Errorf("stored redeem script is inconsistent")
			}
			key, keyErr := store.WalletKey(scope, dep.Terms.Table)
			if keyErr != nil {
				return empty, keyErr
			}
			if err = verifyGamingWalletKey(ctx, key); err != nil {
				return empty, err
			}
			if err = store.VerifyRecovery(scope, id, params); err != nil {
				return empty, err
			}
			return dep, nil
		}
	}
	return empty, fmt.Errorf("no verified deposit belongs to this game and wallet")
}
func RequestGamingDepositSpend(ctx context.Context, game string, req *gamingpb.RequestSpendRequest) (*gamingpb.Spend, error) {
	dep, err := verifyGamingDeposit(ctx, game, req.GetDepositId())
	if err != nil {
		return nil, err
	}
	if req.GetAddress() != dep.Address || req.GetAmountAtoms() != dep.Terms.Atoms {
		return nil, fmt.Errorf("payment differs from verified deposit")
	}
	unsigned, err := spendConstruct(ctx, dep.Scope.Account, dep.Address, dep.Terms.Atoms)
	if err != nil {
		return nil, err
	}
	fee, err := validateGamingFunding(ctx, dep, unsigned)
	if err != nil {
		return nil, err
	}
	proof := &verifiedGamingPayment{Deposit: dep, Unsigned: hex.EncodeToString(unsigned), FeeAtoms: fee}
	sp, err := requestGamingSpend(ctx, game, dep.Address, dep.Terms.Atoms, req.GetReason(), proof)
	return spendProto(sp), err
}

// RequestGamingSpend deliberately refuses the old address-only entry point,
// including direct in-process calls that bypass the gRPC adapter.
func RequestGamingSpend(ctx context.Context, game, address string, atoms int64, reason string) (GamingSpend, error) {
	return GamingSpend{}, fmt.Errorf("%w: a bridge-verified deposit and recovery authority are required", ErrGamingSpendRefused)
}

func ensureGamingFundingPreview(request GamingSpend, p *verifiedGamingPayment) (gamingfunds.FundingApproval, error) {
	store, err := gamingFundsStore()
	if err != nil {
		return gamingfunds.FundingApproval{}, err
	}
	return store.EnsurePreview(request.ID, p.Deposit.Scope, gamingfunds.PaymentPreview{
		RequestedAt: request.RequestedAt,
		ExpiresAt:   request.ExpiresAt,
		DepositID:   p.Deposit.ID,
		Unsigned:    p.Unsigned,
		Reason:      request.Reason,
		FeeAtoms:    p.FeeAtoms,
	})
}
func loadGamingFundingPreview(id, deposit string) ([]byte, error) {
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	return store.Preview(id, deposit)
}
func saveGamingSignedFunding(req GamingSpend, dep gamingfunds.Deposit, raw []byte) error {
	store, err := gamingFundsStore()
	if err != nil {
		return err
	}
	return store.CommitFunding(req.ID, dep.Scope, dep.ID, raw)
}

func authorizeGamingTable(ctx context.Context, game, invite, gcid string) error {
	u, err := url.Parse(invite)
	if err != nil || u.Scheme != "gaming" || u.Host != game || u.Path != "/table" {
		return fmt.Errorf("invalid gaming invitation")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return fmt.Errorf("invalid invitation parameters")
	}
	for _, values := range q {
		if len(values) != 1 {
			return fmt.Errorf("duplicate invitation parameter")
		}
	}
	if q.Get("fv") != "2" {
		return fmt.Errorf("financial protocol version 2 required")
	}
	if _, err = parseGamingGCID(gcid); err != nil {
		return err
	}
	sid := q.Get("sid")
	if !financialTableID.MatchString(sid) {
		return fmt.Errorf("invalid table identifier")
	}
	buyin, err := strconv.ParseInt(q.Get("buyin"), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid stake amount")
	}
	csv, err := strconv.ParseUint(q.Get("csv"), 10, 32)
	if err != nil {
		return fmt.Errorf("invalid refund lock")
	}
	seats, err := strconv.ParseUint(q.Get("seats"), 10, 32)
	if err != nil {
		return fmt.Errorf("invalid seat count")
	}
	until, err := strconv.ParseUint(q.Get("until"), 10, 32)
	if err != nil {
		return fmt.Errorf("invalid admission deadline")
	}
	scope, err := gamingFinancialScope(ctx, game)
	if err != nil {
		return err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return err
	}
	bond, e := strconv.ParseInt(q.Get("bond"), 10, 64)
	if e != nil {
		return fmt.Errorf("invalid admission amount")
	}
	bondcsv, e := strconv.ParseUint(q.Get("bondcsv"), 10, 32)
	if e != nil {
		return fmt.Errorf("invalid admission refund delay")
	}
	tablebond, e := strconv.ParseInt(q.Get("tablebond"), 10, 64)
	if e != nil {
		return fmt.Errorf("invalid table bond amount")
	}
	tablebondcsv, e := strconv.ParseUint(q.Get("tablebondcsv"), 10, 32)
	if e != nil {
		return fmt.Errorf("invalid table bond delay")
	}
	return store.AuthorizeTable(gamingfunds.TableAuthorization{Scope: scope, Table: sid, StakeAtoms: buyin, CSVBlocks: uint32(csv), Seats: uint32(seats), Until: uint32(until), Group: gcid, AdmissionAtoms: bond, AdmissionBlocks: uint32(bondcsv), TableBondAtoms: tablebond, TableBondBlocks: uint32(tablebondcsv)})
}

func GamingFinancialKeyReply(ctx context.Context, game, sid string) (*gamingpb.FinancialKeyReply, error) {
	pub, err := GamingFinancialKey(ctx, game, sid)
	if err != nil {
		return nil, err
	}
	scope, err := gamingFinancialScope(ctx, game)
	if err != nil {
		return nil, err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	key, err := store.WalletKey(scope, sid)
	if err != nil {
		return nil, err
	}
	return &gamingpb.FinancialKeyReply{PublicKey: pub, PayoutAddress: key.Address}, nil
}

func GamingFinancialState(ctx context.Context, game, sid string) (*gamingpb.FinancialStateReply, error) {
	scope, err := gamingFinancialScope(ctx, game)
	if err != nil {
		return nil, err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return nil, err
	}
	tables, err := store.Tables()
	if err != nil {
		return nil, err
	}
	var accepted *gamingfunds.TableAuthorization
	for i := range tables {
		if tables[i].Scope == scope && tables[i].Table == sid {
			accepted = &tables[i]
			break
		}
	}
	if accepted == nil {
		return nil, fmt.Errorf("unknown table")
	}
	out := &gamingpb.FinancialStateReply{Sid: sid, Closed: accepted.Closed, TermsHash: accepted.TermsHash()}
	key, err := store.WalletKey(scope, sid)
	if err == nil {
		out.PayoutAddress = key.Address
	}
	deps, err := store.Deposits(scope)
	if err != nil {
		return nil, err
	}
	for _, dep := range deps {
		if dep.Terms.Table == sid {
			out.Deposits = append(out.Deposits, &gamingpb.DepositStatus{Id: dep.ID, Kind: dep.Terms.Kind, State: dep.State, Outpoint: dep.Outpoint, AmountAtoms: dep.Terms.Atoms, LockBlocks: dep.Terms.LockBlocks, Confirmations: dep.Confirmations})
		}
	}
	return out, nil
}
