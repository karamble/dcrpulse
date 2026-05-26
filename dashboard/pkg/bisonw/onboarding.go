// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"context"
	"encoding/json"
	"strconv"
)

// Decred asset constants for wallet configuration.
const (
	// AssetDCR is the BIP-44 coin type / DCRDEX asset ID for Decred.
	AssetDCR uint32 = 42
	// WalletTypeDcrwalletRPC selects the external dcrwallet (JSON-RPC) backend.
	WalletTypeDcrwalletRPC = "dcrwalletRPC"
)

// Init initializes the bisonw client with the given app password. seed is
// optional (hex-encoded restoration seed); pass "" to generate a fresh seed.
// Calling Init on an already initialized client returns an error.
func (c *Client) Init(ctx context.Context, appPass, seed string) error {
	var args []string
	if seed != "" {
		args = []string{seed}
	}
	return c.Call(ctx, "init", []string{appPass}, args, nil)
}

// Login unlocks the client and connects to registered DEX servers. It returns
// the raw login result (notifications and per-DEX status).
func (c *Client) Login(ctx context.Context, appPass string) (json.RawMessage, error) {
	var res json.RawMessage
	err := c.Call(ctx, "login", []string{appPass}, nil, &res)
	return res, err
}

// Logout locks the client.
func (c *Client) Logout(ctx context.Context) error {
	return c.Call(ctx, "logout", nil, nil, nil)
}

// AppSeed exports the client's app seed (hex) for backup. Requires the app
// password.
func (c *Client) AppSeed(ctx context.Context, appPass string) (string, error) {
	var seed string
	err := c.Call(ctx, "appseed", []string{appPass}, nil, &seed)
	return seed, err
}

// NewWalletParams are the parameters for creating a wallet.
type NewWalletParams struct {
	AppPass    string
	WalletPass string
	AssetID    uint32
	WalletType string
	Config     map[string]string
}

// NewWallet creates and unlocks a wallet for an asset.
func (c *Client) NewWallet(ctx context.Context, p NewWalletParams) error {
	args := []string{strconv.FormatUint(uint64(p.AssetID), 10), p.WalletType}
	if len(p.Config) > 0 {
		cfgJSON, err := json.Marshal(p.Config)
		if err != nil {
			return err
		}
		// Args[2] is an (unused) INI config blob; Args[3] is a JSON config map.
		args = append(args, "", string(cfgJSON))
	}
	return c.Call(ctx, "newwallet", []string{p.AppPass, p.WalletPass}, args, nil)
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
	err := c.Call(ctx, "wallets", nil, nil, &res)
	return res, err
}

// HasWallet reports whether a wallet is configured for the given asset ID.
func (c *Client) HasWallet(ctx context.Context, assetID uint32) (bool, error) {
	raw, err := c.Wallets(ctx)
	if err != nil {
		return false, err
	}
	var states []struct {
		AssetID uint32 `json:"assetID"`
	}
	if err := json.Unmarshal(raw, &states); err != nil {
		return false, err
	}
	for _, s := range states {
		if s.AssetID == assetID {
			return true, nil
		}
	}
	return false, nil
}

// Exchanges returns the raw map of known DEX servers and their markets.
func (c *Client) Exchanges(ctx context.Context) (json.RawMessage, error) {
	var res json.RawMessage
	err := c.Call(ctx, "exchanges", nil, nil, &res)
	return res, err
}

// GetDEXConfig fetches a DEX server's configuration before registering. cert is
// optional (PEM contents); pass "" for servers with a built-in cert.
func (c *Client) GetDEXConfig(ctx context.Context, host, cert string) (json.RawMessage, error) {
	args := []string{host}
	if cert != "" {
		args = append(args, cert)
	}
	var res json.RawMessage
	err := c.Call(ctx, "getdexconfig", nil, args, &res)
	return res, err
}

// DiscoverAccount discovers or restores an account on a DEX server, returning
// true if the account already exists and is paid.
func (c *Client) DiscoverAccount(ctx context.Context, appPass, addr, cert string) (bool, error) {
	args := []string{addr}
	if cert != "" {
		args = append(args, cert)
	}
	var paid bool
	err := c.Call(ctx, "discoveracct", []string{appPass}, args, &paid)
	return paid, err
}

// PostBondParams are the parameters for posting a fidelity bond on v1.0.6.
type PostBondParams struct {
	AppPass      string
	Host         string
	Bond         uint64
	AssetID      uint32 // 0 defaults to AssetDCR (42)
	MaintainTier *bool
	Cert         string
}

// PostBond posts a fidelity bond to register/maintain a DEX account. The raw
// result holds the bond id and required confirmations.
func (c *Client) PostBond(ctx context.Context, p PostBondParams) (json.RawMessage, error) {
	asset := p.AssetID
	if asset == 0 {
		asset = AssetDCR
	}
	args := []string{p.Host, strconv.FormatUint(p.Bond, 10), strconv.FormatUint(uint64(asset), 10)}
	if p.MaintainTier != nil || p.Cert != "" {
		maintain := "true"
		if p.MaintainTier != nil && !*p.MaintainTier {
			maintain = "false"
		}
		args = append(args, maintain)
		if p.Cert != "" {
			args = append(args, p.Cert)
		}
	}
	var res json.RawMessage
	err := c.Call(ctx, "postbond", []string{p.AppPass}, args, &res)
	return res, err
}
