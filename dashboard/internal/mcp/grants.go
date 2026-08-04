// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Spend grants let a user authorize an identified agent to move funds WITHOUT
// ever giving the agent the wallet passphrase. The user enters the passphrase
// once in the dashboard; it is held here in memory only (never persisted, never
// sent to the agent) and used on the agent's behalf, bounded by account scope
// and amount caps. Grants are gone on restart by design.
//
// Authorization has two axes: a DCR spend budget (account scope + per-tx/daily
// caps + tripwire) for fund-moving tools, and a set of write scopes (see
// scopes.go) for non-fund write/action tools. The two combine: a fund move in a
// non-default channel (Lightning, dex.spend) requires both the scope and budget.

const grantWindow = 24 * time.Hour

// Denials an agent may see. They are deliberately actionable ("ask the user...")
// since the agent cannot self-serve a grant.
var (
	errNoGrant           = errors.New("no spend grant: ask the user to grant this agent spend access in the dashboard")
	errGrantExpired      = errors.New("spend grant has expired: ask the user to renew it")
	errAccountNotGranted = errors.New("this account is not covered by the agent's spend grant")
	errBadAmount         = errors.New("amount must be positive")
	errPerTxExceeded     = errors.New("amount exceeds the per-transaction cap of the spend grant")
	errAddrNotAllowed    = errors.New("recipient address is not on the spend grant's allowlist")
	errDailyExceeded     = errors.New("amount would exceed the spend grant's daily cap")
)

// scopeDenied is the denial returned when a grant exists but does not include
// the write scope a tool requires.
func scopeDenied(scope string) error {
	return fmt.Errorf("the agent's grant does not include the %q write scope: ask the user to enable it", scope)
}

// noScopeGrant is the no-grant denial for scope-gated tools. It names the
// missing write scope and avoids "spend" wording: most scoped actions (tor,
// timestamp, governance votes) move no funds, and errNoGrant's spend language
// reads wrong for them.
func noScopeGrant(scope string) error {
	return fmt.Errorf("no write grant: this tool needs the %q write scope: ask the user to grant it in the dashboard", scope)
}

// spendGrant is one agent's in-memory spend capability. Wallet sends and ticket
// purchases use the held passphrase within account scope and DCR caps. The
// writeScopes set enables other fund-moving/signing/write actions that have
// their own funding or auth: voting/staking sign with the held passphrase;
// Lightning spends from dcrlnd (counted against the daily cap); DEX trades via
// the unlocked DEX; Bison Relay writes move no funds.
type spendGrant struct {
	accounts    map[uint32]bool // wallet account numbers the agent may spend from
	perTxAtoms  int64           // literal per-transaction limit (0 permits nothing)
	dailyAtoms  int64           // literal daily limit (0 permits nothing)
	allowlist   map[string]bool // empty = any recipient allowed
	expiry      time.Time       // zero = never expires
	passphrase  []byte          // in memory only; zeroed on revoke/expiry
	writeScopes map[string]bool // granted write/action scopes (see scopes.go)

	spentAtoms  int64 // spent in the current rolling window (all DCR fund moves)
	windowStart time.Time
}

// GrantSpec is the user-provided definition of a spend grant.
type GrantSpec struct {
	Accounts    []uint32
	PerTxAtoms  int64
	DailyAtoms  int64
	Allowlist   []string
	Expiry      time.Time
	Passphrase  []byte
	WriteScopes []string
}

// GrantInfo is the dashboard-facing view of a grant. It never includes the
// passphrase. Amounts are atoms; the API layer converts to DCR for display.
type GrantInfo struct {
	Accounts    []uint32  `json:"accounts"`
	PerTxAtoms  int64     `json:"perTxAtoms"`
	DailyAtoms  int64     `json:"dailyAtoms"`
	Allowlist   []string  `json:"allowlist"`
	Expiry      time.Time `json:"expiry,omitempty"`
	SpentAtoms  int64     `json:"spentAtoms"`
	WindowStart time.Time `json:"windowStart"`
	WriteScopes []string  `json:"writeScopes"`
}

type grantStore struct {
	mu      sync.Mutex
	byAgent map[string]*spendGrant
}

func newGrantStore() *grantStore { return &grantStore{byAgent: map[string]*spendGrant{}} }

