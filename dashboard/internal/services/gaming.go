// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"dcrpulse/internal/config"
	"dcrpulse/internal/types"
)

const (
	// gamingDefaultPerTableAtoms is one DCR, matching the per-player escrow
	// ceiling the poker referee enforces.
	gamingDefaultPerTableAtoms = 100_000_000

	// gamingDefaultPerDayAtoms is five DCR.
	gamingDefaultPerDayAtoms = 500_000_000

	gamingDefaultApprovalSecs = 120
)

// gamingGameIDRE is the routing key a game id has to be.
//
// It is the wire's rule rather than this build's opinion. The id becomes the
// `game=` key in the envelope and the host part of a gaming:// invitation, so an
// id this host accepts but the wire cannot carry is a game whose frames are
// dropped forever with nothing anywhere to say why. It is the same expression
// the invite chip in this repo already parses with, and the same one the game
// on the other side writes with.
var gamingGameIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ErrGamingBadGameID is an id the wire could not carry.
var ErrGamingBadGameID = errors.New(
	"a game id is 1 to 32 characters of lowercase letters, digits, - or _, starting with a letter or digit")

// DefaultGamingSettings is the off, empty starting state: nothing registered
// and nothing carried.
func DefaultGamingSettings() types.GamingSettings {
	return types.GamingSettings{
		Enabled:         false,
		RegisteredGames: []string{},
		Policies:        map[string]types.GamePolicy{},
	}
}

// defaultGamePolicy is what a game is trusted with the moment it is registered:
// caps that let it play, and no account, so it can do nothing at all until the
// operator says which money is at stake.
func defaultGamePolicy() types.GamePolicy {
	return types.GamePolicy{
		Account:             "",
		PerTableCapAtoms:    gamingDefaultPerTableAtoms,
		PerDayCapAtoms:      gamingDefaultPerDayAtoms,
		ApprovalTimeoutSecs: gamingDefaultApprovalSecs,
	}
}

// normalizeGamePolicy clamps one game's policy into the range it may hold.
func normalizeGamePolicy(p types.GamePolicy) types.GamePolicy {
	p.Name = strings.TrimSpace(p.Name)
	p.Account = strings.TrimSpace(p.Account)
	if p.PerTableCapAtoms < 0 {
		p.PerTableCapAtoms = 0
	}
	if p.PerDayCapAtoms < 0 {
		p.PerDayCapAtoms = 0
	}
	// A day cap below the table cap would let a single buy-in exceed the
	// day's budget, so raise the day cap to match rather than silently
	// letting one table overshoot it.
	if p.PerDayCapAtoms < p.PerTableCapAtoms {
		p.PerDayCapAtoms = p.PerTableCapAtoms
	}
	if p.ApprovalTimeoutSecs < 10 {
		p.ApprovalTimeoutSecs = 10
	}
	if p.ApprovalTimeoutSecs > 600 {
		p.ApprovalTimeoutSecs = 600
	}
	return p
}

// carryGamePolicies keeps a policy for every game still registered, takes the
// operator's edits for the ones they named, and mints defaults for a game just
// added.
//
// The shape follows carryGameCredentials for the same reason: the map is rebuilt
// from the registered list rather than edited in place, so a policy cannot
// outlive the game it belongs to. What differs is that a policy is the
// operator's to write and a credential is not, so a named entry wins over the stored
// one - but only for a game that is registered. A policy for anything else is
// dropped: registration happens through the list and nowhere else, or a write
// could leave a funding rule for a game that routes nothing, which nobody would
// see until money moved.
//
// An unnamed game keeps its stored policy. Saving one game's account must not
// reset another's caps to the defaults, the same way saving the section must
// not rotate a connected game's credential.
func carryGamePolicies(prev, in map[string]types.GamePolicy, registered []string) map[string]types.GamePolicy {
	out := make(map[string]types.GamePolicy, len(registered))
	for _, id := range registered {
		switch p, named := in[id]; {
		case named:
			out[id] = normalizeGamePolicy(p)
		default:
			if stored, ok := prev[id]; ok {
				out[id] = normalizeGamePolicy(stored)
				continue
			}
			out[id] = defaultGamePolicy()
		}
	}
	return out
}

