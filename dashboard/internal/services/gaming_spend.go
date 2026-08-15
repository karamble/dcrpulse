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
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"
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
// What it enforces against is the game's credential, which is an identity
// rather than a password. A shared secret would make every game one principal,
// and a cap on a principal nobody can tell apart is not a cap.
//
// Nothing is spent without a person, and there is no setting saying otherwise.
// The dashboard holds no wallet passphrase - every send route takes one from
// the user and wipes it - so a game cannot be paid automatically even in
// principle. There was a mode offering it: it could be chosen, it was stored,
// and then every spend was refused with a message only the game ever saw. A
// choice that can only be refused is worse than no choice at all.

// GamingSpendState is what became of a request.
type GamingSpendState string

const (
	GamingSpendPending  GamingSpendState = "pending"
	GamingSpendApproved GamingSpendState = "approved"
	GamingSpendDenied   GamingSpendState = "denied"
	GamingSpendExpired  GamingSpendState = "expired"
	GamingSpendFailed   GamingSpendState = "failed"

	// GamingSpendPublishing is the moment between a person's approval
	// being signed and the network's answer being recorded. Money may be
	// moving, so nothing may decide, expire or trim an entry in this
	// state - and it never reaches a game, which is only ever told
	// pending or an outcome.
	GamingSpendPublishing GamingSpendState = "publishing"
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

	// ErrGamingSpendOverCap is a spend refused for being too large, as
	// opposed to one refused because the section is not set up to pay
	// anything at all.
	//
	// It wraps ErrGamingSpendRefused, so everything that already asks "was
	// this refused" keeps working. The distinction exists because the two
	// deserve different answers: a cap is the operator's standing decision
	// working exactly as intended, and a game told about one can wait and
	// ask for less. An unbound account is something the operator has not
	// done yet, and a game told it was over a limit would send them looking
	// at the wrong screen. A backlog of unanswered requests refuses under
	// the same name, because waiting is the same right answer.
	ErrGamingSpendOverCap = fmt.Errorf("%w: over a cap", ErrGamingSpendRefused)

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
	Account   func(ctx context.Context, game string) (uint32, error)
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

// maxSpendLog bounds the file. The trim never drops an entry the day cap
// still counts - see spendCountsOrPends - so the bound only ever costs old
// decided history, never the accounting.
const maxSpendLog = 500

// maxSpendAtoms is the total DCR supply in atoms; no single request can
// legitimately name more, whatever the caps say.
const maxSpendAtoms = int64(dcrutil.MaxAmount)

// spendDaySecs is the day the daily cap looks back on. One constant, so
// counting and trimming can never disagree about what a day is.
const spendDaySecs = int64(24 * time.Hour / time.Second)

// maxPendingSpendsPerGame bounds how many unanswered requests one game may
// hold open. Poker needs two per table it is joining and one bond; eight is
// headroom. The bound is also what keeps a game from churning the log by
// asking without end.
const maxPendingSpendsPerGame = 8

// spendPublishingGraceSecs is how long past its window a payment may stay
// publishing before it is read as interrupted. Long enough for the slowest
// honest broadcast, short enough that a crash surfaces the same day.
const spendPublishingGraceSecs = int64(300)

// spendInterruptedText is what an interrupted broadcast records. One fixed
// sentence, so the console and the tests say the same thing.
const spendInterruptedText = "interrupted while broadcasting; check the wallet for the transaction before paying it again"

// readSpendLog reads the whole record. A missing file is a legitimate fresh
// start; anything else unreadable is an error, because this file is both the
// audit trail and the counter the daily allowance is computed from - reading
// corruption as an empty log would answer with a day nobody spent and a
// history nobody kept.
func readSpendLog() (spendLog, error) {
	var log spendLog
	blob, err := os.ReadFile(gamingSpendLogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return spendLog{}, nil
		}
		return spendLog{}, fmt.Errorf("read spend log: %w", err)
	}
	if err := json.Unmarshal(blob, &log); err != nil {
		return spendLog{}, fmt.Errorf("parse spend log: %w", err)
	}
	return log, nil
}

