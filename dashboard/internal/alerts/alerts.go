// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package alerts is the dashboard's alert engine: a persisted ring of
// operator-facing entries fed by watchers in the services package and served
// over /api/alerts. Events are one-shot (read/unread); conditions are
// raise/resolve pairs debounced per code so a flapping daemon never spams
// history. The pill count is unread events plus active conditions.
package alerts

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"dcrpulse/internal/config"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

type Category string

const (
	CategoryNode       Category = "node"
	CategoryWallet     Category = "wallet"
	CategoryStaking    Category = "staking"
	CategoryLightning  Category = "lightning"
	CategoryDex        Category = "dex"
	CategoryBisonrelay Category = "bisonrelay"
	CategorySystem     Category = "system"
)

type Kind string

const (
	KindEvent     Kind = "event"
	KindCondition Kind = "condition"
)

var (
	ErrNotFound    = errors.New("alert not found")
	ErrBadSettings = errors.New("invalid alerts settings")
)

type catalogEntry struct {
	Category Category
	Kind     Kind
	Severity Severity
	Debounce time.Duration
	Title    string
}

// catalog is the single source of truth for every alert the engine can
// produce; producers pass codes only, so kind/severity/debounce/title
// cannot drift between watchers.
var catalog = map[string]catalogEntry{
	"dcrd_unreachable":        {CategoryNode, KindCondition, SeverityCritical, 5 * time.Minute, "Decred node unreachable"},
	"dcrd_no_peers":           {CategoryNode, KindCondition, SeverityCritical, 5 * time.Minute, "Decred node has no peers"},
	"chain_sync_stalled":      {CategoryNode, KindCondition, SeverityWarning, 0, "Chain sync stalled"},
	"wallet_tx_received":      {CategoryWallet, KindEvent, SeverityInfo, 0, "Payment received"},
	"ticket_purchased":        {CategoryStaking, KindEvent, SeverityInfo, 0, "Ticket purchased"},
	"ticket_voted":            {CategoryStaking, KindEvent, SeverityInfo, 0, "Ticket voted"},
	"ticket_expired":          {CategoryStaking, KindEvent, SeverityWarning, 0, "Ticket expired"},
	"ticket_missed":           {CategoryStaking, KindEvent, SeverityWarning, 0, "Ticket missed vote"},
	"ticket_revoked":          {CategoryStaking, KindEvent, SeverityWarning, 0, "Ticket revoked"},
	"ln_channel_force_closed": {CategoryLightning, KindEvent, SeverityCritical, 0, "Lightning channel force-closed"},
	"ln_no_peers":             {CategoryLightning, KindCondition, SeverityWarning, 5 * time.Minute, "Lightning node has no peers"},
	"dex_order_filled":        {CategoryDex, KindEvent, SeverityInfo, 0, "DEX order filled"},
	"dex_unreachable":         {CategoryDex, KindCondition, SeverityWarning, 5 * time.Minute, "DEX client unreachable"},
	"br_disconnected":         {CategoryBisonrelay, KindCondition, SeverityWarning, 5 * time.Minute, "Bison Relay disconnected"},
	"disk_high":               {CategorySystem, KindCondition, SeverityWarning, 0, "Disk space low"},
}

// Entry is one alert in the ring. For conditions, duration is derived
// client-side as ResolvedAt - CreatedAt (CreatedAt is the first observation,
// so the debounce window counts toward the outage).
type Entry struct {
	ID         string     `json:"id"`
	Kind       Kind       `json:"kind"`
	Category   Category   `json:"category"`
	Severity   Severity   `json:"severity"`
	Code       string     `json:"code"`
	Scope      string     `json:"scope,omitempty"`
	Title      string     `json:"title"`
	Detail     string     `json:"detail,omitempty"`
	DedupeKey  string     `json:"dedupeKey,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	Read       bool       `json:"read"`
	Active     bool       `json:"active,omitempty"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
}

type SummaryData struct {
	Unread   int      `json:"unread"`
	Active   int      `json:"active"`
	Severity Severity `json:"severity"`
}

type SettingsData struct {
	Schema   int        `json:"schema"`
	Disabled []Category `json:"disabled"`
}

const (
	ringCap        = 500
	settingsSchema = 1
	maxDisabled    = 16
)

