// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dcrpulse/internal/config"
)

// activeWallet is the name of the wallet dcrwallet currently serves. dcrwallet
// loads one wallet per process, so this is process-global state rather than a
// per-request value. Empty means no wallet is selected (the UI shows the list).
var (
	activeWalletMu sync.RWMutex
	activeWallet   string
)

// selectedWallet is the pointer file shape the dashboard writes for the
// dcrwallet entrypoint supervisor.
type selectedWallet struct {
	Name    string `json:"name"`
	Appdata string `json:"appdata,omitempty"`
	Network string `json:"network,omitempty"`
	Epoch   int64  `json:"epoch,omitempty"`
}

// walletState is the shape the supervisor writes back to report the wallet it
// has running.
type walletState struct {
	Running string `json:"running"`
	Appdata string `json:"appdata,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Epoch   int64  `json:"epoch,omitempty"`
}

// CurrentWalletName returns the active wallet's name, falling back to the
// default wallet so per-wallet config paths resolve even before a wallet is
// explicitly selected.
func CurrentWalletName() string {
	activeWalletMu.RLock()
	defer activeWalletMu.RUnlock()
	if activeWallet == "" {
		return config.DefaultWalletName
	}
	return activeWallet
}

// ActiveWalletName returns the selected wallet's name, or "" when no wallet is
// selected (the wallet-list state).
func ActiveWalletName() string {
	activeWalletMu.RLock()
	defer activeWalletMu.RUnlock()
	return activeWallet
}

func setActiveWalletName(name string) {
	activeWalletMu.Lock()
	activeWallet = name
	activeWalletMu.Unlock()
}

// SeedActiveWallet restores the active wallet at startup. The shared control
// pointer is the daemon source of truth: the supervisors run whatever wallet it
// names, so the dashboard adopts that wallet first (and heals a stale persisted
// selection) to keep the cert it pins for each daemon matched to the wallet
// actually running. Absent a pointer it prefers the persisted selection, and
// absent that it falls back to the default wallet when a legacy wallet database
// already exists on disk, so upgraded watch-only deployments resolve to their
// existing wallet without any migration.
func SeedActiveWallet() {
	if sel, err := readSelectedPointer(); err == nil && sel.Name != "" {
		setActiveWalletName(sel.Name)
		reconcileSelectedWallet(sel.Name)
		return
	}
	cfg, err := config.LoadGlobalCfg()
	if err != nil {
		log.Printf("Seed active wallet: load global config: %v", err)
		return
	}
	var name string
	if _, err := cfg.Get(config.KeySelectedWallet, &name); err != nil {
		log.Printf("Seed active wallet: read selection: %v", err)
	}
	if name != "" {
		setActiveWalletName(name)
		return
	}
	if legacyWalletExists() {
		setActiveWalletName(config.DefaultWalletName)
		if err := persistSelectedWallet(config.DefaultWalletName); err != nil {
			log.Printf("Seed active wallet: persist default: %v", err)
		}
	}
}

// reconcileSelectedWallet heals the persisted selection when it disagrees with
// the control pointer the daemons run from, so the dashboard's remembered wallet
// stays a trustworthy fallback if the pointer is later removed. Best effort: on
// any error the next restart simply re-adopts the pointer.
func reconcileSelectedWallet(name string) {
	cfg, err := config.LoadGlobalCfg()
	if err != nil {
		log.Printf("Seed active wallet: load global config: %v", err)
		return
	}
	var cur string
	if _, err := cfg.Get(config.KeySelectedWallet, &cur); err != nil {
		log.Printf("Seed active wallet: read selection: %v", err)
	}
	if cur == name {
		return
	}
	if err := cfg.Set(config.KeySelectedWallet, name); err != nil {
		log.Printf("Seed active wallet: heal selection: %v", err)
		return
	}
	if err := cfg.Save(); err != nil {
		log.Printf("Seed active wallet: persist healed selection: %v", err)
	}
}

// legacyWalletExists reports whether a wallet database is present at the legacy
// single-wallet appdata path for any supported network.
func legacyWalletExists() bool {
	for _, net := range []string{"mainnet", "testnet", "testnet3", "simnet"} {
		if _, err := os.Stat(config.WalletDbPath(config.LegacyWalletAppdata(), net)); err == nil {
			return true
		}
	}
	return false
}

// SetActiveWallet records the active wallet, persists the selection, and writes
// the supervisor pointer file pointing dcrwallet at the wallet's appdata.
func SetActiveWallet(name, network string) error {
	setActiveWalletName(name)
	if err := persistSelectedWallet(name); err != nil {
		return err
	}
	return writeSelectedPointer(selectedWallet{
		Name:    name,
		Appdata: config.ResolveWalletAppdata(name),
		Network: network,
		Epoch:   time.Now().UnixNano(),
	})
}

// ClearActiveWallet deselects the wallet and tells the supervisor to idle (run
// no dcrwallet process), returning the UI to the wallet list.
func ClearActiveWallet() error {
	setActiveWalletName("")
	if err := persistSelectedWallet(""); err != nil {
		return err
	}
	return writeSelectedPointer(selectedWallet{Name: "", Epoch: time.Now().UnixNano()})
}

func persistSelectedWallet(name string) error {
	cfg, err := config.LoadGlobalCfg()
	if err != nil {
		return err
	}
	if err := cfg.Set(config.KeySelectedWallet, name); err != nil {
		return err
	}
	return cfg.Save()
}

func writeSelectedPointer(sel selectedWallet) error {
	if err := os.MkdirAll(config.StackControlDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sel, "", "  ")
	if err != nil {
		return err
	}
	tmp := config.SelectedWalletPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, config.SelectedWalletPath())
}

// readWalletState returns the supervisor's last reported state.
func readWalletState() (walletState, error) {
	var st walletState
	data, err := os.ReadFile(config.WalletStatePath())
	if err != nil {
		return st, err
	}
	err = json.Unmarshal(data, &st)
	return st, err
}

// readSelectedPointer reads the shared control pointer the daemon supervisors
// run from.
func readSelectedPointer() (selectedWallet, error) {
	var sel selectedWallet
	data, err := os.ReadFile(config.SelectedWalletPath())
	if err != nil {
		return sel, err
	}
	err = json.Unmarshal(data, &sel)
	return sel, err
}

// DaemonWalletSelection returns the wallet name and network the daemon
// supervisors are currently running, read from the same control pointer they
// read. Falls back to the dashboard's active wallet (and an unset network) when
// the pointer is missing or unreadable, so the dashboard pins the cert of the
// wallet the daemons actually serve.
func DaemonWalletSelection() (name, network string) {
	if sel, err := readSelectedPointer(); err == nil && sel.Name != "" {
		return sel.Name, sel.Network
	}
	return CurrentWalletName(), ""
}

// walletDirExists reports whether a directory exists at path.
func walletDirExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false
		}
		return false
	}
	return fi.IsDir()
}

// walletControlBackupsDir is where DeleteWallet stows a tar of a wallet's
// appdata before removing it.
func walletControlBackupsDir() string {
	return filepath.Join(config.WalletDataRoot, "backups")
}
