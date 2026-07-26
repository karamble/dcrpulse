// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"encoding/json"
	"os"
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

	gamingDefaultMaxOpenTables = 1
	gamingDefaultApprovalSecs  = 120

	gamingModeApproval = "approval"
	gamingModeAutopay  = "autopay"
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
		Mode:                gamingModeApproval,
		PerTableCapAtoms:    gamingDefaultPerTableAtoms,
		PerDayCapAtoms:      gamingDefaultPerDayAtoms,
		MaxOpenTables:       gamingDefaultMaxOpenTables,
		ApprovalTimeoutSecs: gamingDefaultApprovalSecs,
		InstalledGames:      []string{},
	}
}

// ReadGamingSettings returns the stored gaming policy, falling back to the
// disabled default when the file is absent or unreadable. Read failures are
// deliberately not surfaced as an enabled policy.
func ReadGamingSettings() types.GamingSettings {
	s := DefaultGamingSettings()
	data, err := os.ReadFile(config.GamingSettingsPath())
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

// WriteGamingSettings validates and persists the gaming policy, bumping Rev so
// a supervisor can notice the change.
func WriteGamingSettings(in types.GamingSettings) (types.GamingSettings, error) {
	cur := ReadGamingSettings()

	out := types.GamingSettings{
		Enabled:             in.Enabled,
		Account:             strings.TrimSpace(in.Account),
		Mode:                in.Mode,
		PerTableCapAtoms:    in.PerTableCapAtoms,
		PerDayCapAtoms:      in.PerDayCapAtoms,
		MaxOpenTables:       in.MaxOpenTables,
		ApprovalTimeoutSecs: in.ApprovalTimeoutSecs,
		InstalledGames:      sanitizeInstalledGames(in.InstalledGames),
		Rev:                 cur.Rev + 1,
	}

	if out.Mode != gamingModeApproval && out.Mode != gamingModeAutopay {
		out.Mode = gamingModeApproval
	}
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
	if out.MaxOpenTables < 1 {
		out.MaxOpenTables = 1
	}
	if out.MaxOpenTables > 10 {
		out.MaxOpenTables = 10
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

	if err := os.MkdirAll(config.StackControlDir(), 0o700); err != nil {
		return out, err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return out, err
	}
	tmp := config.GamingSettingsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return out, err
	}
	if err := os.Rename(tmp, config.GamingSettingsPath()); err != nil {
		return out, err
	}
	return out, nil
}

// GamingCatalogue reports every known game, marked with whether the user
// installed it. Ready stays false until a game's backend actually exists;
// nothing is wired up yet.
func GamingCatalogue() []types.GamingGame {
	installed := make(map[string]bool)
	for _, id := range ReadGamingSettings().InstalledGames {
		installed[id] = true
	}
	out := make([]types.GamingGame, 0, len(gamingCatalogue))
	for _, g := range gamingCatalogue {
		g.Installed = installed[g.ID]
		g.Ready = false
		out = append(out, g)
	}
	return out
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