type condState struct {
	firstRaise time.Time
	entryID    string
}

type engine struct {
	mu       sync.Mutex
	entries  []*Entry
	conds    map[string]*condState
	disabled map[Category]bool
	notify   func()
	seq      int
}

var (
	engOnce sync.Once
	eng     *engine
)

func get() *engine {
	engOnce.Do(func() {
		eng = &engine{conds: map[string]*condState{}, disabled: map[Category]bool{}}
		entries, err := loadEntries(config.AlertsPath())
		if err != nil {
			// Operate on an empty in-memory ring rather than disabling
			// alerting; the next successful save rewrites the file.
			log.Printf("alerts: load: %v", err)
		}
		eng.entries = entries
		eng.loadDisabledLocked()
		eng.adoptActiveLocked()
	})
	return eng
}

// loadDisabledLocked seeds the category opt-outs from the global config.
// Absent, unreadable, or schema-mismatched settings mean all enabled.
func (e *engine) loadDisabledLocked() {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		log.Printf("alerts: load settings: %v", err)
		return
	}
	var s SettingsData
	if ok, err := gc.Get(config.KeyAlertsSettings, &s); !ok || err != nil || s.Schema != settingsSchema {
		return
	}
	for _, c := range s.Disabled {
		if knownCategory(c) {
			e.disabled[c] = true
		}
	}
}

// adoptActiveLocked re-adopts persisted active conditions so their original
// raise time spans restarts; entries of now-disabled categories resolve
// immediately. Pending (pre-debounce) state intentionally resets on restart.
func (e *engine) adoptActiveLocked() {
	changed := false
	now := time.Now().UTC()
	for _, en := range e.entries {
		if en.Kind != KindCondition || !en.Active {
			continue
		}
		if e.disabled[en.Category] {
			t := now
			en.Active = false
			en.ResolvedAt = &t
			changed = true
			continue
		}
		e.conds[condKey(en.Code, en.Scope)] = &condState{firstRaise: en.CreatedAt, entryID: en.ID}
	}
	if changed {
		e.saveLocked()
	}
}

// SetNotify registers the fanout hook fired after any change that affects the
// summary. Wired in main to the Bison Relay event bus (the engine cannot
// import services without a cycle).
func SetNotify(fn func()) {
	e := get()
	e.mu.Lock()
	e.notify = fn
	e.mu.Unlock()
}

