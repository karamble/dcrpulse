// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/gorilla/mux"

	"dcrpulse/internal/auth"
	"dcrpulse/internal/mcp"
	"dcrpulse/internal/services"
	"dcrpulse/internal/utils"
)

const maxAgentNameLen = 64

// grantDailyWindow mirrors the in-memory grant store's rolling spend window for
// computing how much of the daily cap is still available for display.
const grantDailyWindow = 24 * time.Hour

// mcpSettingsResponse describes the MCP server state for the Settings -> Agents
// panel: whether the listener is enabled and where, the capability domains that
// can be granted, the saved agent roster, and the live sessions.
type mcpSettingsResponse struct {
	Enabled     bool                 `json:"enabled"`
	Bind        string               `json:"bind"`
	Port        string               `json:"port"`
	Domains     []string             `json:"domains"`
	WriteScopes []mcp.WriteScope     `json:"writeScopes"`
	Agents      []mcp.AgentInfo      `json:"agents"`
	Sessions    []mcp.Session        `json:"sessions"`
	Grants      map[string]grantView `json:"grants"`
	Audit       []mcp.AuditEntry     `json:"audit"`
	Notify      mcp.OversightConfig  `json:"notify"`
	Logging     mcp.LoggingConfig    `json:"logging"`
}

// grantView is the dashboard-facing spend grant (DCR amounts, no passphrase).
type grantView struct {
	Accounts          []uint32 `json:"accounts"`
	PerTxDCR          float64  `json:"perTxDcr"`
	DailyDCR          float64  `json:"dailyDcr"`
	SpentTodayDCR     float64  `json:"spentTodayDcr"`
	RemainingTodayDCR float64  `json:"remainingTodayDcr"`
	Allowlist         []string `json:"allowlist"`
	Expiry            string   `json:"expiry,omitempty"`
	WriteScopes       []string `json:"writeScopes"`
}

func toGrantView(info mcp.GrantInfo) grantView {
	spent := info.SpentAtoms
	if time.Since(info.WindowStart) >= grantDailyWindow {
		spent = 0
	}
	gv := grantView{
		Accounts:      info.Accounts,
		PerTxDCR:      dcrutil.Amount(info.PerTxAtoms).ToCoin(),
		DailyDCR:      dcrutil.Amount(info.DailyAtoms).ToCoin(),
		SpentTodayDCR: dcrutil.Amount(spent).ToCoin(),
		Allowlist:     info.Allowlist,
		WriteScopes:   info.WriteScopes,
	}
	rem := info.DailyAtoms - spent
	if rem < 0 {
		rem = 0
	}
	gv.RemainingTodayDCR = dcrutil.Amount(rem).ToCoin()
	if !info.Expiry.IsZero() {
		gv.Expiry = info.Expiry.Format(time.RFC3339)
	}
	return gv
}

// MCPSettingsHandler returns the live MCP listener state plus the agent roster
// and live sessions.
func MCPSettingsHandler(w http.ResponseWriter, r *http.Request) {
	running, bind, port := mcp.Status()
	agents := mcp.ListAgents()
	gmap := make(map[string]grantView)
	for _, ag := range agents {
		if info, ok := mcp.SpendGrantInfo(ag.ID); ok {
			gmap[ag.ID] = toGrantView(info)
		}
	}
	resp := mcpSettingsResponse{
		Enabled:     running,
		Bind:        bind,
		Port:        port,
		Domains:     mcp.Domains(),
		WriteScopes: mcp.WriteScopes(),
		Agents:      agents,
		Sessions:    mcp.ActiveSessions(),
		Grants:      gmap,
		Audit:       mcp.AuditLog(50),
		Notify:      mcp.Oversight(),
		Logging:     mcp.Logging(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// SetMCPEnabledHandler starts or stops the MCP listener and persists the new
// on/off state. It returns the resulting live status.
func SetMCPEnabledHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// The agent surface can move funds, so it must not run on an install whose
	// dashboard has no password: the settings that widen an agent's authority
	// would be reachable by anything that can talk to the port.
	if req.Enabled && !auth.Enabled() {
		http.Error(w, auth.ErrAppPasswordRequired, http.StatusConflict)
		return
	}
	if err := mcp.SetEnabled(req.Enabled); err != nil {
		http.Error(w, fmt.Sprintf("failed to update MCP server: %v", err), http.StatusInternalServerError)
		return
	}
	running, bind, port := mcp.Status()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Enabled bool   `json:"enabled"`
		Bind    string `json:"bind"`
		Port    string `json:"port"`
	}{Enabled: running, Bind: bind, Port: port})
}

