// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

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

// DefaultGamingSettings is the disabled, unbound starting policy. It is
// deliberately not enabled and not bound to an account: nothing can be staked
// until the user picks the account games are confined to.
func DefaultGamingSettings() types.GamingSettings {
	return types.GamingSettings{
		Enabled:             false,
		Account:             "",
		PerTableCapAtoms:    gamingDefaultPerTableAtoms,
		PerDayCapAtoms:      gamingDefaultPerDayAtoms,
		ApprovalTimeoutSecs: gamingDefaultApprovalSecs,
		InstalledGames:      []string{},
	}
}

// newGamingToken mints a bearer token for a game.
func newGamingToken() (string, error) {
	var tok [16]byte
	if _, err := rand.Read(tok[:]); err != nil {
		return "", fmt.Errorf("generate game token: %w", err)
	}
	return hex.EncodeToString(tok[:]), nil
}

// carryGameTokens keeps a token for every game still installed and mints one
// for a game that has just been added.
//
// A token is not rotated by an unrelated policy edit, or saving the settings
// would cut off every running game. Removing a game drops its token, so
// uninstalling revokes rather than hides: a game left running finds that its
// identity no longer resolves.
func carryGameTokens(prev map[string]string, installed []string) (map[string]string, error) {
	out := make(map[string]string, len(installed))
	for _, id := range installed {
		if tok := prev[id]; tok != "" {
			out[id] = tok
			continue
		}
		tok, err := newGamingToken()
		if err != nil {
			return nil, err
		}
		out[id] = tok
	}
	return out, nil
}

// carryGameNames keeps a label for every game still registered, taking the one
// the operator just gave and otherwise the one already stored.
//
// Keyed off the registered list rather than edited in place, the same way
// tokens are, so a label cannot outlive the game it names. Unlike a token, a
// label is the operator's to write, so a named entry wins - but only for a game
// that is registered, because registration happens through the list and nowhere
// else.
func carryGameNames(prev, in map[string]string, registered []string) map[string]string {
	out := make(map[string]string, len(registered))
	for _, id := range registered {
		name := strings.TrimSpace(in[id])
		if name == "" {
			name = strings.TrimSpace(prev[id])
		}
		if name != "" {
			out[id] = name
		}
	}
	return out
}

// GamingGameToken is the token this host authenticates as a game with. It
// authorizes spending and must never leave this process.
func GamingGameToken(game string) (string, bool) {
	s := ReadGamingSettings()
	if !s.Enabled {
		return "", false
	}
	tok := s.GameTokens[game]
	return tok, tok != ""
}

// GamingGameForToken resolves a bearer token to the game it identifies.
//
// This is the only place a game's identity is established, and everything the
// host enforces hangs off what it returns.
//
// It resolves game tokens only; panel tokens are GamingUISessionFor's, and the
// two namespaces must not overlap.
func GamingGameForToken(token string) (string, bool) {
	return gamingGameForToken(ReadGamingSettings(), token)
}

