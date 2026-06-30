// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
)

var walletNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidateWalletName enforces a safe, filesystem-friendly wallet name. The
// "imported" name is reserved by dcrwallet for its private-key bucket.
func ValidateWalletName(name string) error {
	if !walletNameRe.MatchString(name) {
		return fmt.Errorf("wallet name must be 1-64 characters of letters, numbers, dash or underscore")
	}
	if name == "imported" {
		return fmt.Errorf("%q is a reserved wallet name", name)
	}
	return nil
}

// SwitchWallet makes name the active wallet: it pauses sync, closes the current
// wallet, repoints the supervisor at the new wallet's appdata, waits for the
// daemon to relaunch, reconnects gRPC, and opens the new wallet.
func SwitchWallet(ctx context.Context, name, publicPass string) error {
	if err := ValidateWalletName(name); err != nil {
		return err
	}
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return err
	}
	if !walletExistsByName(name, network) {
		return fmt.Errorf("wallet %q not found", name)
	}

	if ActiveWalletName() == name {
		if loaded, _ := CheckWalletLoaded(ctx); loaded {
			return nil
		}
	}

	PauseSync()
	defer ResumeSync()

	// Clean unload of the current wallet before its daemon is killed.
	closeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if err := CloseWallet(closeCtx); err != nil {
		log.Printf("Switch wallet: close current (continuing): %v", err)
	}
	cancel()

	if err := SetActiveWallet(name, network); err != nil {
		return fmt.Errorf("select wallet: %w", err)
	}

	if err := waitForSupervisor(ctx, name); err != nil {
		return err
	}
	if err := rpc.ReconnectWalletGrpc(); err != nil {
		return fmt.Errorf("reconnect after switch: %w", err)
	}
	if err := rpc.WaitForWalletDaemon(ctx); err != nil {
		return fmt.Errorf("wait for daemon after switch: %w", err)
	}
	if err := OpenWallet(ctx, publicPass); err != nil {
		return err
	}
	touchLastAccess(network, name)
	reconnectStackServices(ctx, name)
	return nil
}

// reconnectStackServices repoints the dcrlnd / DEX / Bison Relay clients at the
// newly active wallet's per-wallet certs and clears secrets carried from the
// previous profile. The daemons relaunch independently (driven by the shared
// control pointer); these clients reconnect best-effort and each service's
// status machine resolves the rest (needs-unlock / needs-setup / syncing).
func reconnectStackServices(ctx context.Context, name string) {
	rpc.ClearDcrdexAppPass()
	if ActiveWalletIsWatchOnly(ctx) {
		// Watch-only wallets have no dcrlnd / DEX / Bison Relay daemons, so don't
		// repoint those clients at per-wallet certs that never exist. Still drop
		// any stale brclientd stream from the previous wallet; the notifications
		// loop then idles on its own for watch-only (see StartBrclientdNotifs).
		ReconnectBrclientdNotifs()
		return
	}
	rpc.ReconnectDcrlnd(config.DcrlndTLSCert(name), config.DcrlndMacaroon(name))
	rpc.UpdateDcrdexCertPaths(config.DcrdexCert(name), config.DcrdexWSCert(name))
	server, client, key := BrclientdDaemonCertPaths(ctx)
	rpc.UpdateBrclientdCerts(server, client, key)
	ReconnectBrclientdNotifs()
}

// CloseActiveWallet closes the current wallet and idles the supervisor so the UI
// returns to the wallet list.
func CloseActiveWallet(ctx context.Context) error {
	PauseSync()
	defer ResumeSync()

	closeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if err := CloseWallet(closeCtx); err != nil {
		log.Printf("Close active wallet (continuing): %v", err)
	}
	cancel()

	// The previous profile's DEX session secret must not leak into the next.
	rpc.ClearDcrdexAppPass()
	return ClearActiveWallet()
}

// CreateNamedWallet creates a new seed-based wallet under the given name,
// switching the daemon to its appdata first, then running the standard
// create/restore flow.
func CreateNamedWallet(ctx context.Context, name, publicPass, privatePass, seedHex string, discoverAccounts bool) error {
	network, err := newWalletSlot(ctx, name)
	if err != nil {
		return err
	}

	PauseSync()
	defer ResumeSync()

	if err := switchDaemonToNewWallet(ctx, name, network); err != nil {
		return err
	}
	if err := CreateNewWallet(ctx, publicPass, privatePass, seedHex, discoverAccounts); err != nil {
		return err
	}
	finishWalletCreate(ctx, network, name)
	return nil
}

// CreateNamedWatchOnlyWallet creates a watching-only wallet (from an xpub) under
// the given name, using the same supervisor handshake as a seed-based create.
func CreateNamedWatchOnlyWallet(ctx context.Context, name, publicPass, xpub string) error {
	network, err := newWalletSlot(ctx, name)
	if err != nil {
		return err
	}

	PauseSync()
	defer ResumeSync()

	if err := switchDaemonToNewWallet(ctx, name, network); err != nil {
		return err
	}
	if err := CreateWatchOnlyWallet(ctx, publicPass, xpub); err != nil {
		return err
	}
	// CreateWatchingOnlyWallet opens the wallet; ensure it is loaded for the
	// supervisor's sync, then tag the per-wallet config. dcrwallet reports
	// WatchingOnly=true, so the OpenWallet capture reconfirms it on every open.
	if err := OpenWallet(ctx, publicPass); err != nil {
		log.Printf("Watch-only create: ensure open: %v", err)
	}
	cacheWatchOnly(ctx, true)
	finishWalletCreate(ctx, network, name)
	return nil
}

