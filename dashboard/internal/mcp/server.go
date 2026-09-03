// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/auth"
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

// SetAgentAllowedIPs replaces (and persists) the source-IP allowlist enforced
// on an agent's requests. Entries must already be canonical (ParseAllowedIPs);
// an empty list clears the restriction. Returns false if no such agent existed.
func SetAgentAllowedIPs(id string, ips []string) (bool, error) {
	if !reg.setAllowedIPs(id, ips) {
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

// FreezeAllAgents is the kill-switch: it revokes every spend grant (zeroing the
// held passphrases) and blocks every agent token at once, persisting the blocked
// state. Agents are restored one at a time via UnblockAgent plus a fresh spend
// grant. Returns the error from persisting the blocked state.
func FreezeAllAgents() error {
	grants.revokeAll()
	reg.blockAllAgents()
	return saveAgents()
}

// ExportAuditLog returns the full persisted spend-audit trail as JSON.
func ExportAuditLog() ([]byte, error) { return exportAudit() }

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

	// Open the persisted spend-audit log so the trail survives restarts.
	initAuditStore()
	// Seed the activity-log gate before any listener can serve a request.
	applyPersistedLogging()

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
		mcpLog.Infof("MCP server not started: disabled. Turn it on under Settings > AI Agents.")
		return
	}
	// The same precondition the settings route enforces: without a dashboard
	// password the routes that mint tokens and widen an agent's authority are
	// open, so the agent surface must not come up on a persisted flag either.
	if !auth.Enabled() {
		mcpLog.Warnf("MCP server not started: %s", auth.ErrAppPasswordRequired)
		return
	}
	srvMu.Lock()
	defer srvMu.Unlock()
	if err := startListenerLocked(); err != nil {
		mcpLog.Errorf("MCP server start: %v", err)
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
	// Bridge the live event buses into MCP resource notifications (once).
	startResourceFeeds()
	go func(s *http.Server) {
		mcpLog.Infof("MCP server listening on http://%s (streamable HTTP, bearer-auth, default domain=%q)", ln.Addr(), defaultDomain)
		if err := s.Serve(ln); err != nil && err != http.ErrServerClosed {
			mcpLog.Errorf("MCP server error: %v", err)
		}
	}(httpSrv)
	return nil
}

func shutdownServer(s *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.Shutdown(ctx)
	mcpLog.Infof("MCP server stopped")
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
	}, &mcp.StreamableHTTPOptions{
		// The SDK serves the 2026-07-28 wire only in stateless mode (stateful
		// answers it 400). Stateless serves both protocol generations per
		// request, and every request re-resolves the agent's scoped server, so
		// a revocation applies immediately; no session survives to outlive it.
		Stateless: true,
		// The default 4 MiB cap would 413 the base64 file tools (br_file_add,
		// br_file_send, br_store_file_upload).
		MaxRequestBodyBytes: 16 << 20,
		// PropagateRequestCancellation stays off: an HTTP disconnect must not
		// become a cancel trigger inside spend handlers.
		Logger: sdkLogger,
	})
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
	// Subscription handlers gate which resources an agent may subscribe to (only
	// those in a granted domain) and advertise the resources subscribe capability.
	opts := &mcp.ServerOptions{
		SubscribeHandler:   func(_ context.Context, req *mcp.SubscribeRequest) error { return allowResourceSub(a, req.Params.URI) },
		UnsubscribeHandler: func(context.Context, *mcp.UnsubscribeRequest) error { return nil },
		Logger:             sdkLogger,
	}
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "dcrpulse",
		Title:   "Decred Pulse",
		Version: serverVersion,
	}, opts)
	s.AddReceivingMiddleware(activityMiddleware(a))
	for _, t := range toolCatalog {
		if a.allows(t.domain) {
			t.register(s, a)
		}
	}
	for _, rd := range resourceCatalog {
		if a.allows(rd.domain) {
			rd.register(s)
		}
	}
	return s
}

// emptyInput is the argument type for tools that take no parameters.
type emptyInput struct{}

// toolResult wraps a tool's payload so structured content is always an object,
// with list/scalar results nested under "data". Handlers return it through the
// `any` output parameter so the go-sdk omits a generated output schema: a schema
// inferred from the `any` payload emits `"data": true`, which strict MCP clients
// reject when listing tools. The value still travels as structured content.
type toolResult struct {
	Data any `json:"data"`
}

