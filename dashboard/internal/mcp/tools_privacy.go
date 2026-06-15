// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/services"
)

// privacyStatusResult is the composed view of the CoinShuffle++ mixer state,
// mirroring what the wallet privacy-status handler reports.
type privacyStatusResult struct {
	Configured    bool   `json:"configured"`
	MixerRunning  bool   `json:"mixerRunning"`
	LastError     string `json:"lastError"`
	MixedAccount  uint32 `json:"mixedAccount"`
	ChangeAccount uint32 `json:"changeAccount"`
}

// privacyTools are the read-only "privacy" domain tools.
var privacyTools = []toolDef{
	readTool("privacy", "privacy_status",
		"Get the CoinShuffle++ mixer status: whether privacy is configured, the mixer running state, mixed/change accounts, and the last mixer error.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			mixed, change, configured, err := services.FindPrivacyAccounts(ctx)
			if err != nil {
				return nil, err
			}
			return privacyStatusResult{
				Configured:    configured,
				MixerRunning:  services.IsMixerRunning(),
				LastError:     services.LastMixerError(),
				MixedAccount:  mixed,
				ChangeAccount: change,
			}, nil
		}),
}
