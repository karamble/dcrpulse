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
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// defaultDomain is the only capability an agent has until the user grants more.
const defaultDomain = "node"

// agent is a named identity behind a bearer token, with the set of capability
// domains the user has granted it. Tokens are high-entropy, so a fast SHA-256
// (not bcrypt) is sufficient and keeps per-request auth cheap. The two mutable
// fields are atomic: both are read on every request (and the domain set on
// every tool call) while the settings API mutates them.
type agent struct {
	id         string
	name       string
	hash       [32]byte
	domains    atomic.Pointer[map[string]bool] // granted domains; defaults to {node}
	createdAt  time.Time
	blocked    atomic.Bool                   // tripwire: set when the agent attempts to exceed its grant
	allowedIPs atomic.Pointer[ipAllowlist]   // source-IP restriction; nil = any address
	lastDenied atomic.Pointer[DeniedAttempt] // latest allowed-IP denial; cleared on success
}

func (a *agent) allows(domain string) bool {
	if a == nil {
		return false
	}
	m := a.domains.Load()
	return m != nil && (*m)[domain]
}

// domainMap returns the agent's current domain set for read-only use.
func (a *agent) domainMap() map[string]bool {
	if a == nil {
		return nil
	}
	if m := a.domains.Load(); m != nil {
		return *m
	}
	return nil
}

func (a *agent) setDomainMap(m map[string]bool) { a.domains.Store(&m) }

// AgentInfo is the dashboard-facing view of an agent identity. It never
// includes the token (only its hash is stored, and not exposed here).
type AgentInfo struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Domains    []string       `json:"domains"`
	AllowedIPs []string       `json:"allowedIps"`
	LastDenied *DeniedAttempt `json:"lastDenied,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
	Blocked    bool           `json:"blocked"`
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
	r.addAgentRecord(id, name, sha256.Sum256([]byte(token)), []string{defaultDomain}, nil, time.Now(), false)
}

// addAgentRecord installs an agent from an already-hashed token. Used by the
// persistence layer to restore the roster on startup (including a persisted
// blocked state) and by addToken/create. The node domain is always implied.
func (r *registry) addAgentRecord(id, name string, hash [32]byte, domains, allowedIPs []string, createdAt time.Time, blocked bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := &agent{
		id:        id,
		name:      name,
		hash:      hash,
		createdAt: createdAt,
	}
	a.setDomainMap(domainSet(domains))
	a.setIPAllowlist(allowedIPs)
	a.blocked.Store(blocked)
	r.agents[id] = a
}

// create mints a new agent: a random id and a high-entropy bearer token. Only
// the token's hash is retained; the plaintext is returned once for the user to
// copy and is never recoverable afterward.
func (r *registry) create(name string) (id, token string, err error) {
	id, err = randomHex(8)
	if err != nil {
		return "", "", err
	}
	raw, err := randomToken()
	if err != nil {
		return "", "", err
	}
	r.addAgentRecord(id, name, sha256.Sum256([]byte(raw)), []string{defaultDomain}, nil, time.Now(), false)
	return id, raw, nil
}

// remove deletes an agent and any live session, and drops its cached scoped
// server. Returns false if no such agent existed.
func (r *registry) remove(id string) bool {
	r.mu.Lock()
	_, ok := r.agents[id]
	delete(r.agents, id)
	delete(r.sessions, id)
	onChange := r.onChange
	r.mu.Unlock()
	if ok && onChange != nil {
		onChange(id)
	}
	return ok
}

// has reports whether an agent with this id exists.
func (r *registry) has(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.agents[id]
	return ok
}

// agent returns the agent with this id, or nil if unknown.
func (r *registry) agent(id string) *agent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.agents[id]
}

// name returns an agent's display name, or "" if unknown.
func (r *registry) name(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.agents[id]; a != nil {
		return a.name
	}
	return ""
}

// block marks an agent's token as blocked (tripwire), drops its live session,
// and invalidates its cached server. A blocked token is rejected at auth.
func (r *registry) block(id string) {
	r.mu.Lock()
	a := r.agents[id]
	if a != nil {
		a.blocked.Store(true)
	}
	delete(r.sessions, id)
	onChange := r.onChange
	r.mu.Unlock()
	if a != nil && onChange != nil {
		onChange(id)
	}
}

// blockAllAgents blocks every agent and drops all live sessions, invalidating
// each agent's cached scoped server. Used by the freeze-all kill-switch.
func (r *registry) blockAllAgents() {
	r.mu.Lock()
	ids := make([]string, 0, len(r.agents))
	for id, a := range r.agents {
		a.blocked.Store(true)
		ids = append(ids, id)
	}
	r.sessions = map[string]*Session{}
	onChange := r.onChange
	r.mu.Unlock()
	if onChange != nil {
		for _, id := range ids {
			onChange(id)
		}
	}
}

// unblock clears an agent's blocked state. Returns false if no such agent.
func (r *registry) unblock(id string) bool {
	r.mu.Lock()
	a := r.agents[id]
	if a != nil {
		a.blocked.Store(false)
	}
	r.mu.Unlock()
	return a != nil
}

// list returns the agent roster (without tokens), newest first.
func (r *registry) list() []AgentInfo {
	r.mu.Lock()
	out := make([]AgentInfo, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, AgentInfo{
			ID:         a.id,
			Name:       a.name,
			Domains:    sortedDomains(a.domainMap()),
			AllowedIPs: a.allowedIPEntries(),
			LastDenied: a.lastDenied.Load(),
			CreatedAt:  a.createdAt,
			Blocked:    a.blocked.Load(),
		})
	}
	r.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// domainSet builds the granted-domain set, always including the node default.
func domainSet(domains []string) map[string]bool {
	m := map[string]bool{defaultDomain: true}
	for _, d := range domains {
		if d != "" {
			m[d] = true
		}
	}
	return m
}

// sortedDomains returns the granted domains in stable alphabetical order.
func sortedDomains(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for d := range m {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// randomToken returns a URL-safe, high-entropy bearer token (256 bits).
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "mcp_" + base64.RawURLEncoding.EncodeToString(b), nil
}

// setDomains replaces an agent's granted domains (node always implied). Triggers
// rebuild of that agent's scoped MCP server. Returns false if no such agent.
func (r *registry) setDomains(id string, domains []string) bool {
	r.mu.Lock()
	a := r.agents[id]
	if a != nil {
		a.setDomainMap(domainSet(domains))
	}
	onChange := r.onChange
	r.mu.Unlock()
	if a != nil && onChange != nil {
		onChange(id)
	}
	return a != nil
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
		if !a.remoteAllowed(req.RemoteAddr) {
			// Byte-identical to the bad-token response above so a caller from
			// a non-allowed address cannot learn that the token is valid
			// (which the distinct blocked message below would reveal). Logged
			// regardless of the activity-log toggle, and remembered so the
			// settings UI can show the operator the address to allow.
			a.recordDenied(req.RemoteAddr, time.Now())
			mcpLog.Warnf("Agent %s denied: remote address %s is not in its allowed-IP list",
				sanitizeLogField(a.name), sanitizeLogField(req.RemoteAddr))
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		a.lastDenied.Store(nil)
		if a.blocked.Load() {
			http.Error(w, "agent blocked: a spend-limit violation revoked this token; the user must unblock it in the dashboard", http.StatusForbidden)
			return
		}
		r.touch(a, req.RemoteAddr, time.Now())
		next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), agentCtxKey, a)))
	})
}
