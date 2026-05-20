// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// csppsolverState reflects whether dcrwallet successfully spawned its
// csppsolver child process at startup. dcrwallet logs a specific WARN at
// startup when the binary isn't on PATH; presence of that line → missing.
//
//	0 = unknown (log file not yet readable)
//	1 = active   (no warning in startup log content)
//	-1 = missing (warning was found)
var csppsolverState atomic.Int32

const csppsolverMissingMarker = "Unable to start csppsolver"

// CsppsolverState returns a human-readable status string.
func CsppsolverState() string {
	switch csppsolverState.Load() {
	case 1:
		return "active"
	case -1:
		return "missing"
	default:
		return "unknown"
	}
}

// RefreshCsppsolverStateIfUnknown re-runs the startup-banner probe if the
// state hasn't converged. Cheap and idempotent — once the state is "active"
// or "missing", subsequent calls no-op. Intended to be called from the
// PrivacyStatusHandler so the UI converges within one poll cycle if the
// initial probe ran before dcrwallet finished writing its banner.
func RefreshCsppsolverStateIfUnknown() {
	if csppsolverState.Load() == 0 {
		probeCsppsolverState()
	}
}

// dcrwallet writes its log file to ${appdata}/logs/${network}/dcrwallet.log.
// The dashboard container mounts /app-data read-only and the wallet container
// uses /app-data/dcrwallet as appdata, so the path is fixed here.
const walletLogPath = "/app-data/dcrwallet/logs/mainnet/dcrwallet.log"

// walletLogFilter selects mixer-relevant log entries we want surfaced to the
// frontend's MixerEventLog. Decrediton tails its wallet log for the same
// purpose (every 2s poll); we stream them as they're appended.
var walletLogFilter = regexp.MustCompile(
	`\[(DBG|INF|WRN|ERR|TRC|CRT)\] (MIXC|MIXP|TKBY)` + // mix client, mixpool, ticketbuyer
		`|\[INF\] WLLT: Mixing output ` + // mix-output start
		`|\[INF\] WLLT: Account .* (un)?locked`, // change-account state changes
)

// StartWalletLogTail begins streaming mixer-relevant dcrwallet log lines into
// the mixer event ring buffer. Idempotent — safe to call once at process
// start. Survives wallet container restarts and log rotation.
func StartWalletLogTail() {
	go tailWalletLog()
}

func tailWalletLog() {
	var f *os.File
	var lastSize int64
	var partial []byte

	for {
		// Open or reopen the file as needed.
		if f == nil {
			var err error
			f, err = os.Open(walletLogPath)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}
			// Probe csppsolver status from the historical startup section
			// of the log before we skip ahead.
			probeCsppsolverState()

			// Skip historical content on first open — we only want NEW lines.
			info, _ := f.Stat()
			lastSize = info.Size()
			_, _ = f.Seek(0, io.SeekEnd)
			log.Printf("Tailing dcrwallet log %s (starting at offset %d, csppsolver=%s)", walletLogPath, lastSize, CsppsolverState())
		}

		info, err := f.Stat()
		if err != nil {
			_ = f.Close()
			f = nil
			partial = nil
			time.Sleep(2 * time.Second)
			continue
		}

		// Detect rotation/truncation: if size shrank, the file was rotated
		// out from under us.
		if info.Size() < lastSize {
			_ = f.Close()
			f = nil
			lastSize = 0
			partial = nil
			continue
		}

		if info.Size() > lastSize {
			buf := make([]byte, info.Size()-lastSize)
			n, rerr := f.Read(buf)
			if n > 0 {
				data := append(partial, buf[:n]...)
				// Split into complete lines; carry the last partial (no
				// trailing newline) for the next iteration.
				idx := strings.LastIndexByte(string(data), '\n')
				if idx < 0 {
					partial = data
				} else {
					complete := string(data[:idx])
					partial = data[idx+1:]
					for _, line := range strings.Split(complete, "\n") {
						processWalletLogLine(line)
					}
				}
				lastSize += int64(n)
			}
			if rerr != nil && rerr != io.EOF {
				_ = f.Close()
				f = nil
				partial = nil
				continue
			}
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// probeCsppsolverState scans dcrwallet's log file once for the
// "Unable to start csppsolver" warning. The log file accumulates entries
// across multiple dcrwallet process lifetimes, so we look only at the
// segment after the LAST `[INF] DCRW: Version` line, which is the current
// process's startup banner.
func probeCsppsolverState() {
	data, err := os.ReadFile(walletLogPath)
	if err != nil {
		return
	}
	s := string(data)
	idx := strings.LastIndex(s, "[INF] DCRW: Version")
	if idx < 0 {
		// No banner yet; the process is still starting up. Stay "unknown".
		return
	}
	currentRun := s[idx:]
	if strings.Contains(currentRun, csppsolverMissingMarker) {
		csppsolverState.Store(-1)
		return
	}
	// We've seen the banner and no warning followed it within the captured
	// region. dcrwallet emits the warning immediately after the banner if
	// the spawn fails, so its absence within ~512 bytes past the banner
	// means csppsolver started.
	if len(currentRun) >= 512 {
		csppsolverState.Store(1)
	}
}

func processWalletLogLine(line string) {
	// Watch the live stream too in case csppsolver fails at some point
	// after startup (recompilation, file removal, etc.).
	if strings.Contains(line, csppsolverMissingMarker) {
		csppsolverState.Store(-1)
	}

	if line == "" || !walletLogFilter.MatchString(line) {
		return
	}
	level := "info"
	switch {
	case strings.Contains(line, "[ERR]"), strings.Contains(line, "[CRT]"):
		level = "error"
	case strings.Contains(line, "[WRN]"):
		level = "warn"
	}
	recordMixerEvent(level, line)
}
