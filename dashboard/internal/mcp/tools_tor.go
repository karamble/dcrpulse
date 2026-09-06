// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// torSetSettingsInput mirrors the settable fields of types.TorSettings. Rev is
// owned by the writer (bumped so the supervisors relaunch their daemons) and is
// not accepted from the agent.
// applyTorSettings overlays the named fields onto the current settings and
// reports which were named. An omitted field keeps its current value, so a
// caller changing one setting cannot revert another it read earlier, and
// lnOnion - which this tool does not expose - is never written.
func applyTorSettings(cur types.TorSettings, in torSetSettingsInput) (types.TorSettings, string) {
	next := cur
	var named []string
	flag := func(name string, dst, src *bool) {
		if src != nil {
			*dst = *src
			named = append(named, fmt.Sprintf("%s=%t", name, *src))
		}
	}
	flag("enabled", &next.Enabled, in.Enabled)
	flag("isolation", &next.Isolation, in.Isolation)
	flag("dcrdOnion", &next.DcrdOnion, in.DcrdOnion)
	if in.CircuitLimit != nil {
		next.CircuitLimit = *in.CircuitLimit
		named = append(named, fmt.Sprintf("circuitLimit=%d", *in.CircuitLimit))
	}
	return next, strings.Join(named, " ")
}

// torSettingsChanges names the fields that actually differ, so the audit row
// reports what was written rather than what was asked for: the writer clamps an
// out-of-range circuit limit. Rev is the writer's own and never reported.
func torSettingsChanges(prev, next types.TorSettings) string {
	var named []string
	flag := func(name string, was, now bool) {
		if was != now {
			named = append(named, fmt.Sprintf("%s=%t", name, now))
		}
	}
	flag("enabled", prev.Enabled, next.Enabled)
	flag("isolation", prev.Isolation, next.Isolation)
	flag("dcrdOnion", prev.DcrdOnion, next.DcrdOnion)
	if prev.CircuitLimit != next.CircuitLimit {
		named = append(named, fmt.Sprintf("circuitLimit=%d", next.CircuitLimit))
	}
	if len(named) == 0 {
		return "the request was clamped; nothing changed"
	}
	return strings.Join(named, " ")
}

type torSetSettingsInput struct {
	Enabled      *bool `json:"enabled,omitempty" jsonschema:"route the stack's outbound traffic through Tor; omit to leave unchanged"`
	Isolation    *bool `json:"isolation,omitempty" jsonschema:"use stream isolation (a separate circuit per daemon); omit to leave unchanged"`
	DcrdOnion    *bool `json:"dcrdOnion,omitempty" jsonschema:"publish dcrd as an onion service; omit to leave unchanged"`
	CircuitLimit *int  `json:"circuitLimit,omitempty" jsonschema:"maximum number of Tor circuits (1-1000); omit to leave unchanged"`
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
		"Update the Tor configuration (routing, stream isolation, dcrd onion service, circuit limit). Every field is optional: omit one to leave it unchanged. A change bumps the revision so every daemon relaunches with the new flags. Requires a grant with Tor write enabled. Returns the settings.",
		func(_ context.Context, a *agent, in torSetSettingsInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeTor, time.Now()); err != nil {
				recordSpend(a, "tor_set_settings", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			cur := services.ReadTorSettings()
			next, named := applyTorSettings(cur, in)
			// A write bumps Rev and relaunches every daemon, so a call that
			// changes nothing does nothing.
			if next == cur {
				detail := "no fields given"
				if named != "" {
					detail = named + " already set"
				}
				recordSpend(a, "tor_set_settings", 0, 0, "", "unchanged", detail)
				return cur, nil
			}
			out, err := services.WriteTorSettings(next)
			if err != nil {
				recordSpend(a, "tor_set_settings", 0, 0, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "tor_set_settings", 0, 0, "", "ok", torSettingsChanges(cur, out))
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
