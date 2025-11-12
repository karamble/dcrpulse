// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v4/rpc/walletrpc"
)

// CheckWalletExists checks if a wallet database exists
func CheckWalletExists(ctx context.Context) (*types.WalletExistsResponse, error) {
	if rpc.WalletLoaderClient == nil {
		return nil, fmt.Errorf("wallet loader client not initialized")
	}

	req := &pb.WalletExistsRequest{}
	resp, err := rpc.WalletLoaderClient.WalletExists(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to check wallet existence: %w", err)
	}

	return &types.WalletExistsResponse{
		Exists: resp.Exists,
	}, nil
}

// GenerateSeed generates a new cryptographically secure seed
func GenerateSeed(ctx context.Context, seedLength uint32) (*types.GenerateSeedResponse, error) {
	if rpc.SeedServiceClient == nil {
		return nil, fmt.Errorf("seed service client not initialized")
	}

	// Default to 33-word seed (264 bits) if not specified
	if seedLength == 0 {
		seedLength = 33
	}

	req := &pb.GenerateRandomSeedRequest{
		SeedLength: seedLength,
	}

	resp, err := rpc.SeedServiceClient.GenerateRandomSeed(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate seed: %w", err)
	}

	log.Printf("Generated seed mnemonic with %d words", seedLength)

	return &types.GenerateSeedResponse{
		SeedMnemonic: resp.SeedMnemonic,
		SeedHex:      resp.SeedHex,
	}, nil
}

// CreateNewWallet creates a new wallet with the provided passphrases and seed
func CreateNewWallet(ctx context.Context, publicPass, privatePass, seedHex string) error {
	if rpc.WalletLoaderClient == nil {
		return fmt.Errorf("wallet loader client not initialized")
	}

	// Decode seed hex to bytes
	seedBytes, err := hex.DecodeString(seedHex)
	if err != nil {
		return fmt.Errorf("invalid seed hex: %w", err)
	}

	log.Printf("Creating wallet with seed length: %d bytes", len(seedBytes))

	req := &pb.CreateWalletRequest{
		PublicPassphrase:  []byte(publicPass),
		PrivatePassphrase: []byte(privatePass),
		Seed:              seedBytes,
	}

	_, err = rpc.WalletLoaderClient.CreateWallet(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create wallet: %w", err)
	}

	log.Println("Wallet created and opened successfully")

	// Start RPC sync with dcrd in background
	// Use background context so it doesn't get canceled when HTTP request completes
	go func() {
		bgCtx := context.Background()

		// Read dcrd certificate
		var cert []byte
		if rpc.DcrdConfig.RPCCert != "" {
			var err error
			cert, err = os.ReadFile(rpc.DcrdConfig.RPCCert)
			if err != nil {
				log.Printf("Failed to read dcrd cert for RPC sync: %v", err)
				return
			}
		}

		// Build network address
		networkAddr := fmt.Sprintf("%s:%s", rpc.DcrdConfig.RPCHost, rpc.DcrdConfig.RPCPort)

		rpcSyncReq := &pb.RpcSyncRequest{
			NetworkAddress:    networkAddr,
			Username:          rpc.DcrdConfig.RPCUser,
			Password:          []byte(rpc.DcrdConfig.RPCPassword),
			Certificate:       cert,
			DiscoverAccounts:  false,    // Don't discover on new wallet (no transactions yet)
			PrivatePassphrase: []byte{}, // Not needed without account discovery
		}

		stream, err := rpc.WalletLoaderClient.RpcSync(bgCtx, rpcSyncReq)
		if err != nil {
			log.Printf("Failed to start RPC sync: %v", err)
			return
		}

		log.Printf("RPC sync started with dcrd at %s", networkAddr)

		// Consume the stream to keep sync active
		// If stream fails, it will be restarted on next wallet open/operation
		for {
			_, err := stream.Recv()
			if err != nil {
				log.Printf("RPC sync stream ended: %v (sync will resume on wallet reopen)", err)
				return
			}
		}
	}()

	// Give the sync a moment to start
	time.Sleep(100 * time.Millisecond)

	return nil
}

// OpenWallet opens an existing wallet with the provided public passphrase
func OpenWallet(ctx context.Context, publicPass string) error {
	if rpc.WalletLoaderClient == nil {
		return fmt.Errorf("wallet loader client not initialized")
	}

	log.Println("Opening wallet...")

	req := &pb.OpenWalletRequest{
		PublicPassphrase: []byte(publicPass),
	}

	_, err := rpc.WalletLoaderClient.OpenWallet(ctx, req)
	if err != nil {
		// Check if wallet is already opened
		if strings.Contains(err.Error(), "already opened") {
			log.Println("Wallet already opened")
		} else {
			return fmt.Errorf("failed to open wallet: %w", err)
		}
	} else {
		log.Println("Wallet opened successfully")
	}

	// Start RPC sync with dcrd in background
	// Use background context so it doesn't get canceled when HTTP request completes
	go func() {
		bgCtx := context.Background()

		// Read dcrd certificate
		var cert []byte
		if rpc.DcrdConfig.RPCCert != "" {
			var err error
			cert, err = os.ReadFile(rpc.DcrdConfig.RPCCert)
			if err != nil {
				log.Printf("Failed to read dcrd cert for RPC sync: %v", err)
				return
			}
		}

		// Build network address
		networkAddr := fmt.Sprintf("%s:%s", rpc.DcrdConfig.RPCHost, rpc.DcrdConfig.RPCPort)

		rpcSyncReq := &pb.RpcSyncRequest{
			NetworkAddress:    networkAddr,
			Username:          rpc.DcrdConfig.RPCUser,
			Password:          []byte(rpc.DcrdConfig.RPCPassword),
			Certificate:       cert,
			DiscoverAccounts:  false, // Don't need to discover on reopening
			PrivatePassphrase: []byte{},
		}

		stream, err := rpc.WalletLoaderClient.RpcSync(bgCtx, rpcSyncReq)
		if err != nil {
			// RpcSync might already be running, which is okay
			if !strings.Contains(err.Error(), "already") {
				log.Printf("Warning: Failed to start RPC sync (may already be syncing): %v", err)
			}
			return
		}

		log.Printf("RPC sync started/resumed with dcrd at %s", networkAddr)

		// Consume stream to keep sync active
		for {
			_, err := stream.Recv()
			if err != nil {
				log.Printf("RPC sync stream ended: %v", err)
				return
			}
		}
	}()

	// Give the sync a moment to start
	time.Sleep(100 * time.Millisecond)

	return nil
}

// CloseWallet closes the currently open wallet
func CloseWallet(ctx context.Context) error {
	if rpc.WalletLoaderClient == nil {
		return fmt.Errorf("wallet loader client not initialized")
	}

	log.Println("Closing wallet...")

	req := &pb.CloseWalletRequest{}
	_, err := rpc.WalletLoaderClient.CloseWallet(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to close wallet: %w", err)
	}

	log.Println("Wallet closed successfully")
	return nil
}

// OpenWalletWithRetry attempts to open wallet with retries for startup scenarios
func OpenWalletWithRetry(publicPass string, maxRetries int) error {
	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := OpenWallet(ctx, publicPass)
		cancel()

		if err == nil {
			return nil
		}

		if i < maxRetries-1 {
			log.Printf("Failed to open wallet (attempt %d/%d): %v, retrying...", i+1, maxRetries, err)
			time.Sleep(2 * time.Second)
		} else {
			return fmt.Errorf("failed to open wallet after %d attempts: %w", maxRetries, err)
		}
	}
	return nil
}
