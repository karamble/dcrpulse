// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// WalletExistsResponse indicates whether a wallet database exists
type WalletExistsResponse struct {
	Exists bool `json:"exists"`
}

// WalletLoadedResponse indicates whether a wallet is currently loaded and ready
type WalletLoadedResponse struct {
	Loaded bool   `json:"loaded"`
	Error  string `json:"error,omitempty"`
}

// GenerateSeedRequest contains parameters for seed generation.
// SeedLength is in BYTES (not words). Zero or unset -> dcrwallet's
// recommended 32 bytes -> 33-word mnemonic (Decred standard).
type GenerateSeedRequest struct {
	SeedLength uint32 `json:"seedLength,omitempty"`
}

// GenerateSeedResponse contains the generated seed in multiple formats
type GenerateSeedResponse struct {
	SeedMnemonic string `json:"seedMnemonic"` // 33-word mnemonic phrase
	SeedHex      string `json:"seedHex"`      // Hex-encoded seed
}

// DecodeSeedRequest carries a user-supplied seed for validation.
// UserInput accepts either a 33-word mnemonic OR a hex string.
type DecodeSeedRequest struct {
	UserInput string `json:"userInput"`
}

// DecodeSeedResponse carries the decoded hex seed.
type DecodeSeedResponse struct {
	SeedHex string `json:"seedHex"`
}

// CreateWalletRequest contains parameters for wallet creation
type CreateWalletRequest struct {
	PublicPassphrase  string `json:"publicPassphrase"`  // Optional: Encrypts wallet database for viewing
	PrivatePassphrase string `json:"privatePassphrase"` // Required: Encrypts private keys for spending
	SeedHex           string `json:"seedHex"`           // Required: Hex-encoded seed
	DiscoverAccounts  bool   `json:"discoverAccounts"`  // True when restoring from an existing seed; enables post-create chain rescan
}

// CreateWalletResponse indicates wallet creation success
type CreateWalletResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// OpenWalletRequest contains parameters for opening a wallet
type OpenWalletRequest struct {
	PublicPassphrase string `json:"publicPassphrase"` // Optional: Wallet database passphrase (empty if wallet created without one)
}

// OpenWalletResponse indicates wallet open success
type OpenWalletResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}