// carryGameCredentials keeps the credential of every game still registered.
//
// Carry only: unlike the tokens this replaced, a credential is never minted
// here. Issuing one is an explicit act the operator takes and sees the result
// of once, because the private key goes to them and nowhere else - minting
// silently on registration would produce a credential nobody was ever shown.
//
// A credential is not rotated by an unrelated policy edit, or saving the
// settings would cut off every connected game. Removing a game drops its
// credential, so unregistering revokes rather than hides: a game left running
// finds that its identity no longer resolves.
func carryGameCredentials(prev map[string]types.GameCredential, registered []string) map[string]types.GameCredential {
	out := make(map[string]types.GameCredential, len(registered))
	for _, id := range registered {
		if c, ok := prev[id]; ok {
			out[id] = c
		}
	}
	return out
}

// GamingStateDir is where the gaming section keeps its two files: the policy
// saying which account games may spend from and under what caps, and the log of
// every spend one has asked for.
//
// It is a value rather than the constant it starts as, and that is what makes
// the rules below checkable. A path fixed at compile time is a path no test can
// write, so every rule that had to read one - which game a credential belongs
// to, what it may spend, what it has spent today - could only be exercised by
// re-implementing it beside the real thing and asserting the copy. There was
// one of those here and it was never once capable of failing. Production leaves
// this alone; the bridge's own tests point it at a directory they may write.
var GamingStateDir = config.StackControlDir()

// gamingSettingsPath is the policy file. Moving it is not a rename: it holds
// the identity registered for each game, so a bridge that looks somewhere new
// finds no registrations and every game it knew becomes a stranger.
func gamingSettingsPath() string { return filepath.Join(GamingStateDir, "gaming.json") }

// gamingSpendLogPath is the audit trail a person reads, and the record the
// daily cap is counted from.
func gamingSpendLogPath() string { return filepath.Join(GamingStateDir, "gaming-spends.json") }

// ReadGamingSettings returns the stored gaming policy, falling back to the
// disabled default when the file is absent or unreadable. Read failures are
// deliberately not surfaced as an enabled policy.
func ReadGamingSettings() types.GamingSettings {
	s := DefaultGamingSettings()
	data, err := os.ReadFile(gamingSettingsPath())
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return DefaultGamingSettings()
	}
	if s.RegisteredGames == nil {
		s.RegisteredGames = []string{}
	}
	if s.Policies == nil {
		s.Policies = map[string]types.GamePolicy{}
	}
	return s
}

// ErrGamingNeedsAppPassword is why the bridge will not switch on.
var ErrGamingNeedsAppPassword = errors.New(
	"set and enable an App Password before turning the gaming bridge on")

// normalizeGamingSettings is the policy a write is allowed to produce, given
// what is already stored and whether the App Password is actually protecting
// the dashboard right now.
//
// appPasswordActive is an argument rather than a call into the auth package,
// because the rule is "the bridge does not run without a human gate" and that
// is a statement about a boolean, not about where the boolean came from. It is
// also what lets the whole truth table be written out in a test.
//
// It refuses rather than quietly switching the bridge off: an operator who
// asked for it and got nothing, with no reason given, would go looking for a
// bug in the wrong place.
func normalizeGamingSettings(in, cur types.GamingSettings, appPasswordActive bool) (types.GamingSettings, error) {
	if in.Enabled && !appPasswordActive {
		return types.GamingSettings{}, ErrGamingNeedsAppPassword
	}

	registered, err := sanitizeRegisteredGames(in.RegisteredGames)
	if err != nil {
		return types.GamingSettings{}, err
	}

	out := types.GamingSettings{
		Enabled:         in.Enabled,
		RegisteredGames: registered,
		Policies:        carryGamePolicies(cur.Policies, in.Policies, registered),
	}

	// Carry credentials across for games that are still registered. Removing
	// a game drops its credential, so removing actually revokes rather than
	// merely hiding: a game that kept running would find its identity no
	// longer resolves.
	out.GameCredentials = carryGameCredentials(cur.GameCredentials, out.RegisteredGames)

	// Nothing here forces the bridge off for want of an account. Routing
	// frames costs nothing and needs no money, so a bridge with no game
	// funded is a bridge that carries traffic and refuses every spend - and
	// the refusal names the game, which is the thing the operator can fix.
	return out, nil
}

