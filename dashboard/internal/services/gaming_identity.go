// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// A game's identity is the one thing in this stack that only the game holds.
//
// Everything else a game touches is the host's: the wallet it asks to spend
// from, the Bison Relay identity it speaks through, the chain it reads. Its own
// keys are different. They are derived from a seed the game generates on first
// run and keeps in its own data volume, and nothing else has a copy - not the
// wallet, not the dashboard, not a backup of either.
//
// That is right for a component nobody trusts, and it has a cost the user pays
// silently: remove the volume and the seed is gone, which makes the game's
// fidelity bond permanently unspendable and any stake it has in escrow
// unrefundable, because both are locked to keys only that seed derives. So the
// host offers to carry a copy out to the person, once, before that happens.
type GamingBackup struct {
	Game string `json:"game"`
	// SeedHex is the secret itself. It is not stored here, logged here, or
	// written anywhere by the dashboard: it is read from the game and
	// passed straight to whoever asked.
	SeedHex string `json:"seedHex"`
	// BondOutpoint is the deposit the seed's bond key can reclaim, for
	// somebody checking a backup belongs to the coin they think it does.
	BondOutpoint string `json:"bondOutpoint,omitempty"`
}

// gamingHost is where the sandbox answers. It is a container on an internal
// network with no published ports, so this name resolves nowhere else.
func gamingHost() string {
	if h := strings.TrimSpace(os.Getenv("GAMING_HOST")); h != "" {
		return h
	}
	return "gaming"
}

// GamingIdentityBackup asks a running game for the seed it derives its keys
// from.
//
// It is deliberately a pull rather than something the game pushes. A game that
// could hand its host secrets unprompted is a game that could hand it anything,
// and the host would have to decide what to do with it; asking means the answer
// only exists when a person went looking for it.
func GamingIdentityBackup(ctx context.Context, game string) (GamingBackup, error) {
	game = strings.ToLower(strings.TrimSpace(game))
	if game == "" {
		return GamingBackup{}, fmt.Errorf("no game named")
	}

	settings := ReadGamingSettings()
	token := settings.GameTokens[game]
	if token == "" {
		return GamingBackup{}, fmt.Errorf("%q has no token, so it is not installed", game)
	}
	port := ReadGamingState().Ports[game]
	if port == 0 {
		// Installed but not running. The seed lives in the sandbox's
		// volume and only the game can read it, so there is nothing to
		// fetch until the portal has it up.
		return GamingBackup{}, fmt.Errorf("%q is not running, so it cannot be asked for its seed", game)
	}

	url := fmt.Sprintf("http://%s:%d/identity/backup", gamingHost(), port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return GamingBackup{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	// A game asks for a deliberate header before handing over its seed, so a
	// plain fetch from its own page cannot read one. This route is the
	// deliberate path.
	req.Header.Set("X-Poker-Confirm", "seed")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return GamingBackup{}, fmt.Errorf("ask %s for its seed: %w", game, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode != http.StatusOK {
		return GamingBackup{}, fmt.Errorf("%s answered %d: %s", game, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		SeedHex      string `json:"seedHex"`
		BondOutpoint string `json:"bondOutpoint"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return GamingBackup{}, fmt.Errorf("read %s's answer: %w", game, err)
	}
	if len(out.SeedHex) != 64 {
		return GamingBackup{}, fmt.Errorf("%s returned something that is not a seed", game)
	}
	return GamingBackup{Game: game, SeedHex: out.SeedHex, BondOutpoint: out.BondOutpoint}, nil
}
