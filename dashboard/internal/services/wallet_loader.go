// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
	"dcrpulse/internal/utils"

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
//
// privatePass is a slice rather than a string because the discovery goroutine
// below outlives this call by the length of a chain scan; the caller owns it,
// and discovery gets its own copy to wipe.
func CreateNewWallet(ctx context.Context, publicPass string, privatePass []byte, seedHex string, discoverAccounts bool) error {
	if rpc.WalletLoaderClient == nil {
		return fmt.Errorf("wallet loader client not initialized")
	}

	// Decode seed hex to bytes
	seedBytes, err := hex.DecodeString(seedHex)
	if err != nil {
		return fmt.Errorf("invalid seed hex: %w", err)
	}
	// The seed is the wallet. The defer is the backstop for an early return;
	// the wipe that matters happens the moment CreateWallet has taken it.
	defer utils.Zero(seedBytes)

	wlltLog.Infof("Creating wallet with seed length: %d bytes", len(seedBytes))

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
		PrivatePassphrase: privatePass,
		Seed:              seedBytes,
	}

	_, err = rpc.WalletLoaderClient.CreateWallet(ctx, req)
	utils.Zero(seedBytes) // dcrwallet has it now; nothing below needs it
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
	//
	// A failure here is logged, not returned: the wallet already exists and is
	// open, so aborting would strand the caller mid-switch with the new wallet
	// active but the rest of the stack never repointed at it. Any account missed
	// here is migrated by unlockAccountForSpend on first use.
	if !discoverAccounts {
		if err := ensureAllAccountsEncryptedRetry(ctx, privatePass); err != nil {
			wlltLog.Errorf("Wallet created but per-account encryption did not complete: %v", err)
		}
	}

	wlltLog.Info("Wallet created and opened successfully")

	// For restored wallets, kick a one-time RpcSync with DiscoverAccounts=true
	// so dcrwallet scans the chain for addresses derived from the seed. The
	// regular supervisor (cmd/dcrpulse/main.go) resumes with default args
	// once this stream ends.
	if discoverAccounts {
		discoveryLaunched = true
		go runDiscoveryRpcSync(append([]byte(nil), privatePass...))
	}

	return nil
}

// CreateWatchOnlyWallet creates a watching-only wallet from an extended public
// key (dpub mainnet / tpub testnet). The wallet has no private keys, so there is
// no seed and no per-account passphrase to set; dcrwallet reports WatchingOnly=true
// for it. The supervisor's normal RpcSync discovers used addresses under the key.
func CreateWatchOnlyWallet(ctx context.Context, publicPass, xpub string) error {
	if rpc.WalletLoaderClient == nil {
		return fmt.Errorf("wallet loader client not initialized")
	}
	if !strings.HasPrefix(xpub, "dpub") && !strings.HasPrefix(xpub, "tpub") {
		return fmt.Errorf("invalid extended public key: must start with dpub (mainnet) or tpub (testnet)")
	}

	req := &pb.CreateWatchingOnlyWalletRequest{
		ExtendedPubKey:   xpub,
		PublicPassphrase: []byte(publicPass),
	}
	if _, err := rpc.WalletLoaderClient.CreateWatchingOnlyWallet(ctx, req); err != nil {
		return fmt.Errorf("failed to create watching-only wallet: %w", err)
	}

	wlltLog.Info("Watching-only wallet created and opened successfully")
	return nil
}

