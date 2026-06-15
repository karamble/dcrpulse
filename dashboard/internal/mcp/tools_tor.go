// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/services"
)

// torTools are the read-only "tor" domain tools. The snapshot getters do not
// return an error, so they are wrapped with a nil error.
var torTools = []toolDef{
	readTool("tor", "tor_status",
		"Get the Tor proxy reachability and per-daemon routing state.",
		func(_ context.Context, _ emptyInput) (any, error) { return services.TorStatusSnapshot(), nil }),
	readTool("tor", "tor_control",
		"Get live Tor control-port data (bootstrap percentage, circuits, traffic, version).",
		func(_ context.Context, _ emptyInput) (any, error) { return services.TorControlSnapshot(), nil }),
	readTool("tor", "tor_settings",
		"Get the current Tor configuration (enabled, stream isolation, onion service, circuit limit).",
		func(_ context.Context, _ emptyInput) (any, error) { return services.ReadTorSettings(), nil }),
}