// set installs (or replaces) an agent's grant, copying the passphrase and
// zeroing any prior one.
func (s *grantStore) set(agentID string, spec GrantSpec, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Carry the current window across an edit. The daily allowance belongs to
	// the agent's spending, not to the grant document, so editing an unrelated
	// field must not hand back headroom already used. An explicit revoke drops
	// the grant and does start a fresh window.
	spent, windowStart := int64(0), now
	if old := s.byAgent[agentID]; old != nil {
		zero(old.passphrase)
		if now.Sub(old.windowStart) < grantWindow {
			spent, windowStart = old.spentAtoms, old.windowStart
		}
	}
	accounts := make(map[uint32]bool, len(spec.Accounts))
	for _, a := range spec.Accounts {
		accounts[a] = true
	}
	allow := make(map[string]bool, len(spec.Allowlist))
	for _, a := range spec.Allowlist {
		if a != "" {
			allow[a] = true
		}
	}
	scopes := make(map[string]bool, len(spec.WriteScopes))
	for _, sc := range spec.WriteScopes {
		if sc != "" {
			scopes[sc] = true
		}
	}
	s.byAgent[agentID] = &spendGrant{
		accounts:    accounts,
		perTxAtoms:  spec.PerTxAtoms,
		dailyAtoms:  spec.DailyAtoms,
		allowlist:   allow,
		expiry:      spec.Expiry,
		passphrase:  append([]byte(nil), spec.Passphrase...),
		writeScopes: scopes,
		spentAtoms:  spent,
		windowStart: windowStart,
	}
}

// revoke removes an agent's grant and zeroes its passphrase. Returns false if
// the agent had no grant.
func (s *grantStore) revoke(agentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.byAgent[agentID]
	if g == nil {
		return false
	}
	zero(g.passphrase)
	delete(s.byAgent, agentID)
	return true
}

// revokeAll clears every agent's grant, zeroing all held passphrases. Used by
// the freeze-all kill-switch.
func (s *grantStore) revokeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, g := range s.byAgent {
		zero(g.passphrase)
		delete(s.byAgent, id)
	}
}

func (s *grantStore) info(agentID string) (GrantInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.byAgent[agentID]
	if g == nil {
		return GrantInfo{}, false
	}
	return GrantInfo{
		Accounts:    sortedUint32(g.accounts),
		PerTxAtoms:  g.perTxAtoms,
		DailyAtoms:  g.dailyAtoms,
		Allowlist:   sortedStrings(g.allowlist),
		Expiry:      g.expiry,
		SpentAtoms:  g.spentAtoms,
		WindowStart: g.windowStart,
		WriteScopes: sortedStrings(g.writeScopes),
	}, true
}

// currentLocked returns the agent's live grant, removing it (and zeroing the
// passphrase) if it has expired. The caller must hold s.mu.
func (s *grantStore) currentLocked(agentID string, now time.Time) (*spendGrant, error) {
	g := s.byAgent[agentID]
	if g == nil {
		return nil, errNoGrant
	}
	if !g.expiry.IsZero() && !now.Before(g.expiry) {
		zero(g.passphrase)
		delete(s.byAgent, agentID)
		return nil, errGrantExpired
	}
	return g, nil
}

// reserveLocked enforces the per-transaction and daily caps and reserves the
// amount against the rolling window. The caller must hold s.mu.
func (g *spendGrant) reserveLocked(amountAtoms int64, now time.Time) error {
	if amountAtoms <= 0 {
		return errBadAmount
	}
	// Caps are literal hard limits: a 0 cap permits nothing. There is no
	// "unlimited" - to allow spending, the user sets a positive cap.
	if amountAtoms > g.perTxAtoms {
		return errPerTxExceeded
	}
	if now.Sub(g.windowStart) >= grantWindow {
		g.spentAtoms = 0
		g.windowStart = now
	}
	if g.spentAtoms+amountAtoms > g.dailyAtoms {
		return errDailyExceeded
	}
	g.spentAtoms += amountAtoms
	return nil
}

// authorize validates a proposed wallet send against the agent's grant, reserves
// the amount, and (when BR oversight is on) blocks for the operator's approval.
// On success it returns a private copy of the passphrase for immediate use; call
// refund if the spend subsequently fails.
func (s *grantStore) authorize(ctx context.Context, agentID string, account uint32, amountAtoms int64, toAddr string, now time.Time) ([]byte, error) {
	pass, err := s.reserveForSend(agentID, account, amountAtoms, toAddr, now)
	if err != nil {
		return nil, err
	}
	action := fmt.Sprintf("spend %s", dcrAmountStr(amountAtoms))
	if toAddr != "" {
		action = fmt.Sprintf("send %s to %s", dcrAmountStr(amountAtoms), toAddr)
	}
	if err := gateApproval(ctx, agentID, action); err != nil {
		s.refund(agentID, amountAtoms)
		zero(pass)
		return nil, err
	}
	return pass, nil
}

