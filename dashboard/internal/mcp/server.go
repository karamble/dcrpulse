// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/services"
)

const serverVersion = "0.1.0"

// reg is the process-wide agent/token + session registry, shared with the
// dashboard API (Settings -> Agents) so the user can manage tokens, grant
// per-agent domain access, and see live sessions.
var reg = func() *registry {
	r := newRegistry()
	r.onChange = invalidateAgentServer // rebuild a scoped server when grants change
	return r
}()

// AddToken registers a named bearer token (hash only) for an agent identity.
// New agents are limited to the default "node" domain until the user grants more.
func AddToken(id, name, token string) { reg.addToken(id, name, token) }

// CreateAgent mints a new named agent identity and returns its bearer token.
// The token is shown to the user only once; only its hash is retained.
func CreateAgent(name string) (id, token string, err error) {
	id, token, err = reg.create(name)
	if err != nil {
		return "", "", err
	}
	if err := saveAgents(); err != nil {
		reg.remove(id)
		return "", "", err
	}
	return id, token, nil
}

// RevokeAgent deletes an agent identity and persists the change. Returns false
// if no such agent existed.
func RevokeAgent(id string) (bool, error) {
	if !reg.remove(id) {
		return false, nil
	}
	return true, saveAgents()
}

// SetAgentDomains replaces (and persists) the capability domains granted to an
// agent. Returns false if no such agent existed.
func SetAgentDomains(id string, domains []string) (bool, error) {
	if !reg.setDomains(id, domains) {
		return false, nil
	}
	return true, saveAgents()
}

// ListAgents returns the agent roster (without tokens) for the dashboard UI.
func ListAgents() []AgentInfo { return reg.list() }

// Domains returns the capability domains that currently have tools, in a stable
// order, so the dashboard can render per-agent access toggles.
func Domains() []string { return catalogDomains() }

// LoadPersisted restores the saved agent roster into the registry. Safe to call
// even when MCP is disabled, so the Settings UI can manage tokens beforehand.
func LoadPersisted() error { return loadAgents() }

// ActiveSessions returns the currently-connected agents, for the dashboard UI.
func ActiveSessions() []Session { return reg.activeSessions(time.Now()) }

// Config controls the MCP listener. The server is disabled unless Enable is set.
type Config struct {
	Enable bool
	Bind   string
	Port   string
}

// ConfigFromEnv reads MCP_ENABLE / MCP_BIND / MCP_PORT.
func ConfigFromEnv() Config {
	return Config{
		Enable: os.Getenv("MCP_ENABLE") == "true",
		Bind:   envOr("MCP_BIND", "127.0.0.1"),
		Port:   envOr("MCP_PORT", "8090"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Start launches the MCP streamable-HTTP server in the background when enabled.
// It reuses the dashboard's in-process daemon clients via the services layer.
func Start(cfg Config) {
	if !cfg.Enable {
		return
	}
	// Bootstrap token from the environment for local testing. Persistent,
	// Settings-managed per-agent tokens are added in a later phase.
	if t := os.Getenv("MCP_TOKEN"); t != "" {
		AddToken("env", "env-token", t)
	}

	// One MCP server per agent, scoped to that agent's granted domains. getServer
	// resolves the agent (placed in context by authMiddleware) and returns its
	// scoped server (built lazily, rebuilt when grants change).
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		if a, ok := agentFromContext(r.Context()); ok {
			return scopedServerFor(a)
		}
		return mcp.NewServer(&mcp.Implementation{Name: "dcrpulse", Version: serverVersion}, nil)
	}, nil)

	addr := net.JoinHostPort(cfg.Bind, cfg.Port)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           reg.authMiddleware(handler),
		ReadHeaderTimeout: 15 * time.Second,
	}
	go func() {
		log.Printf("MCP server listening on http://%s (streamable HTTP, bearer-auth, default domain=%q)", addr, defaultDomain)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("MCP server error: %v", err)
		}
	}()
}

// per-agent scoped server cache (keyed by agent id), invalidated on grant change.
var (
	serversMu sync.Mutex
	servers   = map[string]*mcp.Server{}
)

func scopedServerFor(a *agent) *mcp.Server {
	serversMu.Lock()
	defer serversMu.Unlock()
	if s := servers[a.id]; s != nil {
		return s
	}
	s := buildServer(a)
	servers[a.id] = s
	return s
}

func invalidateAgentServer(id string) {
	serversMu.Lock()
	delete(servers, id)
	serversMu.Unlock()
}

// buildServer creates an MCP server exposing only the tools whose domain the
// agent is allowed (node-only by default).
func buildServer(a *agent) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "dcrpulse",
		Title:   "Decred Pulse",
		Version: serverVersion,
	}, nil)
	for _, t := range toolCatalog {
		if a.allows(t.domain) {
			t.register(s)
		}
	}
	return s
}

// emptyInput is the argument type for tools that take no parameters.
type emptyInput struct{}

// toolResult wraps a tool's payload in a JSON object. MCP requires a tool's
// structured output schema to be an object, so list/scalar results are nested
// under "data" rather than returned as bare arrays.
type toolResult struct {
	Data any `json:"data"`
}

// ok adapts a `(value, error)` service return into a tool result.
func ok(v any, err error) (*mcp.CallToolResult, toolResult, error) {
	if err != nil {
		return nil, toolResult{}, err
	}
	return nil, toolResult{Data: v}, nil
}

// toolDef tags each tool with its capability domain so the per-agent server can
// register only the tools that agent is permitted to use.
type toolDef struct {
	domain   string
	register func(*mcp.Server)
}

// catalogDomains returns the distinct capability domains in the catalog, in
// first-seen order, so the UI only offers domains that actually have tools.
func catalogDomains() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range toolCatalog {
		if !seen[t.domain] {
			seen[t.domain] = true
			out = append(out, t.domain)
		}
	}
	return out
}

// toolCatalog is the master list of MCP tools. Each is a thin adapter over the
// same services.* function the HTTP handlers use. Spend/state-changing tools
// (gated on a user spend grant) are added in later phases.
var toolCatalog = []toolDef{
	{"node", func(s *mcp.Server) {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "node_status",
			Description: "Get the dcrd node sync status: synced state, block height, peer count, and version.",
		}, func(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, toolResult, error) {
			return ok(services.FetchNodeStatus())
		})
	}},
	{"node", func(s *mcp.Server) {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "node_dashboard",
			Description: "Get the node dashboard summary: chain, circulating supply, staking and treasury overview.",
		}, func(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, toolResult, error) {
			return ok(services.FetchDashboardData())
		})
	}},
	{"node", func(s *mcp.Server) {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "node_blockchain_info",
			Description: "Get detailed blockchain info from dcrd (best block, difficulty, chainwork, etc.).",
		}, func(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, toolResult, error) {
			return ok(services.FetchBlockchainInfo())
		})
	}},
	{"wallet", func(s *mcp.Server) {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "wallet_dashboard",
			Description: "Get the active wallet overview: balances and wallet status.",
		}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, toolResult, error) {
			return ok(services.FetchWalletDashboardDataWithContext(ctx))
		})
	}},
	{"wallet", func(s *mcp.Server) {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "wallet_accounts",
			Description: "List the accounts in the active wallet with their balances.",
		}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, toolResult, error) {
			return ok(services.FetchAllAccounts(ctx))
		})
	}},
	{"staking", func(s *mcp.Server) {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "staking_tickets",
			Description: "List the active wallet's staking tickets with their lifecycle status (live, voted, etc.).",
		}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, toolResult, error) {
			return ok(services.ListTickets(ctx))
		})
	}},
}
