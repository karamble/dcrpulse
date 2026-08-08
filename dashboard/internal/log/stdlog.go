// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package log

import (
	stdlog "log"
	"strings"

	"github.com/decred/slog"
)

// stdWriter routes standard-library log output through a subsystem logger.
// Going through the logger rather than the backend writer is what keeps the
// rotator's unsynchronised size accounting behind slog's own lock.
type stdWriter struct {
	log   slog.Logger
	isErr bool
}

func (w stdWriter) Write(p []byte) (int, error) {
	// The stdlib logger terminates every line itself; slog adds its own.
	line := strings.TrimRight(string(p), "\n")
	if line != "" {
		if w.isErr {
			// Non-formatting variant: a percent sign in a request path must
			// never be read as a verb.
			w.log.Error(line)
		} else {
			w.log.Info(line)
		}
	}
	return len(p), nil
}

// StdErrorLogger adapts a subsystem logger to a standard-library logger that
// emits at error level, for net/http's Server.ErrorLog.
func StdErrorLogger(l slog.Logger) *stdlog.Logger {
	return stdlog.New(stdWriter{log: l, isErr: true}, "", 0)
}

// RedirectStdLog points the standard-library default logger at a subsystem
// logger. Kept permanently so output from dependencies that reach for the
// stdlib logger stays in the log file. Flags are cleared because slog stamps
// its own timestamp.
func RedirectStdLog(l slog.Logger) {
	stdlog.SetFlags(0)
	stdlog.SetOutput(stdWriter{log: l})
}