// reserveForSend performs the locked grant validation and reservation for a
// wallet send, returning a private copy of the passphrase.
func (s *grantStore) reserveForSend(agentID string, account uint32, amountAtoms int64, toAddr string, now time.Time) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.currentLocked(agentID, now)
	if err != nil {
		return nil, err
	}
	if !g.accounts[account] {
		return nil, errAccountNotGranted
	}
	// The allowlist constrains address sends only; non-send spends (e.g. ticket
	// purchases) pass an empty address and are not allowlist-checked.
	if toAddr != "" && len(g.allowlist) > 0 && !g.allowlist[toAddr] {
		return nil, errAddrNotAllowed
	}
	if err := g.reserveLocked(amountAtoms, now); err != nil {
		return nil, err
	}
	return append([]byte(nil), g.passphrase...), nil
}

// precheckAccount verifies a grant exists and covers the account, without
// reserving any amount. It lets account-spend tools reject early - before doing
// work like querying the chain for a ticket price - when there is no grant.
func (s *grantStore) precheckAccount(agentID string, account uint32, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.currentLocked(agentID, now)
	if err != nil {
		return err
	}
	if !g.accounts[account] {
		return errAccountNotGranted
	}
	return nil
}

// precheckGrant verifies a grant exists, without checking an account or scope.
// It lets a tool reject an ungranted agent before doing any work to resolve
// which account it would actually spend from.
func (s *grantStore) precheckGrant(agentID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.currentLocked(agentID, now)
	return err
}

// precheckScope verifies a grant exists and includes the scope, without
// reserving. It lets amount-derived spends (e.g. ln_pay) reject before doing
// work like decoding an invoice when access is not granted.
func (s *grantStore) precheckScope(agentID, scope string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.currentLocked(agentID, now)
	if err != nil {
		return err
	}
	if !g.writeScopes[scope] {
		return scopeDenied(scope)
	}
	return nil
}

// authorizeAction checks the grant includes the given write scope, for a
// non-fund, non-signing write (no amount, no passphrase).
func (s *grantStore) authorizeAction(agentID, scope string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.currentLocked(agentID, now)
	if err != nil {
		if errors.Is(err, errNoGrant) {
			return noScopeGrant(scope)
		}
		return err
	}
	if !g.writeScopes[scope] {
		return scopeDenied(scope)
	}
	return nil
}

// authorizeActionPass is authorizeAction for writes that sign with the held
// passphrase (governance voting, staking VSP-fee recovery). It returns a copy
// of the passphrase. Not amount-capped.
func (s *grantStore) authorizeActionPass(agentID, scope string, now time.Time) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.currentLocked(agentID, now)
	if err != nil {
		if errors.Is(err, errNoGrant) {
			return nil, noScopeGrant(scope)
		}
		return nil, err
	}
	if !g.writeScopes[scope] {
		return nil, scopeDenied(scope)
	}
	return append([]byte(nil), g.passphrase...), nil
}

// authorizeActionGated is authorizeAction plus the operator-approval gate, for
// writes that arm an autonomous spender (DEX bond auto-renewal, Lightning
// autopilot). No amount is reserved because none is known at call time, so the
// approval is the last human checkpoint before the armed component starts
// moving funds on its own.
func (s *grantStore) authorizeActionGated(ctx context.Context, agentID, scope, action string, now time.Time) error {
	// authorizeAction releases s.mu before returning; gateApproval must not be
	// reached holding it (a "freeze" reply revokes the grant, which takes s.mu).
	if err := s.authorizeAction(agentID, scope, now); err != nil {
		return err
	}
	return gateApproval(ctx, agentID, action)
}

// authorizeLightning checks the lightning scope, reserves amountAtoms against
// the (shared) daily cap, and (when BR oversight is on) blocks for the
// operator's approval. No passphrase is returned: dcrlnd is unlocked separately.
// Call refund if the payment then fails.
func (s *grantStore) authorizeLightning(ctx context.Context, agentID string, amountAtoms int64, now time.Time) error {
	if err := s.reserveLightning(agentID, amountAtoms, now); err != nil {
		return err
	}
	if err := gateApproval(ctx, agentID, fmt.Sprintf("make a Lightning payment of %s", dcrAmountStr(amountAtoms))); err != nil {
		s.refund(agentID, amountAtoms)
		return err
	}
	return nil
}

func (s *grantStore) reserveLightning(agentID string, amountAtoms int64, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.currentLocked(agentID, now)
	if err != nil {
		return err
	}
	if !g.writeScopes[scopeLightning] {
		return scopeDenied(scopeLightning)
	}
	return g.reserveLocked(amountAtoms, now)
}

