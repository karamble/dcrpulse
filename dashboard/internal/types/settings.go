// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// WalletSettings is the per-wallet preferences surface exposed via
// /api/wallet/settings.
type WalletSettings struct {
	GapLimit        int    `json:"gapLimit"`
	CurrencyDisplay string `json:"currencyDisplay,omitempty"`
}

// ExternalRequestSettings is the global allowlist for outbound HTTP
// calls the dashboard makes.
type ExternalRequestSettings struct {
	VSPListing bool `json:"vspListing"`
	Politeia   bool `json:"politeia"`
	Brseeder   bool `json:"brseeder"`
}

// GlobalSettings is the cross-wallet preferences surface.
type GlobalSettings struct {
	ExternalRequests ExternalRequestSettings `json:"externalRequests"`
}

// SettingsEnvelope is the GET/POST body for /api/wallet/settings.
// Both subsections are independently optional on POST.
type SettingsEnvelope struct {
	Wallet *WalletSettings `json:"wallet,omitempty"`
	Global *GlobalSettings `json:"global,omitempty"`
}

// ChangePassphraseRequest is the body for /api/wallet/settings/change-passphrase.
type ChangePassphraseRequest struct {
	OldPassphrase string `json:"oldPassphrase"`
	NewPassphrase string `json:"newPassphrase"`
}

// DiscoverUsageRequest is the body for /api/wallet/settings/discover-addresses.
// This endpoint runs address discovery only; account discovery is not exposed.
type DiscoverUsageRequest struct {
	Passphrase string `json:"passphrase"`
	GapLimit   uint32 `json:"gapLimit,omitempty"`
}
