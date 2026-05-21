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
)

// LogComponent identifies which daemon's log to read.
type LogComponent string

const (
	LogComponentDcrd      LogComponent = "dcrd"
	LogComponentDcrwallet LogComponent = "dcrwallet"
	LogComponentDcrlnd    LogComponent = "dcrlnd"
)

const (
	logsRoot   = "/app-data"
	maxLogTail = 5000
)

// logPath resolves the on-disk log file for a component. Each daemon
// uses its own directory layout — dcrlnd writes under
// `logs/decred/<network>/lnd.log` while dcrd and dcrwallet use
// `logs/<network>/<component>.log`.
func logPath(component LogComponent, network string) (string, error) {
	switch component {
	case LogComponentDcrd, LogComponentDcrwallet:
		return filepath.Join(logsRoot, string(component), "logs", network, string(component)+".log"), nil
	case LogComponentDcrlnd:
		return filepath.Join(logsRoot, "dcrlnd", "logs", "decred", network, "lnd.log"), nil
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

	path, err := logPath(component, network)
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
