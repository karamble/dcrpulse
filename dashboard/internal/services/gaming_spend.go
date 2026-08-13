// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/decred/dcrd/wire"

	"dcrpulse/internal/types"
)

// Games play for real money, and this is the one place any of it moves.
//
// Everywhere else the bridge is a tunnel that forms no opinion, because a host
// that judged a fold would be a party to the game. Here it forms every opinion
// it has: which account may be spent from, how much at once, how much in a day,
// and whether a person said yes. That division is the whole design - policy
// belongs where money moves, not where messages do.
//
// What it enforces against is the game's bearer token, which is an identity
// rather than a password. A shared secret would make every game one principal,
// and a cap on a principal nobody can tell apart is not a cap.
//
// Nothing is spent without a person. The dashboard holds no wallet passphrase -
// every send route takes one from the user and wipes it - so a game cannot be
// autopaid even in principle, and the mode that says otherwise is refused
// rather than quietly downgraded.

// GamingSpendState is what became of a request.
type GamingSpendState string

const (
	GamingSpendPending  GamingSpendState = "pending"
	GamingSpendApproved GamingSpendState = "approved"
	GamingSpendDenied   GamingSpendState = "denied"
	GamingSpendExpired  GamingSpendState = "expired"
	GamingSpendFailed   GamingSpendState = "failed"
)

// GamingSpend is one request a game made, and what was decided.
type GamingSpend struct {
	ID          string           `json:"id"`
	Game        string           `json:"game"`
	Address     string           `json:"address"`
	AmountAtoms int64            `json:"amountAtoms"`
	Reason      string           `json:"reason,omitempty"`
	State       GamingSpendState `json:"state"`
	TxID        string           `json:"txid,omitempty"`
	Error       string           `json:"error,omitempty"`
	RequestedAt int64            `json:"requestedAt"`
	DecidedAt   int64            `json:"decidedAt,omitempty"`
	// ExpiresAt is when an unanswered request stops being answerable. A
	// game waiting on one needs to know the waiting ends.
	ExpiresAt int64 `json:"expiresAt"`
}

// Pending reports whether the request is still awaiting a person.
func (s GamingSpend) Pending() bool { return s.State == GamingSpendPending }

var (
	// ErrGamingSpendRefused is a request policy will not carry.
	ErrGamingSpendRefused = errors.New("spend refused by policy")

	// ErrGamingSpendNotFound is an id nobody asked for, or not this game's.
	ErrGamingSpendNotFound = errors.New("no such spend request")

	// ErrGamingSpendNotPending is a request already decided.
	ErrGamingSpendNotPending = errors.New("spend request already decided")
)

// spendMu serialises the read-modify-write of the log. Two games asking at once
// must not each see the other's spend as not yet counted.
var spendMu sync.Mutex

// spendApproving is the requests a person is approving right now. The spend
// itself runs outside spendMu - signing and broadcasting take as long as they
// take - and a request that stays pending while it runs would let a second
// approval pass the Pending check and pay twice. Guarded by spendMu.
var spendApproving = map[string]bool{}

// The staged calls a game's money passes through: the account it resolves to,
// the transaction built for it, the signature a person authorises, the relay
// that puts it on the network, and what the node says an input was.
//
// They are settable because the rules around them are the only thing between an
// untrusted game and this wallet, and a rule that can only be exercised against
// a live wallet and a live node is a rule nobody has exercised. Production sets
// none of them.
var (
	spendAccount   = gamingAccountNumber
	spendConstruct = func(ctx context.Context, account uint32, address string, amountAtoms int64) ([]byte, error) {
		tx, err := ConstructTransaction(ctx, account, []types.TxRecipient{{
			Address: address, AmountAtoms: amountAtoms,
		}}, false)
		if err != nil {
			return nil, err
		}
		return tx.UnsignedTransaction, nil
	}
	spendSign    = signTransactionForSpend
	spendPublish = publishSignedTransaction
	spendPrevout = lookupGamingPrevout
)

// GamingWalletCalls is that same set, named, so a caller outside this package
// can stage them. A nil field is left alone.
type GamingWalletCalls struct {
	Account   func(ctx context.Context) (uint32, error)
	Construct func(ctx context.Context, account uint32, address string, amountAtoms int64) ([]byte, error)
	Sign      func(ctx context.Context, account uint32, unsigned, passphrase []byte) ([]byte, error)
	Publish   func(ctx context.Context, signed []byte) (string, error)
	Prevout   func(ctx context.Context, op wire.OutPoint) (GamingPrevout, error)
}

