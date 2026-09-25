// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"testing"

	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

func TestSyncFromChainInfo(t *testing.T) {
	tests := []struct {
		name          string
		ci            chainjson.GetBlockChainInfoResult
		status, phase string
		msg           string
		progress      float64
	}{
		{"synced", chainjson.GetBlockChainInfoResult{Blocks: 100, Headers: 100, SyncHeight: 100},
			"running", "synced", "Fully synced", 100},
		{"no peer height yet", chainjson.GetBlockChainInfoResult{InitialBlockDownload: true, Blocks: 100, Headers: 100},
			"connecting", "starting", "Looking for peers", 0},
		{"headers", chainjson.GetBlockChainInfoResult{InitialBlockDownload: true, Blocks: 10, Headers: 50, SyncHeight: 200},
			"syncing", "headers", "Fetching block headers: 50 of 200", 25},
		{"blocks", chainjson.GetBlockChainInfoResult{InitialBlockDownload: true, Blocks: 150, Headers: 200, SyncHeight: 200},
			"syncing", "blocks", "Downloading blocks: 150 of 200", 75},
		// The whole chain is on disk but no peer has confirmed the tip yet.
		{"chain complete, waiting for peers", chainjson.GetBlockChainInfoResult{InitialBlockDownload: true, Blocks: 1118284, Headers: 1118284, SyncHeight: 1118284},
			"connecting", "starting", "Waiting for peers", 0},
	}
	for _, tt := range tests {
		got := syncFromChainInfo(&tt.ci)
		if got.Status != tt.status || got.SyncPhase != tt.phase || got.SyncMessage != tt.msg || got.SyncProgress != tt.progress {
			t.Errorf("%s: got %s/%s %q %.1f, want %s/%s %q %.1f", tt.name,
				got.Status, got.SyncPhase, got.SyncMessage, got.SyncProgress, tt.status, tt.phase, tt.msg, tt.progress)
		}
		if got.Blocks != tt.ci.Blocks || got.SyncHeight != tt.ci.SyncHeight {
			t.Errorf("%s: heights not carried", tt.name)
		}
	}
}