func (e *engine) fire() {
	e.mu.Lock()
	fn := e.notify
	e.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Emit records a one-shot event. Dropped when the category is disabled or an
// entry with the same code+dedupeKey is already in the ring (kills reorg
// re-confirmations and restart replays without per-watcher seen-sets).
func Emit(code, detail, dedupeKey string) {
	e := get()
	ce, ok := catalog[code]
	if !ok || ce.Kind != KindEvent {
		log.Printf("alerts: emit unknown event code %q", code)
		return
	}
	e.mu.Lock()
	if e.disabled[ce.Category] {
		e.mu.Unlock()
		return
	}
	if dedupeKey != "" {
		for _, en := range e.entries {
			if en.Code == code && en.DedupeKey == dedupeKey {
				e.mu.Unlock()
				return
			}
		}
	}
	en := e.newEntryLocked(code, ce, time.Now().UTC())
	en.Detail = detail
	en.DedupeKey = dedupeKey
	e.appendLocked(en)
	e.saveLocked()
	e.mu.Unlock()
	log.Printf("alerts: emit %s", code)
	e.fire()
}

// Raise observes a condition as currently true. The first observation is
// pending and silent; an observation arriving past the code's debounce
// promotes it to an active entry. Producers tick every 20-60s, so promotion
// needs no timers.
func Raise(code, scope, detail string) {
	raise(code, scope, detail, "")
}

// RaiseSeverity is Raise with a severity override for conditions that
// escalate (disk 90% warning -> 95% critical). Severity only moves upward
// while active.
func RaiseSeverity(code, scope, detail string, sev Severity) {
	raise(code, scope, detail, sev)
}

func raise(code, scope, detail string, sevOverride Severity) {
	e := get()
	ce, ok := catalog[code]
	if !ok || ce.Kind != KindCondition {
		log.Printf("alerts: raise unknown condition code %q", code)
		return
	}
	sev := ce.Severity
	if sevOverride != "" {
		sev = sevOverride
	}
	now := time.Now().UTC()
	key := condKey(code, scope)
	changed := false
	e.mu.Lock()
	if e.disabled[ce.Category] {
		e.mu.Unlock()
		return
	}
	cs, ok := e.conds[key]
	if !ok {
		// Zero-debounce codes promote on this same call (slow probes like the
		// hourly disk check must not wait a second observation).
		cs = &condState{firstRaise: now}
		e.conds[key] = cs
	}
	switch {
	case cs.entryID == "":
		if now.Sub(cs.firstRaise) >= ce.Debounce {
			en := e.newEntryLocked(code, ce, cs.firstRaise)
			en.Severity = sev
			en.Scope = scope
			en.Detail = detail
			en.Active = true
			cs.entryID = en.ID
			e.appendLocked(en)
			e.saveLocked()
			changed = true
		}
	default:
		if en := e.entryByIDLocked(cs.entryID); en != nil && sevRank(sev) > sevRank(en.Severity) {
			en.Severity = sev
			en.Detail = detail
			e.saveLocked()
			changed = true
		}
	}
	e.mu.Unlock()
	if changed {
		log.Printf("alerts: raise %s", codeScope(code, scope))
		e.fire()
	}
}

// Resolve observes a condition as no longer true: a pending one clears
// silently, an active one keeps its entry as history with the duration.
func Resolve(code, scope string) {
	e := get()
	key := condKey(code, scope)
	e.mu.Lock()
	cs, ok := e.conds[key]
	if !ok {
		e.mu.Unlock()
		return
	}
	delete(e.conds, key)
	if cs.entryID == "" {
		e.mu.Unlock()
		return
	}
	var dur time.Duration
	if en := e.entryByIDLocked(cs.entryID); en != nil {
		now := time.Now().UTC()
		en.Active = false
		en.ResolvedAt = &now
		dur = now.Sub(en.CreatedAt)
	}
	e.saveLocked()
	e.mu.Unlock()
	log.Printf("alerts: resolve %s (%s)", codeScope(code, scope), dur.Round(time.Second))
	e.fire()
}

// ResolveCategories resolves every pending and active condition in the given
// categories. Called on wallet switch, where per-wallet daemon conditions no
// longer apply.
func ResolveCategories(cats ...Category) {
	e := get()
	in := map[Category]bool{}
	for _, c := range cats {
		in[c] = true
	}
	changed := false
	e.mu.Lock()
	now := time.Now().UTC()
	for key, cs := range e.conds {
		code, _, _ := splitCondKey(key)
		if !in[catalog[code].Category] {
			continue
		}
		delete(e.conds, key)
		if cs.entryID == "" {
			continue
		}
		if en := e.entryByIDLocked(cs.entryID); en != nil {
			t := now
			en.Active = false
			en.ResolvedAt = &t
			changed = true
		}
	}
	if changed {
		e.saveLocked()
	}
	e.mu.Unlock()
	if changed {
		e.fire()
	}
}

// Summary is the pill payload: unread events plus active conditions, with the
// highest severity among them. Served from memory, zero RPC and zero disk IO.
func Summary() SummaryData {
	e := get()
	e.mu.Lock()
	defer e.mu.Unlock()
	var s SummaryData
	best := 0
	for _, en := range e.entries {
		counted := false
		if en.Kind == KindEvent && !en.Read {
			s.Unread++
			counted = true
		}
		if en.Kind == KindCondition && en.Active {
			s.Active++
			counted = true
		}
		if counted && sevRank(en.Severity) > best {
			best = sevRank(en.Severity)
			s.Severity = en.Severity
		}
	}
	return s
}

// List returns copies of every entry, newest first.
func List() []Entry {
	e := get()
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Entry, 0, len(e.entries))
	for i := len(e.entries) - 1; i >= 0; i-- {
		out = append(out, *e.entries[i])
	}
	return out
}

// MarkRead marks one entry read. Reading an active condition is allowed but
// never changes the pill count (active conditions count until resolved).
func MarkRead(id string) error {
	e := get()
	e.mu.Lock()
	en := e.entryByIDLocked(id)
	if en == nil {
		e.mu.Unlock()
		return ErrNotFound
	}
	if en.Read {
		e.mu.Unlock()
		return nil
	}
	en.Read = true
	e.saveLocked()
	e.mu.Unlock()
	e.fire()
	return nil
}

