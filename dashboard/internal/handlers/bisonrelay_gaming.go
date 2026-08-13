// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/auth"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// The Bison Relay gaming section's confinement policy (Bison Relay > Gaming).
// Games are untrusted plugins running beside a wallet, dcrlnd and a BR
// identity, so they never hold wallet credentials: they reach one account,
// under caps, through this policy. Storage speaks atoms; the frontend speaks
// DCR, converted here the same way the BR-MCP handlers do.

// gamePolicyView is one game's policy, DCR-denominated.
type gamePolicyView struct {
	Name                string  `json:"name"`
	Account             string  `json:"account"`
	PerTableCapDcr      float64 `json:"perTableCapDcr"`
	PerDayCapDcr        float64 `json:"perDayCapDcr"`
	ApprovalTimeoutSecs int     `json:"approvalTimeoutSecs"`
}

// gamingSettingsView is the DCR-denominated frontend shape.
type gamingSettingsView struct {
	Enabled         bool                      `json:"enabled"`
	RegisteredGames []string                  `json:"registeredGames"`
	Policies        map[string]gamePolicyView `json:"policies"`

	// GameCredentials says which games have been issued a credential and
	// when, so the console can offer generate or regenerate and say how old
	// the current one is.
	//
	// It carries no certificate and no key - only the fingerprint, which is
	// enough to tell two credentials apart and useless for connecting with.
	// It travels outward only: a caller that could set one could name another
	// game's identity, and a game must not be able to choose its own either.
	GameCredentials map[string]gameCredentialView `json:"gameCredentials"`
}

// gameCredentialView is what the console is told about a game's credential.
type gameCredentialView struct {
	Fingerprint string `json:"fingerprint"`
	IssuedAt    int64  `json:"issuedAt"`
}

func gamingToView(s types.GamingSettings) gamingSettingsView {
	if s.RegisteredGames == nil {
		s.RegisteredGames = []string{}
	}
	creds := make(map[string]gameCredentialView, len(s.GameCredentials))
	for id, c := range s.GameCredentials {
		creds[id] = gameCredentialView{Fingerprint: c.Fingerprint, IssuedAt: c.IssuedAt}
	}
	policies := make(map[string]gamePolicyView, len(s.Policies))
	for id, p := range s.Policies {
		policies[id] = gamePolicyView{
			Name:                p.Name,
			Account:             p.Account,
			PerTableCapDcr:      dcrutil.Amount(p.PerTableCapAtoms).ToCoin(),
			PerDayCapDcr:        dcrutil.Amount(p.PerDayCapAtoms).ToCoin(),
			ApprovalTimeoutSecs: p.ApprovalTimeoutSecs,
		}
	}
	return gamingSettingsView{
		Enabled:         s.Enabled,
		RegisteredGames: s.RegisteredGames,
		Policies:        policies,
		GameCredentials: creds,
	}
}

func gamingFromView(v gamingSettingsView) (types.GamingSettings, error) {
	policies := make(map[string]types.GamePolicy, len(v.Policies))
	for id, p := range v.Policies {
		perTable, err := dcrutil.NewAmount(p.PerTableCapDcr)
		if err != nil {
			return types.GamingSettings{}, fmt.Errorf("%s per-table cap: %w", id, err)
		}
		perDay, err := dcrutil.NewAmount(p.PerDayCapDcr)
		if err != nil {
			return types.GamingSettings{}, fmt.Errorf("%s per-day cap: %w", id, err)
		}
		policies[id] = types.GamePolicy{
			Name:                p.Name,
			Account:             p.Account,
			PerTableCapAtoms:    int64(perTable),
			PerDayCapAtoms:      int64(perDay),
			ApprovalTimeoutSecs: p.ApprovalTimeoutSecs,
		}
	}
	return types.GamingSettings{
		Enabled:         v.Enabled,
		RegisteredGames: v.RegisteredGames,
		Policies:        policies,
		// GameCredentials is deliberately not read back: a credential is
		// issued here, and a caller that could set one could name another
		// game's identity.
	}, nil
}

func gamingJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// BisonrelayGamingSettingsHandler round-trips the gaming confinement policy.
func BisonrelayGamingSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		gamingJSON(w, gamingToView(services.ReadGamingSettings()))
	case http.MethodPost:
		var in gamingSettingsView
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		next, err := gamingFromView(in)
		if err != nil {
			http.Error(w, "bad amount: "+err.Error(), http.StatusBadRequest)
			return
		}
		// The bridge is only ever live behind the App Password: the caps
		// below and the approval a spend waits on are worth nothing if the
		// person approving cannot be told from anybody who reached the port.
		saved, err := services.WriteGamingSettings(next, auth.Enabled())
		switch {
		case err == nil:
		case errors.Is(err, services.ErrGamingNeedsAppPassword):
			http.Error(w, err.Error(), http.StatusConflict)
			return
		case errors.Is(err, services.ErrGamingBadGameID):
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		default:
			http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		gamingJSON(w, gamingToView(saved))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// BisonrelayGamingGamesHandler lists the games the operator registered.
func BisonrelayGamingGamesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	gamingJSON(w, map[string]any{"games": services.GamingGames()})
}

// BisonrelayGamingSpendsHandler lists what games have asked to spend, and what
// was decided, for a person to read.
func BisonrelayGamingSpendsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	gamingJSON(w, map[string]any{"spends": services.GamingSpends()})
}

// BisonrelayGamingSpendDecideHandler is a person answering a game's request.
//
// Approving takes the passphrase, because that is the only thing that can move
// money here and it belongs to them. What is paid is the amount and address
// recorded when the request was made, never anything passed in now - so what
// is signed is what was shown.
func BisonrelayGamingSpendDecideHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID         string `json:"id"`
		Approve    bool   `json:"approve"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	passphrase := []byte(req.Passphrase)
	req.Passphrase = ""

	var (
		spend services.GamingSpend
		err   error
	)
	if req.Approve {
		if len(passphrase) == 0 {
			http.Error(w, "passphrase is required to approve a spend", http.StatusBadRequest)
			return
		}
		spend, err = services.ApproveGamingSpend(r.Context(), strings.TrimSpace(req.ID), passphrase)
	} else {
		spend, err = services.DenyGamingSpend(strings.TrimSpace(req.ID))
	}

	switch {
	case err == nil:
		gamingJSON(w, spend)
	case errors.Is(err, services.ErrGamingSpendNotFound):
		http.Error(w, "no such spend request", http.StatusNotFound)
	case errors.Is(err, services.ErrGamingSpendNotPending):
		http.Error(w, "that request was already decided", http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}

// A game's seed is deliberately not reachable from here. It is the one secret
// this host does not hold a copy of, and a game now runs on a machine of the
// person's choosing rather than in a volume beside the wallet - so fetching it
// would carry it across a network to a page a browser session can read, to
// solve a problem the person is already standing in front of. The game offers
// its own backup; this only reports whether they have taken it.
