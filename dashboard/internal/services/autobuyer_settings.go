// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v4/rpc/walletrpc"
)

// LoadAutobuyerSettings reads the autobuyer config from the per-wallet
// Decrediton-compatible config and translates it to the API shape the
// frontend uses (account number, DCR-denominated threshold, explicit
// VSP host + pubkey). Returns (nil, nil) when nothing is configured.
func LoadAutobuyerSettings(ctx context.Context) (*types.AutobuyerSettings, error) {
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return nil, err
	}
	wc, err := config.LoadWalletCfg(network, CurrentWalletName())
	if err != nil {
		return nil, err
	}
	raw, err := wc.AutobuyerSettings()
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}

	accountNum, err := resolveAccountNumber(ctx, raw.Account)
	if err != nil {
		log.Printf("autobuyer settings: account %q no longer resolves: %v", raw.Account, err)
		return nil, nil
	}

	host := wc.RememberedVSPHost()
	pubkey := ""
	if used, _ := wc.UsedVSPs(); used != nil {
		if v, ok := used[host]; ok {
			pubkey = v.Pubkey
		}
	}

	return &types.AutobuyerSettings{
		Account:           accountNum,
		VspHost:           host,
		VspPubkey:         pubkey,
		BalanceToMaintain: float64(raw.BalanceToMaintain) / 1e8,
	}, nil
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

	if err := wc.SetAutobuyerSettings(&config.AutobuyerSettings{
		BalanceToMaintain: int64(s.BalanceToMaintain * 1e8),
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
	return wc.Save()
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
