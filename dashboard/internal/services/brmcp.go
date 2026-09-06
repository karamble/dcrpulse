// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
)

// Readers for brclientd's BR-MCP client engine (bot tool payments). brclientd
// speaks atoms; the dashboard and the agent MCP resource speak DCR, converted
// here via dcrutil. The decoders are pure so the REST proxy and the resource
// share one reading of the wire, and so they can be tested without a daemon.

// BRMCPSettingsWire mirrors brclientd's mcpclient.json shape. The listener
// address is not here - it is brclientd startup config (mcplisten), not a
// runtime setting.
type BRMCPSettingsWire struct {
	Enabled             bool               `json:"enabled"`
	Token               string             `json:"token"`
	Mode                string             `json:"mode"`
	PerCallCapAtoms     int64              `json:"per_call_cap_atoms"`
	PerDayCapAtoms      int64              `json:"per_day_cap_atoms"`
	AllowedBots         []string           `json:"allowed_bots"`
	AllowedIPs          []string           `json:"allowed_ips"`
	ApprovalTimeoutSecs int                `json:"approval_timeout_secs"`
	TipWaitSecs         int                `json:"tip_wait_secs"`
	LastDenied          *types.BRMCPDenied `json:"last_denied,omitempty"`
}

// BRMCPSettingsToView converts the daemon's settings to the DCR-denominated
// frontend shape. The list fields are never nil so they marshal as [].
func BRMCPSettingsToView(w BRMCPSettingsWire) types.BRMCPSettings {
	if w.AllowedBots == nil {
		w.AllowedBots = []string{}
	}
	if w.AllowedIPs == nil {
		w.AllowedIPs = []string{}
	}
	return types.BRMCPSettings{
		Enabled:             w.Enabled,
		Token:               w.Token,
		Mode:                w.Mode,
		PerCallCapDcr:       dcrutil.Amount(w.PerCallCapAtoms).ToCoin(),
		PerDayCapDcr:        dcrutil.Amount(w.PerDayCapAtoms).ToCoin(),
		AllowedBots:         w.AllowedBots,
		AllowedIPs:          w.AllowedIPs,
		ApprovalTimeoutSecs: w.ApprovalTimeoutSecs,
		TipWaitSecs:         w.TipWaitSecs,
		LastDenied:          w.LastDenied,
	}
}

// DecodeBRMCPSettings parses a /settings/mcpclient reply.
func DecodeBRMCPSettings(raw json.RawMessage) (types.BRMCPSettings, error) {
	var wire BRMCPSettingsWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return types.BRMCPSettings{}, fmt.Errorf("parse settings: %w", err)
	}
	return BRMCPSettingsToView(wire), nil
}

// DecodeBRMCPPending parses a /mcp/pending reply, oldest first as brclientd
// lists it. The result is never nil.
func DecodeBRMCPPending(raw json.RawMessage) ([]types.BRMCPPending, error) {
	var wire struct {
		Pending []struct {
			ID      string `json:"id"`
			Bot     string `json:"bot"`
			Tool    string `json:"tool"`
			Atoms   int64  `json:"atoms"`
			Created int64  `json:"created"`
		} `json:"pending"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("parse pending: %w", err)
	}
	out := make([]types.BRMCPPending, 0, len(wire.Pending))
	for _, p := range wire.Pending {
		out = append(out, types.BRMCPPending{
			ID: p.ID, Bot: p.Bot, Tool: p.Tool,
			AmountDcr: dcrutil.Amount(p.Atoms).ToCoin(),
			Created:   p.Created,
		})
	}
	return out, nil
}

// DecodeBRMCPSpend parses a /mcp/spend reply. Entries is never nil.
func DecodeBRMCPSpend(raw json.RawMessage) (types.BRMCPSpend, error) {
	var wire struct {
		Entries []struct {
			TS     int64  `json:"ts"`
			Bot    string `json:"bot"`
			Tool   string `json:"tool"`
			Rail   string `json:"rail"`
			Atoms  int64  `json:"atoms"`
			Status string `json:"status"`
			Err    string `json:"err"`
		} `json:"entries"`
		TodayAtoms int64 `json:"today_atoms"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return types.BRMCPSpend{}, fmt.Errorf("parse spend: %w", err)
	}
	out := types.BRMCPSpend{
		Entries:  make([]types.BRMCPSpendEntry, 0, len(wire.Entries)),
		TodayDcr: dcrutil.Amount(wire.TodayAtoms).ToCoin(),
	}
	for _, e := range wire.Entries {
		out.Entries = append(out.Entries, types.BRMCPSpendEntry{
			TS: e.TS, Bot: e.Bot, Tool: e.Tool, Rail: e.Rail,
			AmountDcr: dcrutil.Amount(e.Atoms).ToCoin(),
			Status:    e.Status, Err: e.Err,
		})
	}
	return out, nil
}

// FetchBRMCPSettings reads the bridge settings from brclientd.
func FetchBRMCPSettings(ctx context.Context) (types.BRMCPSettings, error) {
	raw, err := rpc.BrclientdMCPSettings(ctx)
	if err != nil {
		return types.BRMCPSettings{}, err
	}
	return DecodeBRMCPSettings(raw)
}

// FetchBRMCPPending reads the payments awaiting the operator's approval.
func FetchBRMCPPending(ctx context.Context) ([]types.BRMCPPending, error) {
	raw, err := rpc.BrclientdMCPPending(ctx)
	if err != nil {
		return nil, err
	}
	return DecodeBRMCPPending(raw)
}

// FetchBRMCPSpend reads the bridge spend log and rolling-day total.
func FetchBRMCPSpend(ctx context.Context) (types.BRMCPSpend, error) {
	raw, err := rpc.BrclientdMCPSpend(ctx)
	if err != nil {
		return types.BRMCPSpend{}, err
	}
	return DecodeBRMCPSpend(raw)
}
