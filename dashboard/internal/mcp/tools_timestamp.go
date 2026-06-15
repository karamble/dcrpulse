// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"

	"dcrpulse/internal/timestamp"
)

// timestampTools are the read-only "timestamp" domain tools, backed by the same
// dcrtime proof archive the timestamp page uses.
var timestampTools = []toolDef{
	readTool("timestamp", "timestamp_records",
		"List dcrtime timestamp records and their anchoring status.",
		func(_ context.Context, _ emptyInput) (any, error) {
			store, err := timestamp.Archive()
			if err != nil {
				return nil, err
			}
			return store.List(timestamp.Query{}), nil
		}),
}
