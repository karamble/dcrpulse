// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "dcrpulse/internal/config"

// CurrentWalletName returns the active wallet's name. Hardcoded to
// config.DefaultWalletName while dcrpulse is single-wallet; future
// multi-wallet support changes this to read from the global config or
// an HTTP request header.
func CurrentWalletName() string { return config.DefaultWalletName }
