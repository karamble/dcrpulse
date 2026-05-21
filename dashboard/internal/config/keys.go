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
)

// External-request identifiers used as keys inside the
// allowed_external_requests map. Names match Decrediton's constants.
// Other Decrediton keys (e.g. "dcrdata", "update_check", "politeia",
// "network_status") survive round-trip via the raw-JSON layer; add a
// constant here when we wire a feature that calls the matching endpoint.
const (
	ExternalRequestVSPListing = "stakepool_listing"
)
