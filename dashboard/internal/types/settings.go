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
	// ExchangeRates covers dcrpulse's own price lookups and those of the
	// brclientd and bisonw it runs.
	ExchangeRates bool `json:"exchangeRates"`
}

// SaveSettingsResult names the daemons a saved exchange-rates change could
// not be applied to yet.
type SaveSettingsResult struct {
	NotApplied []string `json:"notApplied"`
}

// GlobalSettings is the cross-wallet preferences surface.
type GlobalSettings struct {
	ExternalRequests ExternalRequestSettings `json:"externalRequests"`
	// DecredPulseBotURL is the brulse invite-bot base URL. Empty means the
	// https default; the BRULSE_API_URL env var still overrides it if set.
	DecredPulseBotURL string `json:"decredPulseBotUrl,omitempty"`
}

// SettingsEnvelope is the GET/POST body for /api/wallet/settings.
// Both subsections are independently optional on POST.
type SettingsEnvelope struct {
	Wallet *WalletSettings `json:"wallet,omitempty"`
	Global *GlobalSettings `json:"global,omitempty"`
}

// ChangePassphraseRequest is the body for /api/wallet/settings/change-passphrase.
// DexAppPass authorizes handing the new passphrase to DCRDEX when it is locked.
type ChangePassphraseRequest struct {
	OldPassphrase string `json:"oldPassphrase"`
	NewPassphrase string `json:"newPassphrase"`
	DexAppPass    string `json:"dexAppPass,omitempty"`
}

// DiscoverUsageRequest is the body for /api/wallet/settings/discover-addresses.
// This endpoint runs address discovery only; account discovery is not exposed.
type DiscoverUsageRequest struct {
	GapLimit uint32 `json:"gapLimit,omitempty"`
}
