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
	"sync/atomic"
	"time"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

// restoreDiscoveryActive guards dcrwallet's single RpcSync slot during a
// restore. dcrwallet permits only one syncer at a time. When a wallet is
// restored from seed, runDiscoveryRpcSync must own that slot so it can run
// RpcSync with DiscoverAccounts=true AND the private passphrase (which keeps
// the wallet unlocked for address/account discovery). The boot-time supervisor
// (superviseRpcSync) starts a passphrase-less RpcSync the instant CreateWallet
// opens the wallet; without this gate it wins the slot, the discovery sync is
// rejected ("already synchronizing") so no accounts are discovered, and the
// supervisor's sync then runs DiscoverActiveAddresses against a wallet that
// locks - which also corrupts the per-account encryption written during restore.
// The supervisor waits while this is set; runDiscoveryRpcSync clears it when its
// discovery stream ends.
var restoreDiscoveryActive atomic.Bool

// BeginRestoreDiscovery marks a restore account-discovery sync as owning the
// RpcSync slot. Call before CreateWallet opens the wallet.
func BeginRestoreDiscovery() { restoreDiscoveryActive.Store(true) }

// EndRestoreDiscovery releases the slot so the sync supervisor can take over.
func EndRestoreDiscovery() { restoreDiscoveryActive.Store(false) }

// RestoreDiscoveryActive reports whether a restore discovery sync owns the slot.
func RestoreDiscoveryActive() bool { return restoreDiscoveryActive.Load() }

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

// GenerateSeed generates a new cryptographically secure seed.
// seedLength is in BYTES. Zero passes through to dcrwallet, which uses its
// RecommendedSeedLen (32 bytes -> 33-word mnemonic).
func GenerateSeed(ctx context.Context, seedLength uint32) (*types.GenerateSeedResponse, error) {
	if rpc.SeedServiceClient == nil {
		return nil, fmt.Errorf("seed service client not initialized")
	}

	req := &pb.GenerateRandomSeedRequest{
		SeedLength: seedLength,
	}

	resp, err := rpc.SeedServiceClient.GenerateRandomSeed(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate seed: %w", err)
	}

	return &types.GenerateSeedResponse{
		SeedMnemonic: resp.SeedMnemonic,
		SeedHex:      resp.SeedHex,
	}, nil
}

// DecodeSeed validates and decodes a user-supplied seed. UserInput can be
// the 33-word mnemonic or a 64-character hex string. dcrwallet's DecodeSeed
// accepts both via a single field.
func DecodeSeed(ctx context.Context, userInput string) (string, error) {
	if rpc.SeedServiceClient == nil {
		return "", fmt.Errorf("seed service client not initialized")
	}
	resp, err := rpc.SeedServiceClient.DecodeSeed(ctx, &pb.DecodeSeedRequest{UserInput: userInput})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(resp.DecodedSeed), nil
}

// CreateNewWallet creates a new wallet with the provided passphrases and seed.
// When discoverAccounts is true (restoring from an existing seed), the
// post-create RpcSync runs with DiscoverAccounts enabled and the private
// passphrase so dcrwallet rescans the chain and rebuilds the address index.
func CreateNewWallet(ctx context.Context, publicPass, privatePass, seedHex string, discoverAccounts bool) error {
	if rpc.WalletLoaderClient == nil {
		return fmt.Errorf("wallet loader client not initialized")
	}

	// Decode seed hex to bytes
	seedBytes, err := hex.DecodeString(seedHex)
	if err != nil {
		return fmt.Errorf("invalid seed hex: %w", err)
	}

	log.Printf("Creating wallet with seed length: %d bytes", len(seedBytes))

	// Claim dcrwallet's RpcSync slot for the restore discovery BEFORE CreateWallet
	// opens the wallet (which unblocks the sync supervisor). runDiscoveryRpcSync
	// releases it when its stream ends; if we never launch it (early error path),
	// the deferred release below clears the gate so the supervisor isn't stuck.
	discoveryLaunched := false
	if discoverAccounts {
		BeginRestoreDiscovery()
		defer func() {
			if !discoveryLaunched {
				EndRestoreDiscovery()
			}
		}()
	}

	req := &pb.CreateWalletRequest{
		PublicPassphrase:  []byte(publicPass),
		PrivatePassphrase: []byte(privatePass),
		Seed:              seedBytes,
	}

	_, err = rpc.WalletLoaderClient.CreateWallet(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create wallet: %w", err)
	}

	// Per-account passphrases are set differently for a fresh wallet vs a restore:
	//
	//   - Fresh create (no discovery): only the default account exists, so give it
	//     its per-account passphrase now.
	//   - Restore (discoverAccounts): do NOT encrypt any account before discovery.
	//     dcrwallet's account discovery corrupts the per-account encryption record
	//     of an account that is already uniquely-encrypted when discovery runs, so
	//     the default account would end up sealed under unrecoverable bytes. Mirror
	//     Decrediton, which runs setAccountsPass only AFTER discovery reaches SYNCED;
	//     runDiscoveryRpcSync does that below once its discovery stream completes.
	if !discoverAccounts {
		if err := ensureAllAccountsEncrypted(ctx, []byte(privatePass)); err != nil {
			return fmt.Errorf("set account passphrases: %w", err)
		}
	}

	log.Println("Wallet created and opened successfully")

	// For restored wallets, kick a one-time RpcSync with DiscoverAccounts=true
	// so dcrwallet scans the chain for addresses derived from the seed. The
	// regular supervisor (cmd/dcrpulse/main.go) resumes with default args
	// once this stream ends.
	if discoverAccounts {
		discoveryLaunched = true
		go runDiscoveryRpcSync(privatePass)
	}

	return nil
}