// authorizeVSPFees checks the staking scope and both fee accounts, reserves a
// worst-case fee ceiling against the caps, and (when BR oversight is on) blocks
// for the operator's approval. The real cost of a VSP fee run is only knowable
// afterwards, so the caller reserves the ceiling here and refunds the unused
// part once the run settles. Returns a private copy of the passphrase.
func (s *grantStore) authorizeVSPFees(ctx context.Context, agentID string, account, changeAccount uint32, feeCeilingAtoms int64, action string, now time.Time) ([]byte, error) {
	pass, err := s.reserveVSPFees(agentID, account, changeAccount, feeCeilingAtoms, now)
	if err != nil {
		return nil, err
	}
	if err := gateApproval(ctx, agentID, action); err != nil {
		s.refund(agentID, feeCeilingAtoms)
		zero(pass)
		return nil, err
	}
	return pass, nil
}

// reserveVSPFees performs the locked validation and reservation for a VSP fee
// run, returning a private copy of the passphrase. A zero ceiling (nothing to
// pay for) still requires the scope and both accounts.
func (s *grantStore) reserveVSPFees(agentID string, account, changeAccount uint32, feeCeilingAtoms int64, now time.Time) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.currentLocked(agentID, now)
	if err != nil {
		if errors.Is(err, errNoGrant) {
			return nil, noScopeGrant(scopeStaking)
		}
		return nil, err
	}
	if !g.writeScopes[scopeStaking] {
		return nil, scopeDenied(scopeStaking)
	}
	// The fee leaves the fee account and the change lands in the change
	// account, so both must be covered by the grant.
	if !g.accounts[account] || !g.accounts[changeAccount] {
		return nil, errAccountNotGranted
	}
	if feeCeilingAtoms < 0 {
		return nil, errBadAmount
	}
	if feeCeilingAtoms > 0 {
		if err := g.reserveLocked(feeCeilingAtoms, now); err != nil {
			return nil, err
		}
	}
	return append([]byte(nil), g.passphrase...), nil
}

// authorizeSpendScoped checks the given fund scope and, when amountAtoms>0
// (DCR-denominated), reserves it against the shared daily cap. Non-DCR moves
// (other DEX assets) pass 0 and are scope-gated only - the DCR cap cannot bound
// a non-DCR amount. No wallet passphrase (the DEX and dcrlnd are unlocked
// separately). The action describes the spend in the operator's approval
// message, so it comes from the caller rather than being fixed here: the same
// reservation serves DEX moves and paid Bison Relay downloads. Call refund if a
// reserved spend then fails.
func (s *grantStore) authorizeSpendScoped(ctx context.Context, agentID, scope string, amountAtoms int64, action string, now time.Time) error {
	if err := s.reserveSpendScoped(agentID, scope, amountAtoms, now); err != nil {
		return err
	}
	if err := gateApproval(ctx, agentID, action); err != nil {
		if amountAtoms > 0 {
			s.refund(agentID, amountAtoms)
		}
		return err
	}
	return nil
}

func (s *grantStore) reserveSpendScoped(agentID, scope string, amountAtoms int64, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.currentLocked(agentID, now)
	if err != nil {
		if errors.Is(err, errNoGrant) {
			return noScopeGrant(scope)
		}
		return err
	}
	if !g.writeScopes[scope] {
		return scopeDenied(scope)
	}
	// Only a zero amount means "not DCR-denominated". A negative one is a
	// caller bug (an unsigned amount that overflowed int64) and must not slip
	// past the caps the way a zero legitimately does.
	if amountAtoms < 0 {
		return errBadAmount
	}
	if amountAtoms > 0 {
		return g.reserveLocked(amountAtoms, now)
	}
	return nil
}

// refund returns reserved spend headroom after a failed transaction. Only a
// positive amount is meaningful: refunding zero is a no-op and refunding a
// negative would add to the spent total and, via the clamp below, hand back the
// whole window's headroom.
func (s *grantStore) refund(agentID string, amountAtoms int64) {
	if amountAtoms <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if g := s.byAgent[agentID]; g != nil {
		g.spentAtoms -= amountAtoms
		if g.spentAtoms < 0 {
			g.spentAtoms = 0
		}
	}
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func sortedUint32(m map[uint32]bool) []uint32 {
	out := make([]uint32, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedStrings(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// grants is the process-wide in-memory grant store.
var grants = newGrantStore()

// SetSpendGrant installs (or replaces) the spend grant for an agent. The
// passphrase is copied into memory and never persisted.
func SetSpendGrant(agentID string, spec GrantSpec) { grants.set(agentID, spec, time.Now()) }

// RevokeSpendGrant clears an agent's spend grant, zeroing the passphrase.
func RevokeSpendGrant(agentID string) bool { return grants.revoke(agentID) }

// SpendGrantInfo returns the (passphrase-free) grant view for the dashboard.
func SpendGrantInfo(agentID string) (GrantInfo, bool) { return grants.info(agentID) }
