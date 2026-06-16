// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"time"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// torSetSettingsInput mirrors the settable fields of types.TorSettings. Rev is
// owned by the writer (bumped so the supervisors relaunch their daemons) and is
// not accepted from the agent.
type torSetSettingsInput struct {
	Enabled      bool `json:"enabled" jsonschema:"route the stack's outbound traffic through Tor"`
	Isolation    bool `json:"isolation" jsonschema:"use stream isolation (a separate circuit per daemon)"`
	DcrdOnion    bool `json:"dcrdOnion" jsonschema:"publish dcrd as an onion service"`
	CircuitLimit int  `json:"circuitLimit" jsonschema:"maximum number of Tor circuits (1-1000)"`
}

// torTools are the "tor" domain tools. The read snapshot getters do not return
// an error, so they are wrapped with a nil error; the write tools are gated on
// the Tor write scope.
var torTools = []toolDef{
	readTool("tor", "tor_status",
		"Get the Tor proxy reachability and per-daemon routing state.",
		func(_ context.Context, _ emptyInput) (any, error) { return services.TorStatusSnapshot(), nil }),
	readTool("tor", "tor_control",
		"Get live Tor control-port data (bootstrap percentage, circuits, traffic, version).",
		func(_ context.Context, _ emptyInput) (any, error) { return services.TorControlSnapshot(), nil }),
	readTool("tor", "tor_settings",
		"Get the current Tor configuration (enabled, stream isolation, onion service, circuit limit).",
		func(_ context.Context, _ emptyInput) (any, error) { return services.ReadTorSettings(), nil }),
	agentTool("tor", "tor_set_settings",
		"Update the Tor configuration (enable/disable routing, stream isolation, dcrd onion service, circuit limit). Bumps the revision so every daemon relaunches with the new flags. Requires a grant with Tor write enabled. Returns the updated settings.",
		func(_ context.Context, a *agent, in torSetSettingsInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeTor, time.Now()); err != nil {
				recordSpend(a, "tor_set_settings", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			out, err := services.WriteTorSettings(types.TorSettings{
				Enabled:      in.Enabled,
				Isolation:    in.Isolation,
				DcrdOnion:    in.DcrdOnion,
				CircuitLimit: in.CircuitLimit,
			})
			if err != nil {
				recordSpend(a, "tor_set_settings", 0, 0, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "tor_set_settings", 0, 0, "", "ok", "")
			return out, nil
		}),
	agentTool("tor", "tor_new_identity",
		"Signal Tor to build fresh circuits (NEWNYM), rotating the exit identity. Requires a grant with Tor write enabled.",
		func(_ context.Context, a *agent, _ emptyInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeTor, time.Now()); err != nil {
				recordSpend(a, "tor_new_identity", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			if err := services.TorNewIdentity(); err != nil {
				recordSpend(a, "tor_new_identity", 0, 0, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "tor_new_identity", 0, 0, "", "ok", "")
			return map[string]any{"ok": true}, nil
		}),
}
