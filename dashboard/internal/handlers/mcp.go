// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"dcrpulse/internal/mcp"
)

const maxAgentNameLen = 64

// mcpSettingsResponse describes the MCP server state for the Settings -> Agents
// panel: whether the listener is enabled and where, the capability domains that
// can be granted, the saved agent roster, and the live sessions.
type mcpSettingsResponse struct {
	Enabled  bool            `json:"enabled"`
	Bind     string          `json:"bind"`
	Port     string          `json:"port"`
	Domains  []string        `json:"domains"`
	Agents   []mcp.AgentInfo `json:"agents"`
	Sessions []mcp.Session   `json:"sessions"`
}

// MCPSettingsHandler returns the live MCP listener state plus the agent roster
// and live sessions.
func MCPSettingsHandler(w http.ResponseWriter, r *http.Request) {
	running, bind, port := mcp.Status()
	resp := mcpSettingsResponse{
		Enabled:  running,
		Bind:     bind,
		Port:     port,
		Domains:  mcp.Domains(),
		Agents:   mcp.ListAgents(),
		Sessions: mcp.ActiveSessions(),
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
