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

	"dcrpulse/internal/config"
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
	grants.revoke(id) // zero any in-memory spend passphrase for this agent
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

// HasAgent reports whether an agent identity exists.
func HasAgent(id string) bool { return reg.has(id) }

// UnblockAgent clears an agent's tripwire block and persists the change. The
// agent must still be re-granted spend access separately. Returns false if no
// such agent existed.
func UnblockAgent(id string) (bool, error) {
	if !reg.unblock(id) {
		return false, nil
	}
	return true, saveAgents()
}

// Domains returns the capability domains that currently have tools, in a stable
// order, so the dashboard can render per-agent access toggles.
func Domains() []string { return catalogDomains() }

// LoadPersisted restores the saved agent roster into the registry. Safe to call
// even when MCP is disabled, so the Settings UI can manage tokens beforehand.
func LoadPersisted() error { return loadAgents() }

// ActiveSessions returns the currently-connected agents, for the dashboard UI.
func ActiveSessions() []Session { return reg.activeSessions(time.Now()) }

// Config holds the listener address and the first-run enable default. Once the
// user toggles MCP in the dashboard the persisted setting wins; Enable only
// seeds the very first run from the MCP_ENABLE env var.
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

// Listener runtime state. The server can be started and stopped at runtime from
// the dashboard, so the live *http.Server is held here behind srvMu.
var (
	srvMu   sync.Mutex
	httpSrv *http.Server // non-nil while the listener is running
	runBind = "127.0.0.1"
	runPort = "8090"
)

// Start records the listener address, registers the optional bootstrap token,
// and brings the server up if it should be enabled. The enabled state is the
// persisted dashboard toggle when present, otherwise the env default.
func Start(cfg Config) {
	srvMu.Lock()
	runBind, runPort = cfg.Bind, cfg.Port
	srvMu.Unlock()

	// Bootstrap token from the environment for headless/dev use. Registered
	// regardless of the enabled state so a later toggle-on can accept it.
	if t := os.Getenv("MCP_TOKEN"); t != "" {
		AddToken("env", "env-token", t)
	}

	enabled := cfg.Enable
	if v, ok := persistedEnabled(); ok {
		enabled = v
	}
	if !enabled {
		return
	}
	srvMu.Lock()
	defer srvMu.Unlock()
	if err := startListenerLocked(); err != nil {
		log.Printf("MCP server start: %v", err)
	}
}

// SetEnabled starts or stops the MCP listener at runtime and persists the new
// state. Enabling binds the port synchronously so a bind failure (e.g. the port
// is already in use) is reported to the caller; disabling drains in the
// background so the UI is not blocked by long-lived streams.
func SetEnabled(enabled bool) error {
	srvMu.Lock()
	if enabled {
		if httpSrv == nil {
			if err := startListenerLocked(); err != nil {
				srvMu.Unlock()
				return err
			}
		}
	} else if httpSrv != nil {
		srv := httpSrv
		httpSrv = nil
		go shutdownServer(srv)
	}
	srvMu.Unlock()
	return persistEnabled(enabled)
}

// Status reports whether the listener is currently running and where it binds.
func Status() (running bool, bind, port string) {
	srvMu.Lock()
	defer srvMu.Unlock()
	return httpSrv != nil, runBind, runPort
}

// startListenerLocked binds the port and serves in the background. The caller
// holds srvMu and has checked that httpSrv is nil.
func startListenerLocked() error {
	ln, err := net.Listen("tcp", net.JoinHostPort(runBind, runPort))
	if err != nil {
		return err
	}
	httpSrv = &http.Server{
		Handler:           reg.authMiddleware(buildHandler()),
		ReadHeaderTimeout: 15 * time.Second,
	}
	go func(s *http.Server) {
		log.Printf("MCP server listening on http://%s (streamable HTTP, bearer-auth, default domain=%q)", ln.Addr(), defaultDomain)
		if err := s.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("MCP server error: %v", err)
		}
	}(httpSrv)
	return nil
}

func shutdownServer(s *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.Shutdown(ctx)
	log.Printf("MCP server stopped")
}

// buildHandler builds the streamable-HTTP handler. The getServer callback
// resolves the agent (placed in context by authMiddleware) and returns its
// scoped server (built lazily, rebuilt when grants change).
func buildHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		if a, ok := agentFromContext(r.Context()); ok {
			return scopedServerFor(a)
		}
		return mcp.NewServer(&mcp.Implementation{Name: "dcrpulse", Version: serverVersion}, nil)
	}, nil)
}

// persistEnabled / persistedEnabled store the on/off toggle in the global config
// so it survives restarts and overrides the env default.
func persistEnabled(enabled bool) error {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return err
	}
	if err := gc.Set(config.KeyMCPEnabled, enabled); err != nil {
		return err
	}
	return gc.Save()
}

func persistedEnabled() (val bool, ok bool) {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return false, false
	}
	var v bool
	found, err := gc.Get(config.KeyMCPEnabled, &v)
	if err != nil || !found {
		return false, false
	}
	return v, true
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
			t.register(s, a)
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
// register only the tools that agent is permitted to use. register receives the
// agent so identity-aware tools (capability introspection, spend) can close over
// it; each agent has its own server, so the binding is per-agent.
type toolDef struct {
	domain   string
	register func(*mcp.Server, *agent)
}

// readTool builds a read-only tool: a thin adapter that calls fn and wraps its
// (value, error) result for MCP. In is the tool's argument type (emptyInput for
// tools that take no parameters); its exported fields become the input schema.
func readTool[In any](domain, name, description string, fn func(context.Context, In) (any, error)) toolDef {
	return toolDef{domain, func(s *mcp.Server, _ *agent) {
		mcp.AddTool(s, &mcp.Tool{Name: name, Description: description},
			func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, toolResult, error) {
				return ok(fn(ctx, in))
			})
	}}
}

// agentTool builds a tool whose handler also receives the calling agent's
// identity, for capability introspection and grant-gated spend tools.
func agentTool[In any](domain, name, description string, fn func(context.Context, *agent, In) (any, error)) toolDef {
	return toolDef{domain, func(s *mcp.Server, a *agent) {
		mcp.AddTool(s, &mcp.Tool{Name: name, Description: description},
			func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, toolResult, error) {
				return ok(fn(ctx, a, in))
			})
	}}
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

// toolCatalog is the master list of MCP tools, assembled from the per-domain
// tool files (tools_<domain>.go). Each tool is a thin adapter over the same
// services.* / rpc.* function the HTTP handlers use. Order here sets the order
// of the capability toggles in the dashboard. Spend/state-changing tools (gated
// on a user spend grant) are added in later phases.
var toolCatalog = func() []toolDef {
	var all []toolDef
	all = append(all, capabilityTools...)
	all = append(all, nodeTools...)
	all = append(all, walletTools...)
	all = append(all, stakingTools...)
	all = append(all, governanceTools...)
	all = append(all, treasuryTools...)
	all = append(all, lightningTools...)
	all = append(all, privacyTools...)
	all = append(all, explorerTools...)
	all = append(all, timestampTools...)
	all = append(all, torTools...)
	all = append(all, dexTools...)
	all = append(all, bisonrelayTools...)
	return all
}()
