// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

// Log tails below are copied from real dcrlnd output, not built from the
// marker tables, so the test still fails if a marker stops matching.
func TestStartupUpgradeInProgressDcrlnd(t *testing.T) {
	starts, ready := startupMarkers(LogComponentDcrlnd)

	tests := []struct {
		name  string
		lines []string
		want  bool
	}{{
		// A brand-new channeldb runs its migrations while being created, and
		// dcrlnd logs "RPC server listening" before that, so the schema lines
		// trail the only other ready marker. This is the fresh-wallet case
		// that used to be reported as an upgrade that never finished.
		name: "fresh database creation is not an upgrade",
		lines: []string{
			"2026-09-03 12:45:22.945 [INF] RPCS: RPC server listening on 0.0.0.0:10009",
			"2026-09-03 12:45:22.948 [INF] LTND: Opening the main database, this might take a few minutes...",
			"2026-09-03 12:45:23.012 [INF] CHDB: Checking for schema update: latest_version=24, db_version=24",
			"2026-09-03 12:45:23.013 [INF] CHDB: Performing decred-specific database schema migration",
			"2026-09-03 12:45:23.013 [INF] CHDB: Applying migration #1",
			"2026-09-03 12:45:23.013 [INF] CHDB: Migrating 0 entries",
			"2026-09-03 12:45:23.055 [INF] LTND: Database(s) now open (time_to_open=106.313437ms)!",
			"2026-09-03 12:45:23.055 [INF] LTND: Waiting for wallet encryption password. Use `lncli create` to create a wallet",
		},
		want: false,
	}, {
		// A real post-version-bump upgrade: the tail ends inside the schema
		// work, with nothing after it.
		name: "unfinished upgrade is reported",
		lines: []string{
			"2026-09-03 12:45:22.945 [INF] RPCS: RPC server listening on 0.0.0.0:10009",
			"2026-09-03 12:45:22.948 [INF] LTND: Opening the main database, this might take a few minutes...",
			"2026-09-03 12:45:23.013 [INF] CHDB: Performing database schema migration",
			"2026-09-03 12:45:23.013 [INF] CHDB: Applying migration #3",
		},
		want: true,
	}, {
		name: "finished upgrade still in the tail is not reported",
		lines: []string{
			"2026-09-03 12:45:23.013 [INF] CHDB: Performing database schema migration",
			"2026-09-03 12:45:23.013 [INF] CHDB: Applying migration #3",
			"2026-09-03 12:45:23.055 [INF] LTND: Database(s) now open (time_to_open=9.1s)!",
			"2026-09-03 12:45:24.100 [INF] RPCS: RPC server listening on 0.0.0.0:10009",
		},
		want: false,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, line := startupUpgradeInProgress(tc.lines, starts, ready)
			if got != tc.want {
				t.Fatalf("upgrade in progress = %v, want %v (start line %q)", got, tc.want, line)
			}
		})
	}
}
