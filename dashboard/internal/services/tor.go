// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/types"
)

const torDefaultCircuitLimit = 32

// torProxyEndpoint returns host:port of the Tor SOCKS proxy from env, and
// whether it is configured.
func torProxyEndpoint() (string, bool) {
	ip := os.Getenv("TOR_PROXY_IP")
	port := os.Getenv("TOR_PROXY_PORT")
	if ip == "" || port == "" {
		return "", false
	}
	return net.JoinHostPort(ip, port), true
}

// ReadTorSettings returns the current Tor toggle state. When the pointer is
// absent (Tor never enabled) it returns the disabled default.
func ReadTorSettings() types.TorSettings {
	return readTorSettings(torSettingsPath())
}

// torSettingsPath is a var so tests can point the toggle at a temp file.
var torSettingsPath = config.TorPointerPath

var torUnreadableOnce sync.Once

// readTorSettings reads the pointer at path. A pointer that exists but cannot
// be read or parsed reads as Tor on: the setting is unknown, and guessing off
// would send traffic over clearnet that the operator may have asked to hide.
func readTorSettings(path string) types.TorSettings {
	s := types.TorSettings{Isolation: true, CircuitLimit: torDefaultCircuitLimit}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s
	}
	if err == nil {
		err = json.Unmarshal(data, &s)
	}
	if err != nil {
		torUnreadableOnce.Do(func() {
			settLog.Warnf("Tor setting %s is unreadable (%v); treating Tor as on until it is saved again", path, err)
		})
		return types.TorSettings{Enabled: true, Isolation: true, CircuitLimit: torDefaultCircuitLimit}
	}
	return s
}

// WriteTorSettings persists the Tor toggle state, bumping Rev so every
// supervisor relaunches its daemon with the new flags.
func WriteTorSettings(in types.TorSettings) (types.TorSettings, error) {
	cur := ReadTorSettings()
	out := types.TorSettings{
		Enabled:      in.Enabled,
		Isolation:    in.Isolation,
		DcrdOnion:    in.DcrdOnion,
		LnOnion:      in.LnOnion,
		CircuitLimit: in.CircuitLimit,
		Rev:          cur.Rev + 1,
	}
	if out.CircuitLimit < 1 || out.CircuitLimit > 1000 {
		out.CircuitLimit = torDefaultCircuitLimit
	}
	if err := os.MkdirAll(config.StackControlDir(), 0o700); err != nil {
		return out, err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return out, err
	}
	if err := writeFileSynced(config.TorPointerPath(), data, 0o644); err != nil {
		return out, err
	}
	return out, nil
}

// TorProxyReachable reports whether the Tor SOCKS proxy accepts connections.
func TorProxyReachable() bool {
	addr, ok := torProxyEndpoint()
	if !ok {
		return false
	}
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// onionHostname returns a tor hidden service's hostname, or "" when the
// onion has not been created or is unreadable.
func onionHostname(hsDir string) string {
	data, err := os.ReadFile(filepath.Join(config.TorDataDir, hsDir, "hostname"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// TorDaemonStates reads each supervisor's control-state file for its live Tor
// routing flag.
func TorDaemonStates(s types.TorSettings) []types.TorDaemonState {
	type stateFile struct {
		PID    int    `json:"pid"`
		Tor    bool   `json:"tor"`
		TorRev string `json:"torRev"`
	}
	srcs := []struct{ name, path string }{
		{"dcrd", config.DcrdStatePath()},
		{"dcrwallet", config.WalletStatePath()},
		{"dcrlnd", config.DcrlndStatePath()},
		{"dcrdex", config.DcrdexStatePath()},
		{"brclientd", config.BrclientdStatePath()},
	}
	out := make([]types.TorDaemonState, 0, len(srcs))
	for _, s := range srcs {
		ds := types.TorDaemonState{Name: s.name}
		if data, err := os.ReadFile(s.path); err == nil {
			var sf stateFile
			if json.Unmarshal(data, &sf) == nil {
				ds.Running = sf.PID > 0
				ds.Tor = sf.Tor
				ds.TorRev = sf.TorRev
			}
		}
		out = append(out, ds)
	}
	// The dashboard's own external calls (rate oracle, Politeia, VSP, BR
	// seeder, invite bot) switch per request, so its entry tracks the
	// toggle directly instead of a supervisor state file. Liquidity provider
	// requests cannot be routed and are refused while the toggle is on.
	out = append(out, types.TorDaemonState{
		Name:    "dashboard",
		Running: true,
		Tor:     s.Enabled,
		TorRev:  strconv.Itoa(s.Rev),
	})
	return out
}

// TorStatusSnapshot aggregates the Tor picture for the settings UI.
func TorStatusSnapshot() types.TorStatus {
	s := ReadTorSettings()
	return types.TorStatus{
		Settings:       s,
		ProxyReachable: TorProxyReachable(),
		OnionAddress:   onionHostname("dcrd-hs"),
		LnOnionAddress: onionHostname("dcrlnd-hs"),
		Daemons:        TorDaemonStates(s),
	}
}

// writeFileSynced replaces path atomically and durably: the new content is
// flushed before the rename and the rename before returning, so a crash
// cannot leave a torn pointer behind.
func writeFileSynced(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
