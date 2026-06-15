// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// Spend grants let a user authorize an identified agent to move funds WITHOUT
// ever giving the agent the wallet passphrase. The user enters the passphrase
// once in the dashboard; it is held here in memory only (never persisted, never
// sent to the agent) and used on the agent's behalf, bounded by account scope
// and amount caps. Grants are gone on restart by design.

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

// spendGrant is one agent's in-memory spend capability.
type spendGrant struct {
	accounts   map[uint32]bool // wallet account numbers the agent may spend from
	perTxAtoms int64           // 0 = no per-transaction cap
	dailyAtoms int64           // 0 = no daily cap
	allowlist  map[string]bool // empty = any recipient allowed
	expiry     time.Time       // zero = never expires
	passphrase []byte          // in memory only; zeroed on revoke/expiry

	spentAtoms  int64 // spent in the current rolling window
	windowStart time.Time
}

// GrantSpec is the user-provided definition of a spend grant.
type GrantSpec struct {
	Accounts   []uint32
	PerTxAtoms int64
	DailyAtoms int64
	Allowlist  []string
	Expiry     time.Time
	Passphrase []byte
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
	if old := s.byAgent[agentID]; old != nil {
		zero(old.passphrase)
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
	s.byAgent[agentID] = &spendGrant{
		accounts:    accounts,
		perTxAtoms:  spec.PerTxAtoms,
		dailyAtoms:  spec.DailyAtoms,
		allowlist:   allow,
		expiry:      spec.Expiry,
		passphrase:  append([]byte(nil), spec.Passphrase...),
		windowStart: now,
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
	}, true
}

// authorize validates a proposed spend against the agent's grant. On success it
// reserves the amount against the daily cap and returns a private copy of the
// passphrase for immediate use; call refund if the spend subsequently fails.
func (s *grantStore) authorize(agentID string, account uint32, amountAtoms int64, toAddr string, now time.Time) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.byAgent[agentID]
	if g == nil {
		return nil, errNoGrant
	}
	if !g.expiry.IsZero() && !now.Before(g.expiry) {
		zero(g.passphrase)
		delete(s.byAgent, agentID)
		return nil, errGrantExpired
	}
	if amountAtoms <= 0 {
		return nil, errBadAmount
	}
	if !g.accounts[account] {
		return nil, errAccountNotGranted
	}
	if g.perTxAtoms > 0 && amountAtoms > g.perTxAtoms {
		return nil, errPerTxExceeded
	}
	if len(g.allowlist) > 0 && !g.allowlist[toAddr] {
		return nil, errAddrNotAllowed
	}
	if now.Sub(g.windowStart) >= grantWindow {
		g.spentAtoms = 0
		g.windowStart = now
	}
	if g.dailyAtoms > 0 && g.spentAtoms+amountAtoms > g.dailyAtoms {
		return nil, errDailyExceeded
	}
	g.spentAtoms += amountAtoms
	return append([]byte(nil), g.passphrase...), nil
}

// refund returns reserved spend headroom after a failed transaction.
func (s *grantStore) refund(agentID string, amountAtoms int64) {
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
