// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	s := types.TorSettings{Isolation: true, CircuitLimit: torDefaultCircuitLimit}
	data, err := os.ReadFile(config.TorPointerPath())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
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
	tmp := config.TorPointerPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return out, err
	}
	if err := os.Rename(tmp, config.TorPointerPath()); err != nil {
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

// DcrdOnionAddress returns the dcrd hidden-service hostname, or "" when the
// onion has not been created or is unreadable.
func DcrdOnionAddress() string {
	data, err := os.ReadFile(filepath.Join(config.TorDataDir, "dcrd-hs", "hostname"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// TorDaemonStates reads each supervisor's control-state file for its live Tor
// routing flag.
func TorDaemonStates() []types.TorDaemonState {
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
	return out
}

// TorStatusSnapshot aggregates the Tor picture for the settings UI.
func TorStatusSnapshot() types.TorStatus {
	return types.TorStatus{
		Settings:       ReadTorSettings(),
		ProxyReachable: TorProxyReachable(),
		OnionAddress:   DcrdOnionAddress(),
		Daemons:        TorDaemonStates(),
	}
}
