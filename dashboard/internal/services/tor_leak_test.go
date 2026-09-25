// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"dcrpulse/internal/types"
)

func pinLiquidityTor(t *testing.T, on bool) {
	t.Helper()
	prev := liquidityTorEnabled
	liquidityTorEnabled = func() bool { return on }
	t.Cleanup(func() { liquidityTorEnabled = prev })
}

// A liquidity request carries this node's Lightning identity and the provider
// client cannot be routed through Tor, so with Tor on nothing may be sent at
// all: not the request and not a local DNS lookup of the provider.
func TestLiquidityIsRefusedWhileTorIsOn(t *testing.T) {
	pinLiquidityTor(t, true)
	withLightning(t, nil)
	unresolvable := "https://lp.invalid"

	if _, err := EstimateLiquidityChannel(context.Background(),
		&types.RequestLiquidityEstimateRequest{Server: unresolvable, ChanSizeAtoms: 1e8}); !errors.Is(err, ErrLiquidityOverTor) {
		t.Fatalf("estimate: err = %v, want %v", err, ErrLiquidityOverTor)
	}
	if _, err := RequestLiquidityChannel(context.Background(),
		&types.RequestLiquidityRequest{Server: unresolvable, ChanSizeAtoms: 1e8}, ApprovedFeeCeiling(0)); !errors.Is(err, ErrLiquidityOverTor) {
		t.Fatalf("request: err = %v, want %v", err, ErrLiquidityOverTor)
	}

	pinLiquidityTor(t, false)
	if _, err := EstimateLiquidityChannel(context.Background(),
		&types.RequestLiquidityEstimateRequest{Server: unresolvable, ChanSizeAtoms: 1e8}); errors.Is(err, ErrLiquidityOverTor) {
		t.Fatal("with Tor off the estimate is still refused as a Tor leak")
	}
}

// An unknown Tor setting must not read as off: off sends everything over
// clearnet. A missing pointer is the one case that means Tor was never on.
func TestAnUnreadableTorSettingReadsAsOn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tor.json")

	if s := readTorSettings(path); s.Enabled {
		t.Fatal("a missing pointer reads as Tor on")
	}
	if err := os.WriteFile(path, []byte(`{"enabled":false,"isolation":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := readTorSettings(path); s.Enabled {
		t.Fatal("an explicit off reads as on")
	}
	if err := os.WriteFile(path, []byte(`{"enabled":tru`), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := readTorSettings(path); !s.Enabled {
		t.Fatal("a torn pointer reads as Tor off")
	}
	if err := os.Chmod(path, 0); err == nil && os.Geteuid() != 0 {
		if s := readTorSettings(path); !s.Enabled {
			t.Fatal("an unreadable pointer reads as Tor off")
		}
	}
}

func TestWriteFileSyncedReplacesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tor.json")
	for _, content := range []string{`{"rev":1}`, `{"rev":2}`} {
		if err := writeFileSynced(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != content {
			t.Fatalf("read back %q, %v; want %q", got, err, content)
		}
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("the temporary file was left behind")
	}
}
