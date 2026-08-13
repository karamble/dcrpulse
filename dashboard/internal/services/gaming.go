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

// gamingCatalogue is every game dcrpulse knows how to route. The id doubles as
// the `game=` key in the Bison Relay wire envelope, so adding an entry here is
// what lets an installation recognise that game's traffic at all; anything else
// is ignored rather than surfaced.
var gamingCatalogue = []types.GamingGame{
	{
		ID:              "poker",
		Name:            "Poker",
		Description:     "Self-custodial Decred poker. Stakes are held in per-player escrow that only the whole table can settle, and are refundable by you alone after a timeout.",
		ProtocolVersion: 1,
	},
}

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

	out := types.GamingSettings{
		Enabled:             in.Enabled,
		Account:             strings.TrimSpace(in.Account),
		PerTableCapAtoms:    in.PerTableCapAtoms,
		PerDayCapAtoms:      in.PerDayCapAtoms,
		ApprovalTimeoutSecs: in.ApprovalTimeoutSecs,
		InstalledGames:      sanitizeInstalledGames(in.InstalledGames),
		GameTokens:          map[string]string{},
	}

	// Carry tokens across for games that are still installed, and mint one
	// for a game that has just been added. Removing a game drops its token,
	// so uninstalling actually revokes rather than merely hiding: a game
	// that kept running would find its identity no longer resolves.
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

// GamingCatalogue reports every known game, marked with whether the user
// registered it and whether it is connected right now.
//
// Registered and connected are separate answers on purpose. A game runs on a
// machine of the person's choosing, so it can be registered here and simply not
// running, and reporting it as ready would send a player to a table nothing is
// listening on.
func GamingCatalogue() []types.GamingGame {
	installed := make(map[string]bool)
	for _, id := range ReadGamingSettings().InstalledGames {
		installed[id] = true
	}

	out := make([]types.GamingGame, 0, len(gamingCatalogue))
	for _, g := range gamingCatalogue {
		g.Installed = installed[g.ID]
		// Connection state arrives with the bridge listener; until then
		// nothing is connected, which is the truthful answer.
		g.Ready = false
		out = append(out, g)
	}
	return out
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

// sanitizeInstalledGames drops unknown ids and duplicates, so a policy can only
// ever name games this build knows how to route.
func sanitizeInstalledGames(ids []string) []string {
	known := make(map[string]bool, len(gamingCatalogue))
	for _, g := range gamingCatalogue {
		known[g.ID] = true
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || !known[id] || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
