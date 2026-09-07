// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"strings"
	"testing"

	"github.com/decred/dcrd/rpcclient/v8"

	"dcrpulse/internal/rpc"
)

// The gate reads the poller's snapshot instead of asking dcrd per request. It
// is a courtesy, not a control: it turns a confusing wallet error into a clear
// one, so any state the old per-request check let through must still pass. The
// snapshot has seven statuses, and two of the pass-through ones are easy to get
// wrong - the empty one, which is permanent on an install with no dcrd
// credentials, and "error", which was never "unreachable".

// disconnectedRPC is a *rpcclient.Client whose every call fails at once: HTTP
// post mode to a port nothing listens on, so New does no I/O. The concrete
// client vars cannot be faked any other way.
func disconnectedRPC(t *testing.T) *rpcclient.Client {
	t.Helper()
	c, err := rpcclient.New(&rpcclient.ConnConfig{
		Host: "127.0.0.1:1", User: "u", Pass: "p",
		HTTPPostMode: true, DisableTLS: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Shutdown)
	return c
}

func withDcrdClient(t *testing.T, c *rpcclient.Client) {
	t.Helper()
	prev := rpc.DcrdClient
	rpc.DcrdClient = c
	t.Cleanup(func() { rpc.DcrdClient = prev })
}

func withWalletClient(t *testing.T, c *rpcclient.Client) {
	t.Helper()
	prev := rpc.WalletClient
	rpc.WalletClient = c
	t.Cleanup(func() { rpc.WalletClient = prev })
}

func TestNodeWalletGateStates(t *testing.T) {
	withDcrdClient(t, disconnectedRPC(t))

	for _, tc := range []struct {
		status string
		want   NodeWalletGate
	}{
		{"running", GateOK},
		{"syncing", GateIBD},
		{"connecting", GateIBD},
		{"starting", GateUnreachable},
		{"upgrading", GateUnreachable},
		// Reachable but refusing was never "unreachable"; it fell through
		// to the wallet before and still must.
		{"error", GateOK},
		// The poller has not run yet, or never will (no dcrd credentials).
		{"", GateOK},
	} {
		t.Run("status="+tc.status, func(t *testing.T) {
			t.Cleanup(SetNodeSyncSnapshotForTest(NodeSyncSnapshot{Status: tc.status, StartupNote: "note"}))
			got, reason := NodeWalletGateState()
			if got != tc.want {
				t.Fatalf("gate = %v, want %v", got, tc.want)
			}
			if got != GateOK && reason == "" {
				t.Fatal("a refusal with no reason would answer 503 with an empty body")
			}
			if got == GateOK && reason != "" {
				t.Fatalf("proceeding, yet a reason was returned: %q", reason)
			}
		})
	}
}

// Without dcrd credentials the client is nil and the poller never starts, so
// the snapshot stays empty forever. The old check skipped itself on a nil
// client; the gate must too, or every wallet page 503s on that install.
func TestNodeWalletGateFailsOpenWithoutAClient(t *testing.T) {
	withDcrdClient(t, nil)
	t.Cleanup(SetNodeSyncSnapshotForTest(NodeSyncSnapshot{Status: "syncing"}))

	if got, _ := NodeWalletGateState(); got != GateOK {
		t.Fatalf("gate = %v with no dcrd client, want %v", got, GateOK)
	}
}

func TestNodeWalletGateReasonIsNeverEmpty(t *testing.T) {
	withDcrdClient(t, disconnectedRPC(t))
	// notReadySnapshot always fills StartupNote, but a hand-built or
	// half-initialised snapshot might not.
	t.Cleanup(SetNodeSyncSnapshotForTest(NodeSyncSnapshot{Status: "starting"}))

	got, reason := NodeWalletGateState()
	if got != GateUnreachable || reason == "" {
		t.Fatalf("gate = %v reason = %q, want a refusal with a message", got, reason)
	}
	t.Cleanup(SetNodeSyncSnapshotForTest(NodeSyncSnapshot{Status: "starting", StartupNote: "dcrd is upgrading its database"}))
	if _, reason := NodeWalletGateState(); reason != "dcrd is upgrading its database" {
		t.Fatalf("the poller's own note was not passed through: %q", reason)
	}
}

func TestWalletReadyRefusesDuringIBD(t *testing.T) {
	withDcrdClient(t, disconnectedRPC(t))
	withWalletClient(t, disconnectedRPC(t))
	t.Cleanup(SetNodeSyncSnapshotForTest(NodeSyncSnapshot{Status: "syncing"}))

	ready, reason := WalletReady(context.Background())
	if ready {
		t.Fatal("feature setup was allowed during initial block download")
	}
	if !strings.Contains(reason, "still downloading the blockchain") {
		t.Fatalf("reason = %q, want the IBD wording", reason)
	}
}

// WalletReady never refused on an unreachable dcrd - it fell through to the
// wallet check, which reports the wallet as not ready. Sharing the gate with the
// handlers must not quietly change that.
func TestWalletReadyIgnoresUnreachableDcrd(t *testing.T) {
	withDcrdClient(t, disconnectedRPC(t))
	withWalletClient(t, disconnectedRPC(t))
	t.Cleanup(SetNodeSyncSnapshotForTest(NodeSyncSnapshot{Status: "starting", StartupNote: "dcrd is starting"}))

	ready, reason := WalletReady(context.Background())
	if ready {
		t.Fatal("ready with no wallet reachable")
	}
	if strings.Contains(reason, "dcrd is starting") {
		t.Fatalf("WalletReady now refuses on an unreachable dcrd: %q", reason)
	}
	// FetchWalletStatus folds a failed RPC into a "no_wallet" status rather
	// than an error, so the fall-through lands in the sync-message branch.
	if !strings.Contains(reason, "Wallet not available") {
		t.Fatalf("reason = %q, want the fall-through wallet wording", reason)
	}
}