// ok adapts a `(value, error)` service return into a tool result. The value is
// returned as `any` (see toolResult) so no output schema is generated.
func ok(v any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	return nil, toolResult{Data: v}, nil
}

// toolDef tags each tool with its capability domain so the per-agent server can
// register only the tools that agent is permitted to use. register receives the
// agent so identity-aware tools (capability introspection, spend) can close over
// it; each agent has its own server, so the binding is per-agent. readOnly marks
// pure reads (vs grant-gated writes) so the MCP annotations and the gating test
// can distinguish them.
type toolDef struct {
	domain   string
	name     string
	readOnly bool
	register func(*mcp.Server, *agent)
}

// requireDomain re-checks the agent's capability domain when the tool is called.
// Domains are also applied when the per-agent server is built, but a live
// session keeps the server it was built with, so without this a domain the user
// revoked would keep working until that session ended.
func requireDomain(a *agent, domain string) error {
	if !a.allows(domain) {
		return fmt.Errorf("agent access does not allow the %s domain", domain)
	}
	return nil
}

func boolPtr(b bool) *bool { return &b }

// MCP tool hints so clients can tell a read from a destructive (fund/state-
// changing) call and warn before executing one. The pointers are never mutated,
// so the shared values are safe to attach to every tool.
var (
	readAnnotations  = &mcp.ToolAnnotations{ReadOnlyHint: true}
	writeAnnotations = &mcp.ToolAnnotations{DestructiveHint: boolPtr(true), OpenWorldHint: boolPtr(true)}
)

// readTool builds a read-only tool: a thin adapter that calls fn and wraps its
// (value, error) result for MCP. In is the tool's argument type (emptyInput for
// tools that take no parameters); its exported fields become the input schema.
func readTool[In any](domain, name, description string, fn func(context.Context, In) (any, error)) toolDef {
	return toolDef{domain: domain, name: name, readOnly: true, register: func(s *mcp.Server, a *agent) {
		mcp.AddTool(s, &mcp.Tool{Name: name, Description: description, Annotations: readAnnotations},
			func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
				if err := requireDomain(a, domain); err != nil {
					return ok(nil, err)
				}
				return ok(fn(ctx, in))
			})
	}}
}

// agentTool builds a grant-gated write tool whose handler also receives the
// calling agent's identity.
func agentTool[In any](domain, name, description string, fn func(context.Context, *agent, In) (any, error)) toolDef {
	return toolDef{domain: domain, name: name, readOnly: false, register: func(s *mcp.Server, a *agent) {
		mcp.AddTool(s, &mcp.Tool{Name: name, Description: description, Annotations: writeAnnotations},
			func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
				if err := requireDomain(a, domain); err != nil {
					return ok(nil, err)
				}
				return ok(fn(ctx, a, in))
			})
	}}
}

// agentToolDesc is agentTool with a description computed per agent at
// registration, so a tool can name the accounts and VSPs this wallet actually
// uses instead of describing its fields in the abstract. The text is a hint: the
// grant checks inside the handler remain the authority.
func agentToolDesc[In any](domain, name string, describe func(*agent) string, fn func(context.Context, *agent, In) (any, error)) toolDef {
	return toolDef{domain: domain, name: name, readOnly: false, register: func(s *mcp.Server, a *agent) {
		mcp.AddTool(s, &mcp.Tool{Name: name, Description: describe(a), Annotations: writeAnnotations},
			func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
				if err := requireDomain(a, domain); err != nil {
					return ok(nil, err)
				}
				return ok(fn(ctx, a, in))
			})
	}}
}

// agentReadTool is an agent-aware but read-only tool (introspection like
// capabilities): it receives the agent yet moves nothing, so it carries the
// read-only hint and is exempt from the spend-gating test.
func agentReadTool[In any](domain, name, description string, fn func(context.Context, *agent, In) (any, error)) toolDef {
	return toolDef{domain: domain, name: name, readOnly: true, register: func(s *mcp.Server, a *agent) {
		mcp.AddTool(s, &mcp.Tool{Name: name, Description: description, Annotations: readAnnotations},
			func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
				if err := requireDomain(a, domain); err != nil {
					return ok(nil, err)
				}
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
	for _, rd := range resourceCatalog {
		if !seen[rd.domain] {
			seen[rd.domain] = true
			out = append(out, rd.domain)
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
