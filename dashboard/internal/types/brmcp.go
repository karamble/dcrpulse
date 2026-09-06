// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// BR-MCP is brclientd's Bison Relay MCP client bridge: agents reach Bison
// Relay tool bots through it and the bridge pays for their tools. These are
// the DCR-denominated shapes the dashboard frontend reads over the REST proxy
// and the agent MCP resource summarises; brclientd itself speaks atoms.

// BRMCPDenied is the bridge listener's most recent allowed-IP denial, passed
// through verbatim (only ever present on replies from brclientd).
type BRMCPDenied struct {
	IP string `json:"ip"`
	At string `json:"at"`
}

// BRMCPSettings is the bridge configuration as the frontend round-trips it.
// The listener address is not here - it is brclientd startup config
// (mcplisten), not a runtime setting.
type BRMCPSettings struct {
	Enabled             bool         `json:"enabled"`
	Token               string       `json:"token"`
	Mode                string       `json:"mode"`
	PerCallCapDcr       float64      `json:"perCallCapDcr"`
	PerDayCapDcr        float64      `json:"perDayCapDcr"`
	AllowedBots         []string     `json:"allowedBots"`
	AllowedIPs          []string     `json:"allowedIps"`
	ApprovalTimeoutSecs int          `json:"approvalTimeoutSecs"`
	TipWaitSecs         int          `json:"tipWaitSecs"`
	LastDenied          *BRMCPDenied `json:"lastDenied,omitempty"`
}

// BRMCPPending is one bot tool payment parked for the operator's approval.
// Created is a Unix timestamp; the payment expires ApprovalTimeoutSecs later.
type BRMCPPending struct {
	ID        string  `json:"id"`
	Bot       string  `json:"bot"`
	Tool      string  `json:"tool"`
	AmountDcr float64 `json:"amountDcr"`
	Created   int64   `json:"created"`
}

// BRMCPSpendEntry is one settled, pending or failed bot tool payment in the
// bridge spend log. TS is a Unix timestamp.
type BRMCPSpendEntry struct {
	TS        int64   `json:"ts"`
	Bot       string  `json:"bot"`
	Tool      string  `json:"tool"`
	Rail      string  `json:"rail"`
	AmountDcr float64 `json:"amountDcr"`
	Status    string  `json:"status,omitempty"`
	Err       string  `json:"err,omitempty"`
}

// BRMCPSpend is the bridge spend log, oldest first as brclientd keeps it, with
// the rolling-day total.
type BRMCPSpend struct {
	Entries  []BRMCPSpendEntry `json:"entries"`
	TodayDcr float64           `json:"todayDcr"`
}
