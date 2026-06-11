// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"context"
	"encoding/json"
	"strconv"
)

func assetArg(assetID uint32) string {
	return strconv.FormatUint(uint64(assetID), 10)
}

// TxHistory returns the wallet's transaction history (raw []WalletTransaction).
// num <= 0 omits the count; refID is a tx-id cursor and requires past to be set.
func (c *Client) TxHistory(ctx context.Context, assetID uint32, num int, refID string, past bool) (json.RawMessage, error) {
	args := []string{assetArg(assetID)}
	if num > 0 || refID != "" {
		args = append(args, strconv.Itoa(num))
	}
	if refID != "" {
		args = append(args, refID, boolArg(past))
	}
	var res json.RawMessage
	err := c.Call(ctx, "txhistory", nil, args, &res)
	return res, err
}

// WalletTx returns a single wallet transaction by id (raw WalletTransaction).
func (c *Client) WalletTx(ctx context.Context, assetID uint32, txID string) (json.RawMessage, error) {
	var res json.RawMessage
	err := c.Call(ctx, "wallettx", nil, []string{assetArg(assetID), txID}, &res)
	return res, err
}

// Send sends value (atomic units) of an asset to an address, returning the coin
// id. Spends real funds; only call on explicit user action.
func (c *Client) Send(ctx context.Context, appPass string, assetID uint32, value uint64, address string) (string, error) {
	var coin string
	err := c.Call(ctx, "send", []string{appPass}, []string{assetArg(assetID), strconv.FormatUint(value, 10), address}, &coin)
	return coin, err
}

// OpenWallet unlocks a wallet for the given asset.
func (c *Client) OpenWallet(ctx context.Context, appPass string, assetID uint32) error {
	return c.Call(ctx, "openwallet", []string{appPass}, []string{assetArg(assetID)}, nil)
}

// CloseWallet locks a wallet for the given asset.
func (c *Client) CloseWallet(ctx context.Context, assetID uint32) error {
	return c.Call(ctx, "closewallet", nil, []string{assetArg(assetID)}, nil)
}

// ToggleWalletStatus enables or disables a wallet.
func (c *Client) ToggleWalletStatus(ctx context.Context, assetID uint32, disable bool) error {
	return c.Call(ctx, "togglewalletstatus", nil, []string{assetArg(assetID), boolArg(disable)}, nil)
}

// RescanWallet triggers a rescan. force is required when the wallet has active
// orders.
func (c *Client) RescanWallet(ctx context.Context, assetID uint32, force bool) error {
	args := []string{assetArg(assetID)}
	if force {
		args = append(args, boolArg(true))
	}
	return c.Call(ctx, "rescanwallet", nil, args, nil)
}

// WalletPeers returns the wallet's configured peers (raw []WalletPeer).
func (c *Client) WalletPeers(ctx context.Context, assetID uint32) (json.RawMessage, error) {
	var res json.RawMessage
	err := c.Call(ctx, "walletpeers", nil, []string{assetArg(assetID)}, &res)
	return res, err
}

// AddWalletPeer adds a persistent peer to the wallet.
func (c *Client) AddWalletPeer(ctx context.Context, assetID uint32, address string) error {
	return c.Call(ctx, "addwalletpeer", nil, []string{assetArg(assetID), address}, nil)
}

// RemoveWalletPeer removes a persistent peer from the wallet.
func (c *Client) RemoveWalletPeer(ctx context.Context, assetID uint32, address string) error {
	return c.Call(ctx, "removewalletpeer", nil, []string{assetArg(assetID), address}, nil)
}