// SetGamingWalletCalls installs the non-nil calls and returns a func putting
// every one of them back. It is the seam the bridge's own tests drive the money
// paths through; nothing in production calls it.
func SetGamingWalletCalls(c GamingWalletCalls) (restore func()) {
	prevAccount, prevConstruct := spendAccount, spendConstruct
	prevSign, prevPublish, prevPrevout := spendSign, spendPublish, spendPrevout

	if c.Account != nil {
		spendAccount = c.Account
	}
	if c.Construct != nil {
		spendConstruct = c.Construct
	}
	if c.Sign != nil {
		spendSign = c.Sign
	}
	if c.Publish != nil {
		spendPublish = c.Publish
	}
	if c.Prevout != nil {
		spendPrevout = c.Prevout
	}
	return func() {
		spendAccount, spendConstruct = prevAccount, prevConstruct
		spendSign, spendPublish, spendPrevout = prevSign, prevPublish, prevPrevout
	}
}

// spendLog is the whole record, oldest first.
type spendLog struct {
	Spends []GamingSpend `json:"spends"`
}

// maxSpendLog bounds the file. It is an audit trail for a person, not an
// accounting system, and the day cap only ever looks a day back.
const maxSpendLog = 500

func readSpendLog() spendLog {
	var log spendLog
	blob, err := os.ReadFile(gamingSpendLogPath())
	if err != nil {
		return spendLog{}
	}
	if err := json.Unmarshal(blob, &log); err != nil {
		return spendLog{}
	}
	return log
}

func writeSpendLog(log spendLog) error {
	if len(log.Spends) > maxSpendLog {
		log.Spends = log.Spends[len(log.Spends)-maxSpendLog:]
	}
	dir := filepath.Dir(gamingSpendLogPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create control directory: %w", err)
	}
	blob, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return err
	}
	tmp := gamingSpendLogPath() + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return fmt.Errorf("write spend log: %w", err)
	}
	return os.Rename(tmp, gamingSpendLogPath())
}

// expireLocked marks anything nobody answered in time. A request that stays
// pending forever is a game blocked forever, which is worse than a refusal.
func expireLocked(log *spendLog, now int64) bool {
	changed := false
	for i := range log.Spends {
		s := &log.Spends[i]
		if s.State == GamingSpendPending && now >= s.ExpiresAt {
			s.State, s.DecidedAt = GamingSpendExpired, now
			changed = true
		}
	}
	return changed
}

// spentInDayLocked totals what has actually left the wallet in the last
// twenty-four hours, plus what is still pending.
//
// Pending counts. A cap that ignored outstanding requests could be walked past
// by asking several times before anyone answered.
func spentInDayLocked(log spendLog, now int64) int64 {
	var total int64
	for _, s := range log.Spends {
		switch s.State {
		case GamingSpendApproved:
			if now-s.DecidedAt < int64((24 * time.Hour).Seconds()) {
				total += s.AmountAtoms
			}
		case GamingSpendPending:
			total += s.AmountAtoms
		}
	}
	return total
}

// checkSpendRequest is the policy decision that needs nothing but the request.
//
// The caller supplies the policy rather than this reading it, because the
// bridge holds one per connected game and must not re-read a file for every
// request a table makes. It is also what lets the whole rule be written out in
// a test instead of sampled through whatever happens to be on disk.
func checkSpendRequest(s types.GamingSettings, installed bool, address string, amountAtoms int64) error {
	if !s.Enabled {
		return fmt.Errorf("%w: gaming is switched off", ErrGamingSpendRefused)
	}
	if strings.TrimSpace(s.Account) == "" {
		return fmt.Errorf("%w: no account is bound for games to spend from", ErrGamingSpendRefused)
	}
	if !installed {
		return ErrGamingGameNotInstalled
	}
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("%w: no address to pay", ErrGamingSpendRefused)
	}
	if amountAtoms <= 0 {
		return fmt.Errorf("%w: nothing to pay", ErrGamingSpendRefused)
	}
	if s.PerTableCapAtoms > 0 && amountAtoms > s.PerTableCapAtoms {
		return fmt.Errorf("%w: %d atoms is over the per-table cap of %d",
			ErrGamingSpendRefused, amountAtoms, s.PerTableCapAtoms)
	}
	if s.Mode == gamingModeAutopay {
		// Not a policy choice, a fact: this process never holds a
		// wallet passphrase - every send route takes one from the user
		// and wipes it - so there is nothing here that could pay
		// without asking. Refusing says so; treating it as approval
		// would make the setting a lie.
		return fmt.Errorf(
			"%w: automatic payment needs a wallet passphrase this never holds; use approval mode",
			ErrGamingSpendRefused)
	}
	return nil
}