// newWalletSlot validates a new wallet name and ensures it does not already
// exist, returning the current network. Shared by the create paths.
func newWalletSlot(ctx context.Context, name string) (string, error) {
	if err := ValidateWalletName(name); err != nil {
		return "", err
	}
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return "", err
	}
	if walletExistsByName(name, network) {
		return "", fmt.Errorf("wallet %q already exists", name)
	}
	return network, nil
}

// switchDaemonToNewWallet runs the supervisor handshake that brings a fresh
// dcrwallet up against a new wallet's appdata: close the current wallet, point
// the supervisor at the new one, wait for the relaunch, reconnect the gRPC
// clients, and wait for the loader to answer. Caller must hold PauseSync.
func switchDaemonToNewWallet(ctx context.Context, name, network string) error {
	closeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if err := CloseWallet(closeCtx); err != nil {
		log.Printf("Create wallet: close current (continuing): %v", err)
	}
	cancel()

	if err := SetActiveWallet(name, network); err != nil {
		return fmt.Errorf("select new wallet: %w", err)
	}
	if err := waitForSupervisor(ctx, name); err != nil {
		return err
	}
	if err := rpc.ReconnectWalletGrpc(); err != nil {
		return fmt.Errorf("reconnect for new wallet: %w", err)
	}
	if err := rpc.WaitForWalletDaemon(ctx); err != nil {
		return fmt.Errorf("wait for daemon for new wallet: %w", err)
	}
	return nil
}

// finishWalletCreate runs the post-create steps shared by every create path:
// stamp last-access and repoint the dcrlnd / DEX / Bison Relay clients at the new
// wallet's per-wallet certs, exactly as a switch does; without this the clients
// stay pinned to the previously active wallet's certs (cert-mismatch on DEX,
// wrong node for Lightning).
func finishWalletCreate(ctx context.Context, network, name string) {
	touchLastAccess(network, name)
	reconnectStackServices(ctx, name)
}

// RenameWallet renames a non-active, non-default wallet on disk and renames its
// dashboard config directory to match.
func RenameWallet(ctx context.Context, from, to string) error {
	if err := ValidateWalletName(to); err != nil {
		return err
	}
	// Validate the source name too: it is used to build the on-disk path that
	// gets renamed, so an unvalidated value such as "../.." could move a
	// directory outside the wallets tree.
	if err := ValidateWalletName(from); err != nil {
		return err
	}
	if from == config.DefaultWalletName {
		return fmt.Errorf("the default wallet cannot be renamed")
	}
	if from == ActiveWalletName() {
		return fmt.Errorf("close the wallet before renaming it")
	}
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return err
	}
	if !walletDirExists(config.WalletAppdataDir(from)) {
		return fmt.Errorf("wallet %q not found", from)
	}
	if walletDirExists(config.WalletAppdataDir(to)) {
		return fmt.Errorf("wallet %q already exists", to)
	}

	if err := os.Rename(config.WalletAppdataDir(from), config.WalletAppdataDir(to)); err != nil {
		return fmt.Errorf("rename wallet data: %w", err)
	}
	// Best-effort rename of the dashboard-side config directory.
	if walletDirExists(config.WalletDir(network, from)) {
		if err := os.Rename(config.WalletDir(network, from), config.WalletDir(network, to)); err != nil {
			log.Printf("Rename wallet: config dir (data already renamed): %v", err)
		}
	}
	return nil
}

// DeleteWallet permanently removes a non-active wallet's data and its dashboard
// config. This is irreversible and does NOT back up: the UI gates it behind a
// typed "DELETE" confirmation. The default wallet cannot be deleted; it is the
// fallback wallet and its appdata root holds the shared control dirs.
func DeleteWallet(ctx context.Context, name string) error {
	if err := ValidateWalletName(name); err != nil {
		return err
	}
	if name == config.DefaultWalletName {
		return fmt.Errorf("the default wallet cannot be deleted")
	}
	if name == ActiveWalletName() {
		return fmt.Errorf("close the wallet before deleting it")
	}
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return err
	}

	dataDir := config.WalletAppdataDir(name)
	if !walletDirExists(dataDir) {
		return fmt.Errorf("wallet %q not found", name)
	}

	// Permanently remove the wallet's appdata (wallet.db and all data).
	if err := os.RemoveAll(dataDir); err != nil {
		return fmt.Errorf("remove wallet data: %w", err)
	}
	log.Printf("Permanently deleted wallet %q (%s)", name, dataDir)

	// Remove the dashboard-side config directory (metadata only).
	if err := os.RemoveAll(config.WalletDir(network, name)); err != nil {
		log.Printf("Delete wallet: remove config dir: %v", err)
	}
	return nil
}

// waitForSupervisor blocks until the dcrwallet entrypoint supervisor reports it
// is running the named wallet, or ctx expires.
func waitForSupervisor(ctx context.Context, name string) error {
	want := config.ResolveWalletAppdata(name)
	for {
		if st, err := readWalletState(); err == nil {
			if st.Running == name && st.Appdata == want && st.PID > 0 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for wallet daemon to switch to %q", name)
		case <-time.After(1 * time.Second):
		}
	}
}

func touchLastAccess(network, name string) {
	cfg, err := config.LoadWalletCfg(network, name)
	if err != nil {
		log.Printf("Touch last access: load cfg: %v", err)
		return
	}
	if err := cfg.SetLastAccess(time.Now().Unix()); err != nil {
		log.Printf("Touch last access: set: %v", err)
		return
	}
	if err := cfg.Save(); err != nil {
		log.Printf("Touch last access: save: %v", err)
	}
}