func MarkAllRead() {
	e := get()
	changed := false
	e.mu.Lock()
	for _, en := range e.entries {
		if !en.Read {
			en.Read = true
			changed = true
		}
	}
	if changed {
		e.saveLocked()
	}
	e.mu.Unlock()
	if changed {
		e.fire()
	}
}

// Settings returns the persisted per-category opt-outs, defaults-filled.
func Settings() SettingsData {
	e := get()
	e.mu.Lock()
	defer e.mu.Unlock()
	out := SettingsData{Schema: settingsSchema, Disabled: []Category{}}
	for c := range e.disabled {
		out.Disabled = append(out.Disabled, c)
	}
	sort.Slice(out.Disabled, func(i, j int) bool { return out.Disabled[i] < out.Disabled[j] })
	return out
}

// UpdateSettings validates, persists, and applies the opt-outs. Disabling a
// category resolves its active conditions and clears its pendings; already
// recorded events stay until read.
func UpdateSettings(s SettingsData) error {
	if s.Schema != settingsSchema || len(s.Disabled) > maxDisabled {
		return ErrBadSettings
	}
	for _, c := range s.Disabled {
		if !knownCategory(c) {
			return ErrBadSettings
		}
	}
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return err
	}
	if err := gc.Set(config.KeyAlertsSettings, s); err != nil {
		return err
	}
	if err := gc.Save(); err != nil {
		return err
	}
	e := get()
	e.mu.Lock()
	e.disabled = map[Category]bool{}
	for _, c := range s.Disabled {
		e.disabled[c] = true
	}
	changed := false
	now := time.Now().UTC()
	for key, cs := range e.conds {
		code, _, _ := splitCondKey(key)
		if !e.disabled[catalog[code].Category] {
			continue
		}
		delete(e.conds, key)
		if cs.entryID == "" {
			continue
		}
		if en := e.entryByIDLocked(cs.entryID); en != nil {
			t := now
			en.Active = false
			en.ResolvedAt = &t
			changed = true
		}
	}
	if changed {
		e.saveLocked()
	}
	e.mu.Unlock()
	if changed {
		e.fire()
	}
	return nil
}

func (e *engine) newEntryLocked(code string, ce catalogEntry, createdAt time.Time) *Entry {
	e.seq++
	return &Entry{
		ID:        fmt.Sprintf("a%d-%d", time.Now().UnixMilli(), e.seq),
		Kind:      ce.Kind,
		Category:  ce.Category,
		Severity:  ce.Severity,
		Code:      code,
		Title:     ce.Title,
		CreatedAt: createdAt,
	}
}

func (e *engine) appendLocked(en *Entry) {
	e.entries = append(e.entries, en)
	if len(e.entries) <= ringCap {
		return
	}
	// Trim oldest first but never an active condition: it must stay
	// addressable for Resolve and the pill count.
	over := len(e.entries) - ringCap
	kept := e.entries[:0]
	for _, x := range e.entries {
		if over > 0 && !(x.Kind == KindCondition && x.Active) {
			over--
			continue
		}
		kept = append(kept, x)
	}
	e.entries = kept
}

func (e *engine) entryByIDLocked(id string) *Entry {
	for i := len(e.entries) - 1; i >= 0; i-- {
		if e.entries[i].ID == id {
			return e.entries[i]
		}
	}
	return nil
}

func condKey(code, scope string) string { return code + "|" + scope }

func splitCondKey(key string) (code, scope string, ok bool) {
	for i := 0; i < len(key); i++ {
		if key[i] == '|' {
			return key[:i], key[i+1:], true
		}
	}
	return key, "", false
}

func codeScope(code, scope string) string {
	if scope == "" {
		return code
	}
	return code + " " + scope
}

func sevRank(s Severity) int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	}
	return 0
}

func knownCategory(c Category) bool {
	switch c {
	case CategoryNode, CategoryWallet, CategoryStaking, CategoryLightning,
		CategoryDex, CategoryBisonrelay, CategorySystem:
		return true
	}
	return false
}
