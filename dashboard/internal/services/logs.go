// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dcrpulse/internal/config"
)

// LogComponent identifies which daemon's log to read.
type LogComponent string

const (
	LogComponentDcrd      LogComponent = "dcrd"
	LogComponentDcrwallet LogComponent = "dcrwallet"
	LogComponentDcrlnd    LogComponent = "dcrlnd"
	LogComponentBrclientd LogComponent = "brclientd"
	LogComponentDcrdex    LogComponent = "dcrdex"
	LogComponentTor       LogComponent = "tor"
)

const (
	logsRoot   = "/app-data"
	maxLogTail = 5000
)

// logPath resolves the on-disk log file for a component. Each daemon
// uses its own directory layout — dcrlnd writes under
// `logs/decred/<network>/lnd.log` while dcrwallet uses
// `logs/<network>/dcrwallet.log`. The per-wallet daemons (dcrwallet,
// dcrlnd, dcrdex, brclientd) write under the active wallet's appdata dir,
// so we resolve their roots against `wallet`. dcrd is a single shared full
// node serving every wallet, so its log stays on the legacy path.
func logPath(component LogComponent, network, wallet string) (string, error) {
	switch component {
	case LogComponentDcrd:
		return filepath.Join(logsRoot, "dcrd", "logs", network, "dcrd.log"), nil
	case LogComponentDcrwallet:
		return filepath.Join(config.ResolveWalletAppdata(wallet), "logs", network, "dcrwallet.log"), nil
	case LogComponentDcrlnd:
		return filepath.Join(config.DcrlndDir(wallet), "logs", "decred", network, "lnd.log"), nil
	case LogComponentBrclientd:
		return BrclientdLogPath(network), nil
	case LogComponentDcrdex:
		// bisonw writes its app log to <appdata>/<network>/logs/dexc.log.
		return filepath.Join(config.DcrdexDir(wallet), network, "logs", "dexc.log"), nil
	case LogComponentTor:
		// The tor sidecar has no network or wallet dimension; torrc logs
		// straight into its DataDirectory volume.
		return filepath.Join(logsRoot, "tor", "tor.log"), nil
	default:
		return "", fmt.Errorf("unknown log component: %q", component)
	}
}

// TailLog returns the last `lines` lines of the named component's log
// for the active network. The dashboard mounts /app-data read-only, so
// we can't write — only read.
func TailLog(ctx context.Context, component LogComponent, lines int) ([]string, error) {
	if lines <= 0 {
		lines = 200
	}
	if lines > maxLogTail {
		lines = maxLogTail
	}

	network, err := CurrentNetwork(ctx)
	if err != nil || network == "" {
		network = "mainnet"
	}

	path, err := logPath(component, network, CurrentWalletName())
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	// Scan into a ring buffer of size `lines`.
	scanner := bufio.NewScanner(f)
	// dcrwallet/dcrd lines can be long; raise the default 64 KB buffer.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	ring := make([]string, lines)
	count := 0
	for scanner.Scan() {
		ring[count%lines] = scanner.Text()
		count++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}

	out := make([]string, 0, lines)
	if count <= lines {
		for i := 0; i < count; i++ {
			out = append(out, ring[i])
		}
	} else {
		start := count % lines
		for i := 0; i < lines; i++ {
			out = append(out, ring[(start+i)%lines])
		}
	}
	// Strip stray carriage returns picked up from logs written on Windows
	// hosts that might leak into bind mounts.
	for i, line := range out {
		out[i] = strings.TrimRight(line, "\r")
	}
	return out, nil
}