// checkSpendAgainstDay is the rest of the decision, once the day's total is
// known.
func checkSpendAgainstDay(s types.GamingSettings, amountAtoms, used int64) error {
	if s.PerDayCapAtoms <= 0 || used+amountAtoms <= s.PerDayCapAtoms {
		return nil
	}
	return fmt.Errorf(
		"%w: %d atoms would pass the daily cap of %d, with %d already spent or awaiting an answer",
		ErrGamingSpendRefused, amountAtoms, s.PerDayCapAtoms, used)
}

// RequestGamingSpend records a game's request, if policy will carry it.
//
// It does not spend. Nothing here can: the passphrase belongs to the person,
// and this only gets as far as asking them.
func RequestGamingSpend(game, address string, amountAtoms int64, reason string) (GamingSpend, error) {
	s := ReadGamingSettings()
	game = strings.ToLower(strings.TrimSpace(game))
	address = strings.TrimSpace(address)

	if err := checkSpendRequest(s, gamingGameInstalled(game), address, amountAtoms); err != nil {
		return GamingSpend{}, err
	}

	spendMu.Lock()
	defer spendMu.Unlock()

	now := time.Now().Unix()
	log := readSpendLog()
	expireLocked(&log, now)

	if err := checkSpendAgainstDay(s, amountAtoms, spentInDayLocked(log, now)); err != nil {
		return GamingSpend{}, err
	}

	timeout := s.ApprovalTimeoutSecs
	if timeout <= 0 {
		timeout = gamingDefaultApprovalSecs
	}
	id, err := newSpendID()
	if err != nil {
		return GamingSpend{}, err
	}
	out := GamingSpend{
		ID:          id,
		Game:        game,
		Address:     address,
		AmountAtoms: amountAtoms,
		Reason:      strings.TrimSpace(reason),
		State:       GamingSpendPending,
		RequestedAt: now,
		ExpiresAt:   now + int64(timeout),
	}
	log.Spends = append(log.Spends, out)
	if err := writeSpendLog(log); err != nil {
		return GamingSpend{}, err
	}
	return out, nil
}

// GamingSpendFor reports on one request, for the game that made it.
//
// Scoped to the caller on purpose: a game asking about another game's spending
// is a question it has no business having answered.
func GamingSpendFor(game, id string) (GamingSpend, error) {
	spendMu.Lock()
	defer spendMu.Unlock()

	now := time.Now().Unix()
	log := readSpendLog()
	if expireLocked(&log, now) {
		_ = writeSpendLog(log)
	}
	for _, s := range log.Spends {
		if s.ID == id && s.Game == strings.ToLower(strings.TrimSpace(game)) {
			return s, nil
		}
	}
	return GamingSpend{}, ErrGamingSpendNotFound
}

// GamingSpends reports the whole log, newest first, for a person to read.
func GamingSpends() []GamingSpend {
	spendMu.Lock()
	defer spendMu.Unlock()

	now := time.Now().Unix()
	log := readSpendLog()
	if expireLocked(&log, now) {
		_ = writeSpendLog(log)
	}
	out := append([]GamingSpend(nil), log.Spends...)
	sort.Slice(out, func(i, j int) bool { return out[i].RequestedAt > out[j].RequestedAt })
	return out
}

// DenyGamingSpend refuses a request without spending anything.
func DenyGamingSpend(id string) (GamingSpend, error) {
	spendMu.Lock()
	defer spendMu.Unlock()

	now := time.Now().Unix()
	log := readSpendLog()
	expireLocked(&log, now)

	for i := range log.Spends {
		s := &log.Spends[i]
		if s.ID != id {
			continue
		}
		if !s.Pending() {
			return *s, ErrGamingSpendNotPending
		}
		s.State, s.DecidedAt = GamingSpendDenied, now
		if err := writeSpendLog(log); err != nil {
			return GamingSpend{}, err
		}
		return *s, nil
	}
	return GamingSpend{}, ErrGamingSpendNotFound
}

