// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	LogComponentDcrpulse  LogComponent = "dcrpulse"
)

const (
	logsRoot   = "/app-data"
	maxLogTail = 5000

	// tailBytesPerLine budgets the seek window per requested line, set above the
	// p99 daemon log line so the tail is served without re-reading a rolled log.
	tailBytesPerLine = 256
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
	case LogComponentDcrpulse:
		// The dashboard's own log, on its data volume rather than under
		// logsRoot, and like tor with no network or wallet dimension.
		return config.DashboardLogPath(), nil
	default:
		return "", fmt.Errorf("unknown log component: %q", component)
	}
}

// tailLineMax bounds one returned line. A longer line is reported as its head
// plus a marker and the rest is discarded, so a single huge line costs its own
// content and nothing else.
const tailLineMax = 8 * 1024

// readLogLine reads one newline-terminated line. It uses ReadSlice rather than
// ReadString so an over-long line is never accumulated: past tailLineMax the
// remainder is read and dropped a bufferful at a time. A Scanner cannot do this
// at all - it stops at the first token over its buffer, hiding every line after
// it. Returns io.EOF alongside a final unterminated line.
func readLogLine(r *bufio.Reader) (string, error) {
	var b []byte
	truncated := false
	for {
		chunk, err := r.ReadSlice('\n')
		if n := len(b); n < tailLineMax {
			b = append(b, chunk[:min(len(chunk), tailLineMax-n)]...)
		} else if len(chunk) > 0 {
			truncated = true
		}
		if err == bufio.ErrBufferFull {
			// The delimiter is not in the buffer yet; keep draining.
			truncated = truncated || len(b) >= tailLineMax
			continue
		}
		line := strings.TrimRight(string(b), "\r\n")
		if len(line) >= tailLineMax {
			truncated = true
		}
		if truncated {
			line += "...[line truncated]"
		}
		return line, err
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
	if os.IsNotExist(err) && component == LogComponentDcrpulse {
		// We write this one ourselves, so a read can land inside the
		// rotator's rename-and-recreate window.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
		f, err = os.Open(path)
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	// Start near the end rather than at byte 0: dcrd rolls at 10 MB, and the
	// startup hint tails this file on a timer while the node is unreachable.
	seeked := false
	if window := int64(lines) * tailBytesPerLine; window > 0 {
		if st, serr := f.Stat(); serr == nil && st.Size() > window {
			if _, serr := f.Seek(st.Size()-window, io.SeekStart); serr == nil {
				seeked = true
			}
		}
	}

	// Read into a ring buffer of size `lines`. A bufio.Reader rather than a
	// Scanner: a Scanner stops at the first token over its buffer, so one
	// over-long line would hide every line after it, which is exactly when the
	// operator most wants to see what followed.
	r := bufio.NewReaderSize(f, 64*1024)

	// A seek lands mid-line, so the first line read is a partial one.
	if seeked {
		_, _ = readLogLine(r)
	}

	ring := make([]string, lines)
	count := 0
	for {
		line, err := readLogLine(r)
		if line != "" || err == nil {
			ring[count%lines] = line
			count++
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			break
		}
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
