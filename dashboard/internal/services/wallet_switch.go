// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	rpc.ReconnectDcrlnd(config.DcrlndTLSCert(name), config.DcrlndMacaroon(name))
	rpc.UpdateDcrdexCertPaths(config.DcrdexCert(name), config.DcrdexWSCert(name))
	rpc.ClearDcrdexAppPass()
	if network, err := CurrentNetwork(ctx); err == nil {
		base := config.BrclientdDir(name)
		rpc.UpdateBrclientdCerts(
			filepath.Join(base, "data", network, "rpc", "rpc.cert"),
			filepath.Join(base, "data", network, "rpc", "rpc-client.cert"),
			filepath.Join(base, "data", network, "rpc", "rpc-client.key"),
		)
	}
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

// CreateNamedWallet creates a new wallet under the given name, switching the
// daemon to its appdata first, then running the standard create/restore flow.
func CreateNamedWallet(ctx context.Context, name, publicPass, privatePass, seedHex string, discoverAccounts bool) error {
	if err := ValidateWalletName(name); err != nil {
		return err
	}
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return err
	}
	if walletExistsByName(name, network) {
		return fmt.Errorf("wallet %q already exists", name)
	}

	PauseSync()
	defer ResumeSync()

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
	if err := CreateNewWallet(ctx, publicPass, privatePass, seedHex, discoverAccounts); err != nil {
		return err
	}
	touchLastAccess(network, name)
	// Repoint the dcrlnd / DEX / Bison Relay clients at the new wallet's
	// per-wallet certs, exactly as a switch does; without this the clients stay
	// pinned to the previously active wallet's certs (cert-mismatch on DEX,
	// wrong node for Lightning).
	reconnectStackServices(ctx, name)
	return nil
}

// RenameWallet renames a non-active, non-default wallet on disk and renames its
// dashboard config directory to match.
func RenameWallet(ctx context.Context, from, to string) error {
	if err := ValidateWalletName(to); err != nil {
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

// DeleteWallet backs up a non-active wallet's data, then removes it along with
// its dashboard config. The default wallet's database directory is removed in
// place (its appdata root also holds shared control/backup directories).
func DeleteWallet(ctx context.Context, name string) error {
	if err := ValidateWalletName(name); err != nil {
		return err
	}
	if name == ActiveWalletName() {
		return fmt.Errorf("close the wallet before deleting it")
	}
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return err
	}

	var dataDir string
	if name == config.DefaultWalletName {
		// Only the network database directory, never the shared appdata root.
		dataDir = filepath.Join(config.LegacyWalletAppdata(), network)
	} else {
		dataDir = config.WalletAppdataDir(name)
	}
	if !walletDirExists(dataDir) {
		return fmt.Errorf("wallet %q not found", name)
	}

	// Back up by moving the data aside before removal so coins are never
	// destroyed outright (mirrors the backup-before-risky-ops policy).
	backupRoot := walletControlBackupsDir()
	dest := filepath.Join(backupRoot, fmt.Sprintf("%s-%d", name, time.Now().UnixNano()))
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return fmt.Errorf("prepare backup: %w", err)
	}
	if err := os.Rename(dataDir, filepath.Join(dest, filepath.Base(dataDir))); err != nil {
		return fmt.Errorf("back up wallet data: %w", err)
	}
	log.Printf("Deleted wallet %q; data backed up to %s", name, dest)

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
