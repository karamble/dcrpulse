// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package mcp serves dcrpulse's capabilities to AI agents over the Model Context
// Protocol (streamable HTTP). Agents authenticate with a per-agent bearer token.
// Access is granular and per-agent: on first connect an agent may use ONLY the
// "node" domain (read blockchain/node status); every other domain is granted by
// the user in the dashboard. Fund-moving tools additionally require a user-granted
// spend capability (later phase) so an agent never receives the wallet passphrase.
package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
	"sync"
	"time"
)

// defaultDomain is the only capability an agent has until the user grants more.
const defaultDomain = "node"

// agent is a named identity behind a bearer token, with the set of capability
// domains the user has granted it. Tokens are high-entropy, so a fast SHA-256
// (not bcrypt) is sufficient and keeps per-request auth cheap.
type agent struct {
	id      string
	name    string
	hash    [32]byte
	domains map[string]bool // granted domains; defaults to {node}
}

func (a *agent) allows(domain string) bool {
	return a != nil && a.domains[domain]
}

// Session is a recently-active agent connection, surfaced to the dashboard so the
// user can see which identified agents are connected.
type Session struct {
	AgentID   string    `json:"agentId"`
	Name      string    `json:"name"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
	Remote    string    `json:"remote"`
}

const sessionActiveWindow = 90 * time.Second

type registry struct {
	mu       sync.Mutex
	agents   map[string]*agent
	sessions map[string]*Session
	onChange func(agentID string) // invalidate cached per-agent server on grant change
}

func newRegistry() *registry {
	return &registry{agents: map[string]*agent{}, sessions: map[string]*Session{}}
}

// addToken registers a named bearer token, storing only its hash. New agents
// start with only the default ("node") domain.
func (r *registry) addToken(id, name, token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[id] = &agent{
		id:      id,
		name:    name,
		hash:    sha256.Sum256([]byte(token)),
		domains: map[string]bool{defaultDomain: true},
	}
}

// setDomains replaces an agent's granted domains (node always implied). Triggers
// rebuild of that agent's scoped MCP server.
func (r *registry) setDomains(id string, domains []string) {
	r.mu.Lock()
	a := r.agents[id]
	if a != nil {
		m := map[string]bool{defaultDomain: true}
		for _, d := range domains {
			m[d] = true
		}
		a.domains = m
	}
	onChange := r.onChange
	r.mu.Unlock()
	if a != nil && onChange != nil {
		onChange(id)
	}
}

// verify resolves a bearer token to its agent identity in constant time.
func (r *registry) verify(token string) (*agent, bool) {
	if token == "" {
		return nil, false
	}
	sum := sha256.Sum256([]byte(token))
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.agents {
		if subtle.ConstantTimeCompare(a.hash[:], sum[:]) == 1 {
			return a, true
		}
	}
	return nil, false
}

func (r *registry) touch(a *agent, remote string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sessions[a.id]
	if s == nil {
		s = &Session{AgentID: a.id, Name: a.name, FirstSeen: now}
		r.sessions[a.id] = s
	}
	s.LastSeen = now
	s.Remote = remote
}

// activeSessions returns agents seen within the active window.
func (r *registry) activeSessions(now time.Time) []Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		if now.Sub(s.LastSeen) <= sessionActiveWindow {
			out = append(out, *s)
		}
	}
	return out
}

func bearerToken(req *http.Request) string {
	const p = "Bearer "
	if h := req.Header.Get("Authorization"); strings.HasPrefix(h, p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}

// agent identity passes from the auth middleware to getServer via the request
// context, so each agent gets an MCP server scoped to its granted domains.
type ctxKey int

const agentCtxKey ctxKey = iota

func agentFromContext(ctx context.Context) (*agent, bool) {
	a, ok := ctx.Value(agentCtxKey).(*agent)
	return a, ok
}

// authMiddleware authenticates the bearer token, records the live session, and
// stashes the resolved agent in the request context for per-agent tool scoping.
func (r *registry) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		a, ok := r.verify(bearerToken(req))
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r.touch(a, req.RemoteAddr, time.Now())
		next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), agentCtxKey, a)))
	})
}