// ApproveGamingSpend pays a request, with the passphrase the person supplied.
//
// The amount and address are the ones recorded when the request was made, never
// anything the approver passes in - so what is paid is what was shown, and a
// request cannot be edited between being seen and being signed.
//
// What an error decides depends on where it happened. Everything up to and
// including signing is pre-broadcast by construction - resolving the account,
// constructing the transaction, unlocking with the passphrase - so a failure
// there leaves the request pending and the person free to try again; a
// mistyped passphrase costs a retype, not the table. Only publishing is
// ambiguous: the transaction may have been relayed despite the error, and a
// request left approvable after a possible broadcast is the documented
// double-payment. That one records failed, as every failure used to.
func ApproveGamingSpend(ctx context.Context, id string, passphrase []byte) (GamingSpend, error) {
	spendMu.Lock()
	now := time.Now().Unix()
	log := readSpendLog()
	expireLocked(&log, now)

	idx := -1
	for i := range log.Spends {
		if log.Spends[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		spendMu.Unlock()
		return GamingSpend{}, ErrGamingSpendNotFound
	}
	if !log.Spends[idx].Pending() {
		out := log.Spends[idx]
		spendMu.Unlock()
		return out, ErrGamingSpendNotPending
	}
	// Pending in the log is no longer enough: a retryable failure leaves the
	// request pending on purpose, so the state alone cannot tell "waiting on
	// a person" from "a person's approval is running right now" - and two
	// copies of the second would each construct their own transaction and
	// pay the address twice.
	if spendApproving[id] {
		out := log.Spends[idx]
		spendMu.Unlock()
		return out, ErrGamingSpendNotPending
	}
	spendApproving[id] = true
	req := log.Spends[idx]
	spendMu.Unlock()
	defer func() {
		spendMu.Lock()
		delete(spendApproving, id)
		spendMu.Unlock()
	}()

	// Spend outside the lock: signing and broadcasting take as long as they
	// take, and holding the log shut meanwhile would stall every other game.
	account, err := spendAccount(ctx)
	if err != nil {
		return req, err
	}
	unsigned, err := spendConstruct(ctx, account, req.Address, req.AmountAtoms)
	if err != nil {
		return req, err
	}
	signed, err := spendSign(ctx, account, unsigned, passphrase)
	if err != nil {
		return req, err
	}
	txid, err := spendPublish(ctx, signed)
	if err != nil {
		return recordSpendOutcome(id, GamingSpendFailed, "", err.Error())
	}
	return recordSpendOutcome(id, GamingSpendApproved, txid, "")
}

// recordSpendOutcome writes what happened, whatever happened. A spend that was
// broadcast and not recorded would be one nobody could account for.
func recordSpendOutcome(id string, state GamingSpendState, txid, failure string) (GamingSpend, error) {
	spendMu.Lock()
	defer spendMu.Unlock()

	log := readSpendLog()
	for i := range log.Spends {
		s := &log.Spends[i]
		if s.ID != id {
			continue
		}
		s.State, s.TxID, s.Error = state, txid, failure
		s.DecidedAt = time.Now().Unix()
		if err := writeSpendLog(log); err != nil {
			return *s, err
		}
		if state != GamingSpendApproved {
			return *s, fmt.Errorf("spend failed: %s", failure)
		}
		return *s, nil
	}
	return GamingSpend{}, ErrGamingSpendNotFound
}

// gamingAccountNumber resolves the account games may spend from.
//
// Account scope is enforced here rather than by the wallet because dcrwallet's
// accounts share one seed and one passphrase: an account is a boundary above
// the wallet, never a cryptographic one below it.
func gamingAccountNumber(ctx context.Context) (uint32, error) {
	name := strings.TrimSpace(ReadGamingSettings().Account)
	if name == "" {
		return 0, fmt.Errorf("no account is bound for games to spend from")
	}
	accounts, err := FetchAllAccounts(ctx)
	if err != nil {
		return 0, fmt.Errorf("read accounts: %w", err)
	}
	for _, a := range accounts {
		if a.AccountName == name {
			return a.AccountNumber, nil
		}
	}
	return 0, fmt.Errorf("the account bound for games, %q, is not in this wallet", name)
}

func newSpendID() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