// runDiscoveryRpcSync owns privatePass and wipes it when the scan ends.
func runDiscoveryRpcSync(privatePass []byte) {
	// Release the RpcSync slot for the supervisor once this discovery stream ends.
	defer EndRestoreDiscovery()
	defer utils.Zero(privatePass)
	// A restore scans the whole chain, so this goroutine outlives its request by
	// minutes or hours. The passphrase step below is the one cleanup path in this
	// package that retries, so it is the only one that can still be issuing RPCs
	// after a wallet switch repointed the shared clients.
	startedOn := ActiveWalletName()
	if rpc.WalletLoaderClient == nil {
		return
	}
	var cert []byte
	if rpc.DcrdConfig.RPCCert != "" {
		c, err := os.ReadFile(rpc.DcrdConfig.RPCCert)
		if err != nil {
			wlltLog.Warnf("Discovery RPC sync: failed to read dcrd cert: %v", err)
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
		PrivatePassphrase: privatePass,
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
		wlltLog.Warnf("Discovery RPC sync: failed to start: %v", err)
		return
	}
	wlltLog.Infof("Discovery RPC sync started against %s", networkAddr)
	for {
		resp, err := stream.Recv()
		if err != nil {
			// Not SYNCED, so discovery never finished: the accounts the step
			// below would seal may not all exist yet. Returning also stops it
			// running against whatever wallet a switch has since loaded.
			wlltLog.Warnf("Discovery RPC sync stream ended: %v", err)
			return
		}
		ApplyRpcSyncNotification(resp)
		if resp.Synced {
			wlltLog.Info("Discovery RPC sync reached SYNCED; finalizing account passphrases")
			cancelSync()
			break
		}
	}

	// Discovery has recovered the seed's accounts. Now (and only now, after
	// discovery) give every account the same per-account passphrase as the wallet
	// passphrase so they all unlock uniformly via UnlockAccount. Mirrors
	// Decrediton's setAccountsPass-on-SYNCED. Best-effort: log on failure.
	if now := ActiveWalletName(); now != startedOn {
		wlltLog.Warnf("Discovery RPC sync: skipping account passphrases, the active wallet changed from %q to %q", startedOn, now)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := ensureAllAccountsEncryptedRetry(ctx, privatePass); err != nil {
		wlltLog.Errorf("Discovery RPC sync: set account passphrases: %v", err)
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
		wlltLog.Info("Wallet is already loaded and ready")
		return nil
	}

	wlltLog.Info("Opening wallet...")

	req := &pb.OpenWalletRequest{
		PublicPassphrase: []byte(publicPass),
	}

	resp, err := rpc.WalletLoaderClient.OpenWallet(ctx, req)
	if err != nil {
		// Check if wallet is already opened
		if strings.Contains(err.Error(), "already opened") {
			wlltLog.Info("Wallet already opened")
		} else {
			return fmt.Errorf("failed to open wallet: %w", err)
		}
	} else {
		wlltLog.Info("Wallet opened successfully")
		// dcrwallet authoritatively reports watching-only here; cache it for the
		// active wallet so it survives restarts that skip this open path.
		cacheWatchOnly(ctx, resp.GetWatchingOnly())
	}

	// RpcSync is kicked + supervised by SuperviseRpcSync in main.go.

	return nil
}

// cacheWatchOnly persists dcrwallet's authoritative watching-only flag (from
// OpenWalletResponse) into the ACTIVE wallet's config, so the value survives
// dashboard restarts that hit the already-loaded short-circuit above. Per-wallet
// keyed for the multi-wallet setup; writes only when the stored value changes.
func cacheWatchOnly(ctx context.Context, watching bool) {
	name := ActiveWalletName()
	if name == "" {
		return
	}
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return
	}
	cfg, err := config.LoadWalletCfg(network, name)
	if err != nil {
		return
	}
	var stored bool
	if present, _ := cfg.Get(config.KeyIsWatchOnly, &stored); present && stored == watching {
		return
	}
	if err := cfg.Set(config.KeyIsWatchOnly, watching); err != nil {
		wlltLog.Errorf("watch-only flag: set failed for %s: %v", name, err)
		return
	}
	if err := cfg.Save(); err != nil {
		wlltLog.Errorf("watch-only flag: save failed for %s: %v", name, err)
	}
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
			wlltLog.Info("RPC sync already running in dcrwallet — will resync on next opportunity")
			return nil
		}
		return fmt.Errorf("open RpcSync stream: %w", err)
	}

	wlltLog.Infof("RPC sync stream open to dcrd at %s", networkAddr)

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

	wlltLog.Info("Closing wallet...")

	req := &pb.CloseWalletRequest{}
	_, err := rpc.WalletLoaderClient.CloseWallet(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to close wallet: %w", err)
	}

	wlltLog.Info("Wallet closed successfully")
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