// WriteGamingSettings validates and persists the gaming state.
//
// appPasswordActive is passed in by the caller, which is what keeps this
// package from depending on the auth package for a boolean.
func WriteGamingSettings(in types.GamingSettings, appPasswordActive bool) (types.GamingSettings, error) {
	gamingSettingsMu.Lock()
	defer gamingSettingsMu.Unlock()

	out, err := normalizeGamingSettings(in, ReadGamingSettings(), appPasswordActive)
	if err != nil {
		return types.GamingSettings{}, err
	}
	if err := writeGamingSettingsLocked(out); err != nil {
		return out, err
	}
	return out, nil
}

// gamingSettingsMu serialises the read-modify-write of gaming.json.
//
// There are two writers now: saving the section, and issuing or revoking a
// credential. Both read the whole file, change part of it and write it back, so
// without this one could land between the other's read and write and lose a
// registration or a credential.
var gamingSettingsMu sync.Mutex

// writeGamingSettingsLocked persists settings that have already been normalized.
func writeGamingSettingsLocked(out types.GamingSettings) error {
	if err := os.MkdirAll(GamingStateDir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	tmp := gamingSettingsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, gamingSettingsPath())
}

// GamingGames reports every game the operator registered, with the name they
// gave it and whether it is connected right now.
//
// There is no catalogue behind this. What an installation can route is whatever
// the operator registered, because routing on a key is the whole reason a game
// this host has never heard of can exist at all.
//
// Registered and connected are separate answers on purpose. A game runs on a
// machine of the person's choosing, so it can be registered here and simply not
// running, and reporting it as ready would send a player to a table nothing is
// listening on.
func GamingGames() []types.GamingGame {
	s := ReadGamingSettings()
	out := make([]types.GamingGame, 0, len(s.RegisteredGames))
	for _, id := range s.RegisteredGames {
		out = append(out, types.GamingGame{
			ID:    id,
			Name:  gamingDisplayName(s, id),
			Ready: gamingGameConnected(id),
		})
	}
	return out
}

// gamingDisplayName is what to call a game in the interface: the label the
// operator gave it, or the id, which is the only name the wire carries.
func gamingDisplayName(s types.GamingSettings, id string) string {
	if name := strings.TrimSpace(s.Policies[id].Name); name != "" {
		return name
	}
	return id
}

// sanitizeRegisteredGames folds the ids an operator gave into the form the wire
// uses, and refuses anything the wire could not carry.
//
// Folding and refusing are different acts. Blank entries, duplicates, case and
// surrounding space are tidying - nobody meant anything by them and nothing is
// lost. An id that is not a routing key is a mistake with a consequence:
// registered, it would route nothing at all, and dropped in silence it would
// leave an operator looking at a list that did not grow, with nothing to read
// and a typo they cannot see. So it comes back as an error naming the rule.
func sanitizeRegisteredGames(ids []string) ([]string, error) {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" || seen[id] {
			continue
		}
		if !gamingGameIDRE.MatchString(id) {
			return nil, fmt.Errorf("%w: %q is not one", ErrGamingBadGameID, id)
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}
