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

// SetBondOptions updates a DEX account's auto-bond maintenance options. The
// positional bondopts args are [host, targetTier, maxBonded, bondAsset,
// penaltyComps]; a negative value leaves that option unchanged (and the server
// treats penaltyComps 0 as unchanged too). targetTier 0 disables auto-renewal;
// a positive value maintains that tier.
func (c *Client) SetBondOptions(ctx context.Context, host string, targetTier, maxBonded, bondAsset, penaltyComps int) error {
	args := []string{
		host,
		strconv.Itoa(targetTier),
		strconv.Itoa(maxBonded),
		strconv.Itoa(bondAsset),
		strconv.Itoa(penaltyComps),
	}
	return c.Call(ctx, "bondopts", nil, args, nil)
}
