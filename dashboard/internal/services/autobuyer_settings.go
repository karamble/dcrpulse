// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/dcrutil/v4"
)

// readAutobuyerCfg is the disk half of LoadAutobuyerSettings: the wallet
// config's autobuyer entry, its remembered VSP host and that host's pubkey. A
// seam, since the config path is fixed and a test cannot point it elsewhere.
var readAutobuyerCfg = func(ctx context.Context) (raw *config.AutobuyerSettings, host, pubkey string, err error) {
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return nil, "", "", err
	}
	wc, err := config.LoadWalletCfg(network, CurrentWalletName())
	if err != nil {
		return nil, "", "", err
	}
	raw, err = wc.AutobuyerSettings()
	if err != nil || raw == nil {
		return nil, "", "", err
	}
	host = wc.RememberedVSPHost()
	if used, _ := wc.UsedVSPs(); used != nil {
		if v, ok := used[host]; ok {
			pubkey = v.Pubkey
		}
	}
	return raw, host, pubkey, nil
}

// The settings change only when the operator saves them, a purchase records a
// VSP, or an account is renamed, yet the status route asked the disk and the
// wallet for them on every poll. They are read once per wallet, until one of
// those happens.
var autobuyerSettingsCache struct {
	mu     sync.Mutex
	wallet string
	have   bool
	s      *types.AutobuyerSettings
}

func invalidateAutobuyerSettings() {
	autobuyerSettingsCache.mu.Lock()
	autobuyerSettingsCache.have = false
	autobuyerSettingsCache.s = nil
	autobuyerSettingsCache.mu.Unlock()
}

// LoadAutobuyerSettings reads the autobuyer config from the per-wallet
// Decrediton-compatible config and translates it to the API shape the
// frontend uses (account number, DCR-denominated threshold, explicit
// VSP host + pubkey). Returns (nil, nil) when nothing is configured.
func LoadAutobuyerSettings(ctx context.Context) (*types.AutobuyerSettings, error) {
	wallet := CurrentWalletName()
	autobuyerSettingsCache.mu.Lock()
	if autobuyerSettingsCache.have && autobuyerSettingsCache.wallet == wallet {
		s := autobuyerSettingsCache.s
		autobuyerSettingsCache.mu.Unlock()
		return copyAutobuyerSettings(s), nil
	}
	autobuyerSettingsCache.mu.Unlock()

	raw, host, pubkey, err := readAutobuyerCfg(ctx)
	if err != nil {
		return nil, err
	}
	var s *types.AutobuyerSettings
	if raw != nil {
		accountNum, err := resolveAccountNumber(ctx, raw.Account)
		if err != nil {
			stkeLog.Warnf("autobuyer settings: account %q no longer resolves: %v", raw.Account, err)
			return nil, nil // not cached: the wallet may just be unreachable
		}
		s = &types.AutobuyerSettings{
			Account:           accountNum,
			VspHost:           host,
			VspPubkey:         pubkey,
			BalanceToMaintain: dcrutil.Amount(raw.BalanceToMaintain).ToCoin(),
		}
	}
	autobuyerSettingsCache.mu.Lock()
	autobuyerSettingsCache.wallet, autobuyerSettingsCache.have, autobuyerSettingsCache.s = wallet, true, s
	autobuyerSettingsCache.mu.Unlock()
	return copyAutobuyerSettings(s), nil
}

// copyAutobuyerSettings hands callers their own value, so nothing mutates the
// cached one.
func copyAutobuyerSettings(s *types.AutobuyerSettings) *types.AutobuyerSettings {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}

// SaveAutobuyerSettings translates the API shape into Decrediton's
// wallet-config schema and persists it.
func SaveAutobuyerSettings(ctx context.Context, s *types.AutobuyerSettings) error {
	if s == nil {
		return fmt.Errorf("nil settings")
	}
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return err
	}
	wc, err := config.LoadWalletCfg(network, CurrentWalletName())
	if err != nil {
		return err
	}

	accountName, err := resolveAccountName(ctx, s.Account)
	if err != nil {
		return fmt.Errorf("resolve account: %w", err)
	}

	balanceAtoms, err := dcrutil.NewAmount(s.BalanceToMaintain)
	if err != nil {
		return fmt.Errorf("balance to maintain: %w", err)
	}

	if err := wc.SetAutobuyerSettings(&config.AutobuyerSettings{
		BalanceToMaintain: int64(balanceAtoms),
		Account:           accountName,
		MaxFeePercentage:  10,
	}); err != nil {
		return err
	}
	if err := wc.SetRememberedVSPHost(s.VspHost); err != nil {
		return err
	}
	if err := wc.UpsertUsedVSP(config.VSPMetadata{
		Host:     s.VspHost,
		Pubkey:   s.VspPubkey,
		LastUsed: time.Now().Unix(),
	}); err != nil {
		return err
	}
	if err := wc.Save(); err != nil {
		return err
	}
	invalidateAutobuyerSettings()
	return nil
}

// resolveAccountNumber returns the account number for the given name by
// querying the wallet's gRPC Accounts list.
func resolveAccountNumber(ctx context.Context, name string) (uint32, error) {
	if rpc.WalletGrpcClient == nil {
		return 0, fmt.Errorf("wallet gRPC client unavailable")
	}
	resp, err := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{})
	if err != nil {
		return 0, err
	}
	for _, a := range resp.Accounts {
		if a.AccountName == name {
			return a.AccountNumber, nil
		}
	}
	return 0, fmt.Errorf("account %q not found", name)
}

// resolveAccountName is the reverse lookup.
func resolveAccountName(ctx context.Context, num uint32) (string, error) {
	if rpc.WalletGrpcClient == nil {
		return "", fmt.Errorf("wallet gRPC client unavailable")
	}
	resp, err := rpc.WalletGrpcClient.Accounts(ctx, &pb.AccountsRequest{})
	if err != nil {
		return "", err
	}
	for _, a := range resp.Accounts {
		if a.AccountNumber == num {
			return a.AccountName, nil
		}
	}
	return "", fmt.Errorf("account %d not found", num)
}
