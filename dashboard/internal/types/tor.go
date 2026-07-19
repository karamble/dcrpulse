// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// TorSettings is the Tor toggle state the dashboard writes to the shared control
// pointer (/app-data/control/tor.json) and every daemon supervisor reads. Rev is
// bumped on each change so the supervisors relaunch their daemon with the new
// flags. The JSON keys match the sed parse in the entrypoint scripts.
type TorSettings struct {
	Enabled      bool `json:"enabled"`
	Isolation    bool `json:"isolation"`
	DcrdOnion    bool `json:"dcrdOnion"`
	LnOnion      bool `json:"lnOnion"`
	CircuitLimit int  `json:"circuitLimit"`
	Rev          int  `json:"rev"`
}

// TorDaemonState is one daemon's live Tor routing state, read from its
// supervisor control-state file.
type TorDaemonState struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Tor     bool   `json:"tor"`
	TorRev  string `json:"torRev"`
}

// TorStatus aggregates the live Tor picture for the settings UI.
type TorStatus struct {
	Settings       TorSettings      `json:"settings"`
	ProxyReachable bool             `json:"proxyReachable"`
	OnionAddress   string           `json:"onionAddress,omitempty"`
	Daemons        []TorDaemonState `json:"daemons"`
}

// TorControlInfo is the live data read from the Tor control port.
type TorControlInfo struct {
	Reachable    bool   `json:"reachable"`
	BootstrapPct int    `json:"bootstrapPct"`
	BootstrapTag string `json:"bootstrapTag"`
	Circuits     int    `json:"circuits"`
	BytesRead    int64  `json:"bytesRead"`
	BytesWritten int64  `json:"bytesWritten"`
	Version      string `json:"version"`
	Error        string `json:"error,omitempty"`
}