func runDiscoveryRpcSync(privatePass string) {
	// Release the RpcSync slot for the supervisor once this discovery stream ends.
	defer EndRestoreDiscovery()
	if rpc.WalletLoaderClient == nil {
		return
	}
	var cert []byte
	if rpc.DcrdConfig.RPCCert != "" {
		c, err := os.ReadFile(rpc.DcrdConfig.RPCCert)
		if err != nil {
			log.Printf("Discovery RPC sync: failed to read dcrd cert: %v", err)
			return
		}
		cert = c
	}
	networkAddr := fmt.Sprintf("%s:%s", rpc.DcrdConfig.RPCHost, rpc.DcrdConfig.RPCPort)
	req := &pb.RpcSyncRequest{
		NetworkAddress:    networkAddr,
		Username:          rpc.DcrdConfig.RPCUser,
		Password:          []byte(rpc.DcrdConfig.RPCPassword),
		Certificate:       cert,
		DiscoverAccounts:  true,
		PrivatePassphrase: []byte(privatePass),
	}
	// Run discovery on a cancellable context so we can stop the stream once the
	// initial discovery+sync reaches SYNCED. dcrwallet keeps the wallet unlocked
	// for the whole lifetime of a DiscoverAccounts RpcSync stream, so leaving it
	// open forever would both keep the wallet unlocked and prevent the supervisor
	// from ever taking over. We stop at SYNCED, set per-account passphrases, then
	// release the slot to the supervisor for ongoing sync.
	syncCtx, cancelSync := context.WithCancel(context.Background())
	defer cancelSync()
	stream, err := rpc.WalletLoaderClient.RpcSync(syncCtx, req)
	if err != nil {
		log.Printf("Discovery RPC sync: failed to start: %v", err)
		return
	}
	log.Printf("Discovery RPC sync started against %s", networkAddr)
	for {
		resp, err := stream.Recv()
		if err != nil {
			log.Printf("Discovery RPC sync stream ended: %v", err)
			break
		}
		ApplyRpcSyncNotification(resp)
		if resp.Synced {
			log.Println("Discovery RPC sync reached SYNCED; finalizing account passphrases")
			cancelSync()
			break
		}
	}

	// Discovery has recovered the seed's accounts. Now (and only now, after
	// discovery) give every account the same per-account passphrase as the wallet
	// passphrase so they all unlock uniformly via UnlockAccount. Mirrors
	// Decrediton's setAccountsPass-on-SYNCED. Best-effort: log on failure.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := ensureAllAccountsEncrypted(ctx, []byte(privatePass)); err != nil {
		log.Printf("Discovery RPC sync: set account passphrases: %v", err)
	}
}

// OpenWallet opens an existing wallet with the provided public passphrase
func OpenWallet(ctx context.Context, publicPass string) error {
	if rpc.WalletLoaderClient == nil {
		return fmt.Errorf("wallet loader client not initialized")
	}

	// First check if wallet is already loaded to avoid unnecessary open attempts
	loaded, err := CheckWalletLoaded(ctx)
	if err == nil && loaded {
		log.Println("Wallet is already loaded and ready")
		return nil
	}

	log.Println("Opening wallet...")

	req := &pb.OpenWalletRequest{
		PublicPassphrase: []byte(publicPass),
	}

	_, err = rpc.WalletLoaderClient.OpenWallet(ctx, req)
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

	// RpcSync is kicked + supervised by SuperviseRpcSync in main.go.

	return nil
}

// EnsureRpcSync opens an RpcSync stream and dispatches notifications until
// ctx is cancelled or the stream errors.
func EnsureRpcSync(ctx context.Context) error {
	if rpc.WalletLoaderClient == nil {
		return fmt.Errorf("wallet loader client not initialized")
	}

	var cert []byte
	if rpc.DcrdConfig.RPCCert != "" {
		var err error
		cert, err = os.ReadFile(rpc.DcrdConfig.RPCCert)
		if err != nil {
			return fmt.Errorf("read dcrd cert for RPC sync: %w", err)
		}
	}

	networkAddr := fmt.Sprintf("%s:%s", rpc.DcrdConfig.RPCHost, rpc.DcrdConfig.RPCPort)
	req := &pb.RpcSyncRequest{
		NetworkAddress:    networkAddr,
		Username:          rpc.DcrdConfig.RPCUser,
		Password:          []byte(rpc.DcrdConfig.RPCPassword),
		Certificate:       cert,
		DiscoverAccounts:  false,
		PrivatePassphrase: []byte{},
	}

	stream, err := rpc.WalletLoaderClient.RpcSync(ctx, req)
	if err != nil {
		if strings.Contains(err.Error(), "already") {
			log.Println("RPC sync already running in dcrwallet — will resync on next opportunity")
			return nil
		}
		return fmt.Errorf("open RpcSync stream: %w", err)
	}

	log.Printf("RPC sync stream open to dcrd at %s", networkAddr)

	for {
		resp, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("RpcSync stream ended: %w", err)
		}
		ApplyRpcSyncNotification(resp)
	}
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

// CheckWalletLoaded checks if a wallet is currently loaded and ready
func CheckWalletLoaded(ctx context.Context) (bool, error) {
	if rpc.WalletGrpcClient == nil {
		return false, fmt.Errorf("wallet gRPC client not initialized")
	}

	// Try to ping the wallet service - this only works if wallet is loaded
	req := &pb.PingRequest{}
	_, err := rpc.WalletGrpcClient.Ping(ctx, req)
	if err != nil {
		if strings.Contains(err.Error(), "wallet has not loaded") ||
			strings.Contains(err.Error(), "wallet is not opened") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check wallet status: %w", err)
	}

	return true, nil
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
