// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import "github.com/decred/slog"

// gameLog is where the listener writes. It says nothing until UseLogger.
var gameLog = slog.Disabled

// UseLogger sets the logger the listener writes to.
func UseLogger(logger slog.Logger) { gameLog = logger }
