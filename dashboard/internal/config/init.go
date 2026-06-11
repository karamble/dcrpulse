// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import (
	"fmt"
	"os"
)

// walletInitialDefaults mirrors Decrediton's WALLET_INITIAL_VALUE: seeded
// only when a wallet's config is being created for the first time.
var walletInitialDefaults = map[string]any{
	KeyEnableTicketBuyer: false,
	KeyDiscoverAccounts:  true,
	KeyGapLimit:          200,
}

// InitWalletCfg ensures the wallet directory exists and seeds default
// keys when missing. Existing keys are left untouched. Returns the
// freshly loaded WalletCfg.
func InitWalletCfg(network, walletName string) (*WalletCfg, error) {
	if err := os.MkdirAll(WalletDir(network, walletName), 0o700); err != nil {
		return nil, fmt.Errorf("create wallet dir: %w", err)
	}
	c, err := LoadWalletCfg(network, walletName)
	if err != nil {
		return nil, err
	}
	dirty := false
	for k, v := range walletInitialDefaults {
		if c.Has(k) {
			continue
		}
		if err := c.Set(k, v); err != nil {
			return nil, err
		}
		dirty = true
	}
	if dirty {
		if err := c.Save(); err != nil {
			return nil, err
		}
	}
	return c, nil
}