// CreateMCPTokenHandler mints a new named agent token. The plaintext token is
// returned exactly once; only its hash is stored.
func CreateMCPTokenHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, "agent name is required", http.StatusBadRequest)
		return
	}
	if len(name) > maxAgentNameLen {
		http.Error(w, "agent name too long", http.StatusBadRequest)
		return
	}
	id, token, err := mcp.CreateAgent(name)
	if err != nil {
		http.Error(w, "failed to create agent token", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	}{ID: id, Name: name, Token: token})
}

// RevokeMCPTokenHandler deletes an agent identity.
func RevokeMCPTokenHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(mux.Vars(r)["id"])
	ok, err := mcp.RevokeAgent(id)
	if err != nil {
		http.Error(w, "failed to revoke agent token", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetMCPAgentDomainsHandler replaces the capability domains granted to an agent.
// Unknown domains are ignored; the node domain is always implied server-side.
func SetMCPAgentDomainsHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(mux.Vars(r)["id"])
	var req struct {
		Domains []string `json:"domains"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	known := map[string]bool{}
	for _, d := range mcp.Domains() {
		known[d] = true
	}
	filtered := make([]string, 0, len(req.Domains))
	for _, d := range req.Domains {
		if known[d] {
			filtered = append(filtered, d)
		}
	}
	ok, err := mcp.SetAgentDomains(id, filtered)
	if err != nil {
		http.Error(w, "failed to update agent access", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetMCPAgentAllowedIPsHandler replaces the source-IP allowlist enforced on an
// agent's requests. Entries are single IPs or CIDR prefixes (IPv4 or IPv6); an
// invalid entry rejects the whole request by name, and an empty list clears
// the restriction.
func SetMCPAgentAllowedIPsHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(mux.Vars(r)["id"])
	var req struct {
		IPs []string `json:"ips"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	ips, err := mcp.ParseAllowedIPs(req.IPs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ok, err := mcp.SetAgentAllowedIPs(id, ips)
	if err != nil {
		http.Error(w, "failed to update agent allowed IPs", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetMCPGrantHandler grants an agent an account-scoped spend capability. The
// user supplies the wallet passphrase here; it is verified once, then held in
// memory on the agent's behalf (never persisted, never sent to the agent).
func SetMCPGrantHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(mux.Vars(r)["id"])
	if !mcp.HasAgent(id) {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	var req struct {
		Accounts    []uint32 `json:"accounts"`
		PerTxDCR    float64  `json:"perTxDcr"`
		DailyDCR    float64  `json:"dailyDcr"`
		Allowlist   []string `json:"allowlist"`
		ExpiryHours float64  `json:"expiryHours"`
		Passphrase  string   `json:"passphrase"`
		WriteScopes []string `json:"writeScopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	pass := []byte(req.Passphrase)
	req.Passphrase = ""
	defer utils.Zero(pass)

	// Drop unknown scopes; learn whether any retained scope signs with the
	// passphrase (needsPass) or draws on the DCR spend budget (needsFund).
	scopes, needsPass, needsFund := mcp.FilterScopes(req.WriteScopes)

	// A grant must enable at least one capability.
	if len(req.Accounts) == 0 && len(scopes) == 0 {
		http.Error(w, "grant must cover at least one account or enable an action", http.StatusBadRequest)
		return
	}
	if req.PerTxDCR < 0 || req.DailyDCR < 0 {
		http.Error(w, "caps must not be negative", http.StatusBadRequest)
		return
	}
	// Fund-moving grants require explicit positive caps. Caps are literal limits
	// (0 permits nothing); there is no unlimited.
	if len(req.Accounts) > 0 || needsFund {
		if req.PerTxDCR <= 0 || req.DailyDCR <= 0 {
			http.Error(w, "set a per-transaction and daily cap (greater than 0)", http.StatusBadRequest)
			return
		}
	}
	perTx, err := dcrutil.NewAmount(req.PerTxDCR)
	if err != nil {
		http.Error(w, "invalid per-transaction cap", http.StatusBadRequest)
		return
	}
	daily, err := dcrutil.NewAmount(req.DailyDCR)
	if err != nil {
		http.Error(w, "invalid daily cap", http.StatusBadRequest)
		return
	}

	// The passphrase is needed only for capabilities that sign with it: spending
	// from accounts, or a scope flagged NeedsPass (governance voting, staking).
	// Other scopes (Lightning, DEX, Bison Relay write, timestamp, tor, mixer)
	// move no wallet funds via the passphrase and need none.
	if len(req.Accounts) > 0 || needsPass {
		if len(pass) == 0 {
			http.Error(w, "wallet passphrase is required for spend or voting access", http.StatusBadRequest)
			return
		}
		// Verify before holding it in memory, so a typo is caught now.
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := services.VerifyWalletPassphrase(ctx, pass); err != nil {
			http.Error(w, "wallet passphrase verification failed", http.StatusBadRequest)
			return
		}
	}

	allow := make([]string, 0, len(req.Allowlist))
	for _, a := range req.Allowlist {
		if s := strings.TrimSpace(a); s != "" {
			allow = append(allow, s)
		}
	}
	var expiry time.Time
	if req.ExpiryHours > 0 {
		expiry = time.Now().Add(time.Duration(req.ExpiryHours * float64(time.Hour)))
	}

	mcp.SetSpendGrant(id, mcp.GrantSpec{
		Accounts:    req.Accounts,
		PerTxAtoms:  int64(perTx),
		DailyAtoms:  int64(daily),
		Allowlist:   allow,
		Expiry:      expiry,
		Passphrase:  pass, // copied by the store; our slice is wiped on return
		WriteScopes: scopes,
	})
	w.WriteHeader(http.StatusNoContent)
}

// RevokeMCPGrantHandler clears an agent's spend grant (zeroing the passphrase).
func RevokeMCPGrantHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(mux.Vars(r)["id"])
	mcp.RevokeSpendGrant(id)
	w.WriteHeader(http.StatusNoContent)
}

// UnblockMCPAgentHandler clears an agent's tripwire block so its token works
// again. The agent must still be re-granted spend access separately.
func UnblockMCPAgentHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(mux.Vars(r)["id"])
	ok, err := mcp.UnblockAgent(id)
	if err != nil {
		http.Error(w, "failed to unblock agent", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// FreezeAllMCPAgentsHandler is the kill-switch: it revokes every spend grant and
// blocks every agent token at once. Agents are restored individually via unblock
// plus a fresh grant.
func FreezeAllMCPAgentsHandler(w http.ResponseWriter, r *http.Request) {
	if err := mcp.FreezeAllAgents(); err != nil {
		http.Error(w, "failed to freeze agents", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ExportMCPAuditHandler downloads the full persisted spend-audit trail as JSON.
func ExportMCPAuditHandler(w http.ResponseWriter, r *http.Request) {
	data, err := mcp.ExportAuditLog()
	if err != nil {
		http.Error(w, "failed to export audit log", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="dcrpulse-mcp-audit.json"`)
	_, _ = w.Write(data)
}

// SetMCPNotifyHandler persists the Bison Relay oversight settings: the on/off
// state and the contact UID that receives approval requests and spend notices.
func SetMCPNotifyHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool   `json:"enabled"`
		Contact string `json:"contact"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	contact := strings.TrimSpace(req.Contact)
	if req.Enabled && contact == "" {
		http.Error(w, "select a Bison Relay contact to enable oversight", http.StatusBadRequest)
		return
	}
	if err := mcp.SetOversightConfig(req.Enabled, contact); err != nil {
		http.Error(w, "failed to save oversight settings", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetMCPLoggingHandler persists the agent-activity logging on/off state.
func SetMCPLoggingHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := mcp.SetLogging(req.Enabled); err != nil {
		http.Error(w, "failed to save logging settings", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
