// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"dcrpulse/internal/config"
)

// currentWalletLogPath returns the active wallet's dcrwallet log file. dcrwallet
// writes to ${appdata}/logs/${network}/dcrwallet.log; the appdata follows the
// selected wallet so the mixer feed tracks wallet switches.
func currentWalletLogPath() string {
	name := ActiveWalletName()
	if name == "" {
		name = config.DefaultWalletName
	}
	return filepath.Join(config.ResolveWalletAppdata(name), "logs", "mainnet", "dcrwallet.log")
}

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
	var openPath string

	for {
		// Reopen when the active wallet (and thus its log path) changes.
		if f != nil && openPath != currentWalletLogPath() {
			_ = f.Close()
			f = nil
			partial = nil
		}

		// Open or reopen the file as needed.
		if f == nil {
			var err error
			openPath = currentWalletLogPath()
			f, err = os.Open(openPath)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}
			// Skip historical content on first open — we only want NEW lines.
			info, _ := f.Stat()
			lastSize = info.Size()
			_, _ = f.Seek(0, io.SeekEnd)
			log.Printf("Tailing dcrwallet log %s (starting at offset %d)", openPath, lastSize)
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

func processWalletLogLine(line string) {
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
