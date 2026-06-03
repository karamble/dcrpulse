// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

// Keys mirror Decrediton's cfgConstants.* string values
// (decrediton/app/constants/config.js). Only the keys dcrpulse actively
// reads or writes are defined here; round-trip preservation of unknown
// keys is handled by the WalletCfg raw-JSON layer.
const (
	KeyAutobuyerSettings = "autobuyer_settings"
	KeyEnableTicketBuyer = "enable_ticket_buyer"
	KeyRememberedVSPHost = "remembered_vsp_host"
	KeyUsedVSPs          = "used_vsps"
	KeyEnablePrivacy     = "enable_privacy"
	KeyLastAccess        = "last_access"
	KeyIsWatchOnly       = "is_watch_only"
	KeyHiddenAccounts    = "hidden_accounts"
	KeyGapLimit          = "gap_limit"
	KeyDiscoverAccounts  = "discover_accounts"
	KeyMixedAccountCfg   = "mixed_account_cfg"
	KeyChangeAccountCfg  = "change_account_cfg"
	KeyMixedAccBranch    = "mixed_acc_branch"
	KeySendFromUnmixed   = "send_from_unmixed"

	// Global config keys (live in /dashboard-data/config.json).
	KeyAllowedExternalRequests = "allowed_external_requests"

	// KeyThemeStore holds the active theme selection plus any user-created
	// themes ({"schema":1,"activeThemeId":...,"customThemes":[...]}). Shipped
	// themes live in the frontend bundle, so only the active selection and
	// custom themes are persisted here.
	KeyThemeStore = "theme_store"

	// KeySelectedWallet records the active wallet's name across restarts.
	// Empty or absent means no wallet is selected (the UI shows the wallet
	// list). dcrwallet serves one wallet per process, so the active wallet is
	// process-global state, not per-request.
	KeySelectedWallet = "selected_wallet"

	// KeyDcrdexInitialized records that the dcrdex (bisonw) client has been
	// initialized through the dashboard. Not a secret (the app password is
	// never stored); it only distinguishes first-time setup from unlock.
	KeyDcrdexInitialized = "dcrdex_initialized"

	// KeyDcrdexSeedBackedUp records that the user has backed up the dcrdex app
	// seed (or restored from one). dcrdex itself keeps no such flag; set false on
	// a fresh init, true on restore or completed backup, so the unlock nag knows
	// whether to prompt.
	KeyDcrdexSeedBackedUp = "dcrdex_seed_backed_up"

	// Per-wallet record of Politeia vote choices we cast through this
	// dashboard. Map keyed by proposal token, value = "yes"|"no"|"abstain".
	// Mirrors Decrediton's savePiVote local cache so the UI can show
	// "you voted X" without a round-trip to proposals.decred.org for the
	// vote-results endpoint after each cast.
	KeyPoliteiaVotes = "politeia_votes"
)

// External-request identifiers used as keys inside the
// allowed_external_requests map. Names match Decrediton's constants.
// Other Decrediton keys (e.g. "dcrdata", "update_check", "politeia",
// "network_status") survive round-trip via the raw-JSON layer; add a
// constant here when we wire a feature that calls the matching endpoint.
const (
	ExternalRequestVSPListing = "stakepool_listing"
	ExternalRequestPoliteia   = "politeia"
	ExternalRequestBrseeder   = "brseeder"
)
