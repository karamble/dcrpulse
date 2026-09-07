// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"encoding/hex"
	"time"

	"dcrpulse/internal/config"
)

// persistedAgent is the on-disk shape of an agent identity. Only the token's
// SHA-256 hash is stored (hex), never the plaintext token.
type persistedAgent struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	TokenHash  string    `json:"tokenHash"`
	Domains    []string  `json:"domains"`
	AllowedIPs []string  `json:"allowedIps,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	Blocked    bool      `json:"blocked,omitempty"`
}

// snapshot captures the current roster for persistence, excluding the ephemeral
// environment bootstrap agent.
func (r *registry) snapshot() []persistedAgent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]persistedAgent, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, persistedAgent{
			ID:         a.id,
			Name:       a.name,
			TokenHash:  hex.EncodeToString(a.hash[:]),
			Domains:    sortedDomains(a.domainMap()),
			AllowedIPs: a.allowedIPEntries(),
			CreatedAt:  a.createdAt,
			Blocked:    a.blocked.Load(),
		})
	}
	return out
}

// loadAgents restores the persisted roster into the in-memory registry. An
// absent key (never configured) is not an error.
func loadAgents() error {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return err
	}
	var recs []persistedAgent
	ok, err := gc.Get(config.KeyMCPAgents, &recs)
	if err != nil || !ok {
		return err
	}
	for _, rec := range recs {
		raw, err := hex.DecodeString(rec.TokenHash)
		if err != nil || len(raw) != 32 {
			continue
		}
		var hash [32]byte
		copy(hash[:], raw)
		reg.addAgentRecord(rec.ID, rec.Name, hash, rec.Domains, rec.AllowedIPs, rec.CreatedAt, rec.Blocked)
	}
	return nil
}

// saveAgents writes the current roster back to the global config, preserving
// other keys via the raw-JSON layer.
func saveAgents() error {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return err
	}
	if err := gc.Set(config.KeyMCPAgents, reg.snapshot()); err != nil {
		return err
	}
	return gc.Save()
}