func writeSpendLog(log spendLog, now int64) error {
	if len(log.Spends) > maxSpendLog {
		drop := len(log.Spends) - maxSpendLog
		kept := make([]GamingSpend, 0, maxSpendLog)
		for _, s := range log.Spends {
			if drop > 0 && !spendCountsOrPends(s, now) {
				drop--
				continue
			}
			kept = append(kept, s)
		}
		// Oldest droppable entries go first; if everything still counts,
		// everything is kept - the bound yields to the accounting, never
		// the other way round.
		log.Spends = kept
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

// spentInDayLocked totals what one game has moved out of the wallet in the last
// twenty-four hours, plus what it still has waiting on a person.
//
// Per game, because the cap is. Summed across every game, one busy table
// consumes the allowance of a game that has asked for nothing, and that game is
// then refused for money it never spent - a limit that falls on the wrong
// principal is not a limit on anybody.
//
// Pending counts. A cap that ignored outstanding requests could be walked past
// by asking several times before anyone answered.
func spentInDayLocked(log spendLog, game string, now int64) int64 {
	var total int64
	for _, s := range log.Spends {
		if s.Game != game || !spendCountsOrPends(s, now) {
			continue
		}
		if s.AmountAtoms < 0 || s.AmountAtoms > math.MaxInt64-total {
			// A negative amount is a log this code never wrote, and a
			// total past counting is the same answer either way: a day
			// already full, never a number that wrapped.
			return math.MaxInt64
		}
		total += s.AmountAtoms
	}
	return total
}

// spendOutstanding reports whether an entry is still in flight: awaiting a
// person, or approved and being broadcast. Both hold a slot in the per-game
// ceiling, because both are exposure nobody has finished accounting for.
func spendOutstanding(s GamingSpend) bool {
	return s.State == GamingSpendPending || s.State == GamingSpendPublishing
}

// spendCountsOrPends reports whether an entry still feeds the daily cap:
// still in flight, or approved inside the day the cap looks back on. The
// trim keeps exactly what this counts, which is what makes the cap window
// untrimmable.
func spendCountsOrPends(s GamingSpend, now int64) bool {
	return spendOutstanding(s) ||
		(s.State == GamingSpendApproved && now-s.DecidedAt < spendDaySecs)
}

// sweepPublishingLocked settles what a crash left mid-broadcast. An entry
// still publishing long after its window, with no approval running in this
// process, is a payment whose answer was lost: it fails with the sentence
// that says where to look. An id in spendApproving is a broadcast running
// right here and is never touched, however late it runs. A swept entry
// stops counting toward the day, the same accounting the ambiguous-publish
// arm has always had.
func sweepPublishingLocked(log *spendLog, now int64) bool {
	changed := false
	for i := range log.Spends {
		s := &log.Spends[i]
		if s.State != GamingSpendPublishing || spendApproving[s.ID] {
			continue
		}
		if now >= s.ExpiresAt+spendPublishingGraceSecs {
			s.State, s.DecidedAt, s.Error = GamingSpendFailed, now, spendInterruptedText
			changed = true
		}
	}
	return changed
}

// checkSpendRequest is the policy decision that needs nothing but the request,
// and it returns the policy it decided against so the caller need not look the
// same game up twice.
//
// Whether a game is registered is read from the policy map rather than from a
// second list, because the two are kept in step and a rule consulting whichever
// it happens to hold is a rule that can disagree with itself.
func checkSpendRequest(s types.GamingSettings, game, address string, amountAtoms int64) (types.GamePolicy, error) {
	if !s.Enabled {
		return types.GamePolicy{}, fmt.Errorf("%w: gaming is switched off", ErrGamingSpendRefused)
	}
	p, registered := s.Policies[game]
	if !registered {
		return types.GamePolicy{}, ErrGamingGameNotRegistered
	}
	if strings.TrimSpace(p.Account) == "" {
		return p, fmt.Errorf("%w: no account is bound for %q to spend from",
			ErrGamingSpendRefused, game)
	}
	if strings.TrimSpace(address) == "" {
		return p, fmt.Errorf("%w: no address to pay", ErrGamingSpendRefused)
	}
	if amountAtoms <= 0 {
		return p, fmt.Errorf("%w: nothing to pay", ErrGamingSpendRefused)
	}
	if amountAtoms > maxSpendAtoms {
		return p, fmt.Errorf("%w: %d atoms is more than the whole supply",
			ErrGamingSpendRefused, amountAtoms)
	}
	if p.PerTableCapAtoms > 0 && amountAtoms > p.PerTableCapAtoms {
		return p, fmt.Errorf("%w: %d atoms is over %q's per-table cap of %d",
			ErrGamingSpendOverCap, amountAtoms, game, p.PerTableCapAtoms)
	}
	return p, nil
}

// checkSpendAgainstDay is the rest of the decision, once this game's day total
// is known.
//
// Phrased as a bound on the request rather than a sum, because a sum's
// operands are a caller's to choose: used+amount can wrap, and a wrapped
// total reads as under any cap. A negative total is a log this code never
// wrote, and it fails closed rather than granting headroom nobody spent.
func checkSpendAgainstDay(p types.GamePolicy, amountAtoms, used int64) error {
	if p.PerDayCapAtoms <= 0 {
		return nil
	}
	if used >= 0 && amountAtoms <= p.PerDayCapAtoms-used {
		return nil
	}
	return fmt.Errorf(
		"%w: %d atoms would pass the daily cap of %d, with %d already spent or awaiting an answer",
		ErrGamingSpendOverCap, amountAtoms, p.PerDayCapAtoms, used)
}

// RequestGamingSpend records a game's request, if policy will carry it.
//
// It does not spend. Nothing here can: the passphrase belongs to the person,
// and this only gets as far as asking them.
func RequestGamingSpend(game, address string, amountAtoms int64, reason string) (GamingSpend, error) {
	s := ReadGamingSettings()
	game = strings.ToLower(strings.TrimSpace(game))
	address = strings.TrimSpace(address)

	policy, err := checkSpendRequest(s, game, address, amountAtoms)
	if err != nil {
		return GamingSpend{}, err
	}

	spendMu.Lock()
	defer spendMu.Unlock()

	now := time.Now().Unix()
	log, err := readSpendLog()
	if err != nil {
		return GamingSpend{}, err
	}
	expireLocked(&log, now)
	sweepPublishingLocked(&log, now)

	// A backlog past answering is refused, not queued: nothing here writes,
	// so a game that keeps asking wears out nothing but its own turn.
	pending := 0
	for _, sp := range log.Spends {
		if sp.Game == game && spendOutstanding(sp) {
			pending++
		}
	}
	if pending >= maxPendingSpendsPerGame {
		return GamingSpend{}, fmt.Errorf("%w: %q already has %d requests awaiting an answer",
			ErrGamingSpendOverCap, game, pending)
	}

	if err := checkSpendAgainstDay(policy, amountAtoms, spentInDayLocked(log, game, now)); err != nil {
		return GamingSpend{}, err
	}

	timeout := policy.ApprovalTimeoutSecs
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
	if err := writeSpendLog(log, now); err != nil {
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
	log, err := readSpendLog()
	if err != nil {
		return GamingSpend{}, err
	}
	changed := expireLocked(&log, now)
	if sweepPublishingLocked(&log, now) {
		changed = true
	}
	if changed {
		_ = writeSpendLog(log, now)
	}
	for _, s := range log.Spends {
		if s.ID == id && s.Game == strings.ToLower(strings.TrimSpace(game)) {
			return s, nil
		}
	}
	return GamingSpend{}, ErrGamingSpendNotFound
}

// GamingSpends reports the whole log, newest first, for a person to read. An
// unreadable log is reported as itself, never as an empty history.
func GamingSpends() ([]GamingSpend, error) {
	spendMu.Lock()
	defer spendMu.Unlock()

	now := time.Now().Unix()
	log, err := readSpendLog()
	if err != nil {
		return nil, err
	}
	changed := expireLocked(&log, now)
	if sweepPublishingLocked(&log, now) {
		changed = true
	}
	if changed {
		_ = writeSpendLog(log, now)
	}
	out := append([]GamingSpend(nil), log.Spends...)
	sort.Slice(out, func(i, j int) bool { return out[i].RequestedAt > out[j].RequestedAt })
	return out, nil
}

// DenyGamingSpend refuses a request without spending anything.
func DenyGamingSpend(id string) (GamingSpend, error) {
	spendMu.Lock()
	defer spendMu.Unlock()

	now := time.Now().Unix()
	log, err := readSpendLog()
	if err != nil {
		return GamingSpend{}, err
	}
	expireLocked(&log, now)
	sweepPublishingLocked(&log, now)

	for i := range log.Spends {
		s := &log.Spends[i]
		if s.ID != id {
			continue
		}
		// An approval running for this id right now also refuses: a deny
		// that reported success while the payment went out would be worse
		// than any refusal.
		if !s.Pending() || spendApproving[s.ID] {
			return *s, ErrGamingSpendNotPending
		}
		s.State, s.DecidedAt = GamingSpendDenied, now
		if err := writeSpendLog(log, now); err != nil {
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
	log, err := readSpendLog()
	if err != nil {
		spendMu.Unlock()
		return GamingSpend{}, err
	}
	expireLocked(&log, now)
	sweepPublishingLocked(&log, now)

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
	account, err := spendAccount(ctx, req.Game)
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
	// The broadcast is written down before it happens. After the network
	// has the transaction it is too late for a crash to be harmless: an
	// entry still pending on disk while the coins moved is a request a
	// person can be asked to pay twice. Failing here aborts before any
	// money moves, which costs a retry and nothing else - and it is the
	// last look at the clock, so an approval that outlived its window
	// stops here instead of publishing anyway.
	if err := markSpendPublishing(id); err != nil {
		return req, err
	}
	txid, err := spendPublish(ctx, signed)
	if err != nil {
		return recordSpendOutcome(req, GamingSpendFailed, "", err.Error())
	}
	return recordSpendOutcome(req, GamingSpendApproved, txid, "")
}

// markSpendPublishing records that a signed transaction is about to be
// handed to the network. Called with an approval in flight and spendMu not
// held; any error aborts the approval pre-broadcast, retryably.
func markSpendPublishing(id string) error {
	spendMu.Lock()
	defer spendMu.Unlock()

	now := time.Now().Unix()
	log, err := readSpendLog()
	if err != nil {
		return err
	}
	changed := expireLocked(&log, now)
	if sweepPublishingLocked(&log, now) {
		changed = true
	}
	for i := range log.Spends {
		s := &log.Spends[i]
		if s.ID != id {
			continue
		}
		if !s.Pending() {
			// The refusal stands either way; what expiry just decided
			// is still worth writing down.
			if changed {
				_ = writeSpendLog(log, now)
			}
			return ErrGamingSpendNotPending
		}
		s.State = GamingSpendPublishing
		return writeSpendLog(log, now)
	}
	return ErrGamingSpendNotFound
}

// recordSpendOutcome writes what the network said about a payment that was
// marked publishing. Whatever happened, something is written: a spend that was
// broadcast and not recorded would be one nobody could account for.
//
// Only publishing may take an outcome. An outcome is final the moment it is
// written, and a request in any other state is not being paid by this call -
// so a decided entry can never be rewritten, however the paths interleave. A
// refusal is logged with the txid, so a broadcast is never silently
// unaccounted even when the log will not carry it.
func recordSpendOutcome(req GamingSpend, state GamingSpendState, txid, failure string) (GamingSpend, error) {
	spendMu.Lock()
	defer spendMu.Unlock()

	now := time.Now().Unix()
	log, err := readSpendLog()
	if err != nil {
		// Money already moved, so the record lands even when the log
		// cannot be read: the unreadable bytes are set aside for a
		// person to look at, never deleted, and the outcome starts a
		// fresh log. Losing the rolling day counter here is the lesser
		// wrong - the greater one is a broadcast nobody wrote down.
		aside := fmt.Sprintf("%s.corrupt-%d", gamingSpendLogPath(), now)
		if renameErr := os.Rename(gamingSpendLogPath(), aside); renameErr != nil {
			gameLog.Errorf("set the unreadable spend log aside: %v", renameErr)
		}
		gameLog.Errorf("spend log unreadable while recording spend %s (txid %q): %v; the old bytes are at %s",
			req.ID, txid, err, aside)
		out := req
		out.State, out.TxID, out.Error, out.DecidedAt = state, txid, failure, now
		if werr := writeSpendLog(spendLog{Spends: []GamingSpend{out}}, now); werr != nil {
			gameLog.Errorf("record the outcome of spend %s (txid %q): %v", req.ID, txid, werr)
			return out, werr
		}
		if state != GamingSpendApproved {
			return out, fmt.Errorf("spend failed: %s", failure)
		}
		return out, nil
	}
	for i := range log.Spends {
		s := &log.Spends[i]
		if s.ID != req.ID {
			continue
		}
		if s.State != GamingSpendPublishing {
			gameLog.Errorf("refusing to record %s over %s for spend %s (txid %q): the log no longer holds it as being paid",
				state, s.State, req.ID, txid)
			return *s, fmt.Errorf("the log no longer holds request %s as being paid", req.ID)
		}
		s.State, s.TxID, s.Error = state, txid, failure
		s.DecidedAt = now
		if err := writeSpendLog(log, now); err != nil {
			gameLog.Errorf("record the outcome of spend %s (txid %q): %v", req.ID, txid, err)
			return *s, err
		}
		if state != GamingSpendApproved {
			return *s, fmt.Errorf("spend failed: %s", failure)
		}
		return *s, nil
	}
	return GamingSpend{}, ErrGamingSpendNotFound
}

// gamingAccountFor names the account a game may spend from.
//
// Split out from the wallet lookup below so the rule can be exercised: which
// account a game's money comes from is the most consequential answer in this
// file, and a rule that can only be checked against a live wallet is a rule
// nobody has checked.
//
// Account scope is enforced here rather than by the wallet because dcrwallet's
// accounts share one seed and one passphrase: an account is a boundary above
// the wallet, never a cryptographic one below it. What it does buy is a
// bankroll - what one game loses is not drawn from another's.
func gamingAccountFor(s types.GamingSettings, game string) (string, error) {
	p, registered := s.Policies[game]
	if !registered {
		// Unregistering between a request and its approval revokes, so
		// this is the honest answer rather than a fallback to whatever
		// account happens to be lying around.
		return "", fmt.Errorf("%q is not a registered game", game)
	}
	name := strings.TrimSpace(p.Account)
	if name == "" {
		return "", fmt.Errorf("no account is bound for %q to spend from", game)
	}
	return name, nil
}

// gamingAccountNumber resolves the account one game may spend from.
//
// The game is named by the caller rather than read from anything global,
// because a spend is paid out of the account the game that asked for it was
// confined to. Resolving it globally would let one game's approval draw on
// another game's money.
func gamingAccountNumber(ctx context.Context, game string) (uint32, error) {
	name, err := gamingAccountFor(ReadGamingSettings(), game)
	if err != nil {
		return 0, err
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
	return 0, fmt.Errorf("the account bound for %q, %q, is not in this wallet", game, name)
}

func newSpendID() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
