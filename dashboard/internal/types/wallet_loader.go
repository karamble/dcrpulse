// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// WalletExistsResponse indicates whether a wallet database exists
type WalletExistsResponse struct {
	Exists bool `json:"exists"`
}

// GenerateSeedRequest contains parameters for seed generation
type GenerateSeedRequest struct {
	SeedLength uint32 `json:"seedLength,omitempty"` // Optional, defaults to 33
}

// GenerateSeedResponse contains the generated seed in multiple formats
type GenerateSeedResponse struct {
	SeedMnemonic string `json:"seedMnemonic"` // 33-word mnemonic phrase
	SeedHex      string `json:"seedHex"`      // Hex-encoded seed
}

// CreateWalletRequest contains parameters for wallet creation
type CreateWalletRequest struct {
	PublicPassphrase  string `json:"publicPassphrase"`  // Optional: Encrypts wallet database for viewing
	PrivatePassphrase string `json:"privatePassphrase"` // Required: Encrypts private keys for spending
	SeedHex           string `json:"seedHex"`           // Required: Hex-encoded seed
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
