// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"
)

// spendCapability is the agent-facing view of its spend grant (no passphrase).
// Amounts are DCR. Caps are literal hard limits: there is no unlimited, and a
// 0 cap permits nothing.
type spendCapability struct {
	Granted           bool     `json:"granted"`
	Accounts          []uint32 `json:"accounts,omitempty"`
	PerTxDCR          float64  `json:"perTxDcr"`
	DailyDCR          float64  `json:"dailyDcr"`
	SpentTodayDCR     float64  `json:"spentTodayDcr"`
	RemainingTodayDCR float64  `json:"remainingTodayDcr"`
	Allowlist         []string `json:"allowlist,omitempty"`
	Expiry            string   `json:"expiry,omitempty"`
	AllowVoting       bool     `json:"allowVoting"`
	AllowLightning    bool     `json:"allowLightning"`
	AllowDex          bool     `json:"allowDex"`
	AllowBRWrite      bool     `json:"allowBrWrite"`
	Note              string   `json:"note,omitempty"`
}

// capabilityReport tells an agent what it is allowed to do.
type capabilityReport struct {
	Agent   string          `json:"agent"`
	Domains []string        `json:"domains"`
	Spend   spendCapability `json:"spend"`
	Note    string          `json:"note"`
}

func dcr(atoms int64) float64 { return dcrutil.Amount(atoms).ToCoin() }

// capabilityTools is always registered (node domain is always granted) so every
// agent can introspect its own permissions and remaining spend allowance.
var capabilityTools = []toolDef{
	agentTool("node", "capabilities",
		"Report what this agent may do: its granted capability domains and current spend grant (account scope, caps, remaining daily allowance). Call this to discover your own permissions before attempting actions.",
		func(_ context.Context, a *agent, _ emptyInput) (any, error) {
			rep := capabilityReport{
				Agent:   a.name,
				Domains: sortedDomains(a.domains),
				Note:    "Tools outside your granted domains are not visible. Spending requires a user-granted, account-scoped spend grant; you never receive the wallet passphrase.",
			}
			info, ok := grants.info(a.id)
			if !ok {
				rep.Spend = spendCapability{
					Granted: false,
					Note:    "No spend grant. Ask the user to grant spend access (account and caps) in the dashboard under Settings -> AI Agents.",
				}
				return rep, nil
			}
			spent := info.SpentAtoms
			if time.Since(info.WindowStart) >= grantWindow {
				spent = 0 // the rolling daily window has elapsed
			}
			sc := spendCapability{
				Granted:        true,
				Accounts:       info.Accounts,
				PerTxDCR:       dcr(info.PerTxAtoms),
				DailyDCR:       dcr(info.DailyAtoms),
				SpentTodayDCR:  dcr(spent),
				Allowlist:      info.Allowlist,
				AllowVoting:    info.AllowVoting,
				AllowLightning: info.AllowLightning,
				AllowDex:       info.AllowDex,
				AllowBRWrite:   info.AllowBRWrite,
			}
			rem := info.DailyAtoms - spent
			if rem < 0 {
				rem = 0
			}
			sc.RemainingTodayDCR = dcr(rem)
			if !info.Expiry.IsZero() {
				sc.Expiry = info.Expiry.Format(time.RFC3339)
			}
			rep.Spend = sc
			return rep, nil
		}),
}
