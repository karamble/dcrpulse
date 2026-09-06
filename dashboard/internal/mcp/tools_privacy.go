// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"fmt"
	"time"

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
	agentTool("privacy", "privacy_mixer_start",
		"Start the CoinShuffle++ mixer for the active wallet. Requires a spend grant with mixer control enabled; signs with the held wallet passphrase. Privacy must already be configured. Fails if the mixer, autobuyer, or a ticket purchase is already active.",
		func(ctx context.Context, a *agent, _ emptyInput) (any, error) {
			pass, err := grants.authorizeActionPass(a.id, scopePrivacy, time.Now())
			if err != nil {
				recordSpend(a, "privacy_mixer_start", 0, 0, "mixer", "denied", err.Error())
				return nil, err
			}
			defer zero(pass)
			mixed, change, configured, err := services.FindPrivacyAccounts(ctx)
			if err != nil {
				recordSpend(a, "privacy_mixer_start", 0, 0, "mixer", "error", err.Error())
				return nil, err
			}
			if !configured {
				err := fmt.Errorf("privacy not configured: run setup first")
				recordSpend(a, "privacy_mixer_start", 0, 0, "mixer", "error", err.Error())
				return nil, err
			}
			// Mixed branch is the external branch (0), matching the wallet handler.
			if err := services.StartMixer(pass, mixed, 0, change); err != nil {
				recordSpend(a, "privacy_mixer_start", 0, 0, "mixer", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "privacy_mixer_start", 0, 0, "mixer", "ok", "")
			return map[string]any{"ok": true}, nil
		}),
	agentTool("privacy", "privacy_mixer_stop",
		"Stop the CoinShuffle++ mixer for the active wallet. Requires a spend grant with mixer control enabled.",
		func(ctx context.Context, a *agent, _ emptyInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopePrivacy, time.Now()); err != nil {
				recordSpend(a, "privacy_mixer_stop", 0, 0, "mixer", "denied", err.Error())
				return nil, err
			}
			if !services.IsMixerRunning() {
				recordSpend(a, "privacy_mixer_stop", 0, 0, "mixer", "unchanged", "the mixer was not running")
				return map[string]any{"ok": true}, nil
			}
			// StopMixer only signals the mixer goroutine, which relocks the change
			// account on its way out, so this is a request and not a stopped mixer.
			services.StopMixer()
			recordSpend(a, "privacy_mixer_stop", 0, 0, "mixer", "ok", "stop requested")
			return map[string]any{"ok": true}, nil
		}),
}