// gamingGameForToken is the resolution itself, without the file read.
//
// Comparison is constant time so a caller cannot learn a token a character at a
// time, and a disabled section resolves nothing at all - the tunnel then
// answers as though it is not there.
func gamingGameForToken(s types.GamingSettings, token string) (string, bool) {
	if token == "" || !s.Enabled {
		return "", false
	}
	for game, tok := range s.GameTokens {
		if tok != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(token)) == 1 {
			return game, true
		}
	}
	return "", false
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
	if s.InstalledGames == nil {
		s.InstalledGames = []string{}
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
// Refusing rather than quietly clamping, unlike the unbound-account rule below:
// an operator who asked for the bridge and got it switched off with no reason
// would go looking for a bug. The account rule can clamp because the interface
// will not offer an account-less enable in the first place.
func normalizeGamingSettings(in, cur types.GamingSettings, appPasswordActive bool) (types.GamingSettings, error) {
	if in.Enabled && !appPasswordActive {
		return types.GamingSettings{}, ErrGamingNeedsAppPassword
	}

	registered, err := sanitizeInstalledGames(in.InstalledGames)
	if err != nil {
		return types.GamingSettings{}, err
	}

	out := types.GamingSettings{
		Enabled:             in.Enabled,
		Account:             strings.TrimSpace(in.Account),
		PerTableCapAtoms:    in.PerTableCapAtoms,
		PerDayCapAtoms:      in.PerDayCapAtoms,
		ApprovalTimeoutSecs: in.ApprovalTimeoutSecs,
		InstalledGames:      registered,
		GameNames:           carryGameNames(cur.GameNames, in.GameNames, registered),
		GameTokens:          map[string]string{},
	}

	// Carry tokens across for games that are still registered, and mint one
	// for a game that has just been added. Removing a game drops its token,
	// so removing actually revokes rather than merely hiding: a game that
	// kept running would find its identity no longer resolves.
	tokens, err := carryGameTokens(cur.GameTokens, out.InstalledGames)
	if err != nil {
		return types.GamingSettings{}, err
	}
	out.GameTokens = tokens

	if out.PerTableCapAtoms < 0 {
		out.PerTableCapAtoms = 0
	}
	if out.PerDayCapAtoms < 0 {
		out.PerDayCapAtoms = 0
	}
	// A day cap below the table cap would let a single buy-in exceed the
	// day's budget, so raise the day cap to match rather than silently
	// letting one table overshoot it.
	if out.PerDayCapAtoms < out.PerTableCapAtoms {
		out.PerDayCapAtoms = out.PerTableCapAtoms
	}
	if out.ApprovalTimeoutSecs < 10 {
		out.ApprovalTimeoutSecs = 10
	}
	if out.ApprovalTimeoutSecs > 600 {
		out.ApprovalTimeoutSecs = 600
	}
	// Enabling without an account bound would leave games nothing to spend
	// from while looking active, so refuse the combination.
	if out.Account == "" {
		out.Enabled = false
	}
	return out, nil
}

// WriteGamingSettings validates and persists the gaming policy, bumping Rev so
// a reader can notice the change.
//
// appPasswordActive is passed in by the caller, which is what keeps this
// package from depending on the auth package for a boolean.
func WriteGamingSettings(in types.GamingSettings, appPasswordActive bool) (types.GamingSettings, error) {
	out, err := normalizeGamingSettings(in, ReadGamingSettings(), appPasswordActive)
	if err != nil {
		return types.GamingSettings{}, err
	}

	if err := os.MkdirAll(GamingStateDir, 0o700); err != nil {
		return out, err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return out, err
	}
	tmp := gamingSettingsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return out, err
	}
	if err := os.Rename(tmp, gamingSettingsPath()); err != nil {
		return out, err
	}
	return out, nil
}

// ErrGamingGameNotRunning says no game is connected under that name.
var ErrGamingGameNotRunning = errors.New("game is not connected")

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
	out := make([]types.GamingGame, 0, len(s.InstalledGames))
	for _, id := range s.InstalledGames {
		out = append(out, types.GamingGame{
			ID:   id,
			Name: gamingDisplayName(s, id),
			// Connection state arrives with the bridge listener; until
			// then nothing is connected, which is the truthful answer.
			Ready: false,
		})
	}
	return out
}

// gamingDisplayName is what to call a game in the interface: the label the
// operator gave it, or the id, which is the only name the wire carries.
func gamingDisplayName(s types.GamingSettings, id string) string {
	if name := strings.TrimSpace(s.GameNames[id]); name != "" {
		return name
	}
	return id
}

// gamingTokenFor returns the token the host issued a game, which is the only
// thing that game will answer to.
func gamingTokenFor(id string) (string, error) {
	s := ReadGamingSettings()
	token := s.GameTokens[id]
	if strings.TrimSpace(token) == "" {
		return "", ErrGamingGameNotInstalled
	}
	return token, nil
}

// sanitizeInstalledGames folds the ids an operator gave into the form the wire
// uses, and refuses anything the wire could not carry.
//
// Folding and refusing are different acts. Blank entries, duplicates, case and
// surrounding space are tidying - nobody meant anything by them and nothing is
// lost. An id that is not a routing key is a mistake with a consequence:
// registered, it would route nothing at all, and dropped in silence it would
// leave an operator looking at a list that did not grow, with nothing to read
// and a typo they cannot see. So it comes back as an error naming the rule.
func sanitizeInstalledGames(ids []string) ([]string, error) {
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
