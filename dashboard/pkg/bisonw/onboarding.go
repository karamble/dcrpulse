// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"context"
	"encoding/json"
)

// Decred asset constants for wallet configuration.
const (
	// AssetDCR is the BIP-44 coin type / DCRDEX asset ID for Decred.
	AssetDCR uint32 = 42
	// WalletTypeDcrwalletRPC selects the external dcrwallet (JSON-RPC) backend.
	WalletTypeDcrwalletRPC = "dcrwalletRPC"
)

// Init initializes the bisonw client with the given app password. seed is
// optional; pass "" to generate a fresh seed. Calling Init on an already
// initialized client returns an error.
func (c *Client) Init(ctx context.Context, appPass, seed string) error {
	params := struct {
		AppPass string  `json:"appPass"`
		Seed    *string `json:"seed,omitempty"`
	}{AppPass: appPass}
	if seed != "" {
		params.Seed = &seed
	}
	return c.Call(ctx, "init", params, nil)
}

// Login unlocks the client and connects to registered DEX servers. It returns
// the raw login result (notifications and per-DEX status).
func (c *Client) Login(ctx context.Context, appPass string) (json.RawMessage, error) {
	var res json.RawMessage
	err := c.Call(ctx, "login", struct {
		AppPass string `json:"appPass"`
	}{appPass}, &res)
	return res, err
}

// Logout locks the client.
func (c *Client) Logout(ctx context.Context) error {
	return c.Call(ctx, "logout", nil, nil)
}

// AppSeed exports the client's app seed (hex) for backup. Requires the app
// password.
func (c *Client) AppSeed(ctx context.Context, appPass string) (string, error) {
	var seed string
	err := c.Call(ctx, "appseed", struct {
		AppPass string `json:"appPass"`
	}{appPass}, &seed)
	return seed, err
}

// NewWalletParams are the parameters for creating a wallet.
type NewWalletParams struct {
	AppPass    string            `json:"appPass"`
	WalletPass string            `json:"walletPass"`
	AssetID    uint32            `json:"assetID"`
	WalletType string            `json:"walletType"`
	Config     map[string]string `json:"config,omitempty"`
}

// NewWallet creates and unlocks a wallet for an asset.
func (c *Client) NewWallet(ctx context.Context, p NewWalletParams) error {
	return c.Call(ctx, "newwallet", p, nil)
}

// DCRWalletRPCConfig holds the connection settings for DCRDEX's external
// dcrwallet (dcrwalletRPC) backend for the Decred asset.
type DCRWalletRPCConfig struct {
	// Account is the dcrwallet account DCRDEX trades from.
	Account string
	// Username and Password are dcrwallet's JSON-RPC credentials.
	Username string
	Password string
	// RPCListen is dcrwallet's JSON-RPC address (host:port).
	RPCListen string
	// RPCCert is the path (inside the bisonw container) to dcrwallet's TLS cert.
	RPCCert string
}

// ConfigMap renders the config as the map[string]string DCRDEX expects.
func (cfg DCRWalletRPCConfig) ConfigMap() map[string]string {
	return map[string]string{
		"account":   cfg.Account,
		"username":  cfg.Username,
		"password":  cfg.Password,
		"rpclisten": cfg.RPCListen,
		"rpccert":   cfg.RPCCert,
	}
}

// NewDCRWallet creates the Decred dcrwalletRPC wallet using the dashboard's
// dcrwallet. walletPass is the dcrwallet account passphrase; appPass is the
// bisonw app password.
func (c *Client) NewDCRWallet(ctx context.Context, appPass, walletPass string, cfg DCRWalletRPCConfig) error {
	return c.NewWallet(ctx, NewWalletParams{
		AppPass:    appPass,
		WalletPass: walletPass,
		AssetID:    AssetDCR,
		WalletType: WalletTypeDcrwalletRPC,
		Config:     cfg.ConfigMap(),
	})
}

// Wallets returns the raw list of configured wallets and their state.
func (c *Client) Wallets(ctx context.Context) (json.RawMessage, error) {
	var res json.RawMessage
	err := c.Call(ctx, "wallets", nil, &res)
	return res, err
}

// Exchanges returns the raw map of known DEX servers and their markets.
func (c *Client) Exchanges(ctx context.Context) (json.RawMessage, error) {
	var res json.RawMessage
	err := c.Call(ctx, "exchanges", nil, &res)
	return res, err
}

// GetDEXConfig fetches a DEX server's configuration before registering. cert is
// optional (path or PEM contents); pass "" for servers with a known cert.
func (c *Client) GetDEXConfig(ctx context.Context, host, cert string) (json.RawMessage, error) {
	var res json.RawMessage
	err := c.Call(ctx, "getdexconfig", struct {
		Host string `json:"host"`
		Cert string `json:"cert,omitempty"`
	}{host, cert}, &res)
	return res, err
}

// DiscoverAccount discovers or restores an account on a DEX server, returning
// true if the account already exists and is paid.
func (c *Client) DiscoverAccount(ctx context.Context, appPass, addr, cert string) (bool, error) {
	var paid bool
	err := c.Call(ctx, "discoveracct", struct {
		AppPass string `json:"appPass"`
		Addr    string `json:"addr"`
		Cert    string `json:"cert,omitempty"`
	}{appPass, addr, cert}, &paid)
	return paid, err
}

// PostBondParams are the parameters for posting a fidelity bond. It mirrors
// DCRDEX's core.PostBondForm.
type PostBondParams struct {
	Host         string  `json:"host"`
	AppPass      string  `json:"appPass"`
	Bond         uint64  `json:"bond"`
	AssetID      *uint32 `json:"assetID,omitempty"`
	LockTime     uint64  `json:"lockTime,omitempty"`
	MaintainTier *bool   `json:"maintainTier,omitempty"`
	Cert         string  `json:"cert,omitempty"`
	FeeBuffer    uint64  `json:"feeBuffer,omitempty"`
	MaxBondedAmt *uint64 `json:"maxBondedAmt,omitempty"`
}

// PostBond posts a fidelity bond to register/maintain a DEX account. The raw
// result holds the bond id and required confirmations.
func (c *Client) PostBond(ctx context.Context, p PostBondParams) (json.RawMessage, error) {
	var res json.RawMessage
	err := c.Call(ctx, "postbond", p, &res)
	return res, err
}
