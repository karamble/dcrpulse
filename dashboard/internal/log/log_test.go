// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package log

import (
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/decred/slog"
)

func TestRotatorCapturesSubsystemOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "dcrpulse.log")
	if err := InitRotator(path); err != nil {
		t.Fatalf("InitRotator: %v", err)
	}
	defer CloseRotator()

	DCRP.Infof("rotator test line %d", 42)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "[INF] DCRP: rotator test line 42") {
		t.Fatalf("log file missing the subsystem line, got: %q", data)
	}
}

func TestInitRotatorIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.log")
	if err := InitRotator(first); err != nil {
		t.Fatalf("InitRotator: %v", err)
	}
	defer CloseRotator()

	// A second call must not swap the open file out from under the writer.
	if err := InitRotator(filepath.Join(dir, "second.log")); err != nil {
		t.Fatalf("second InitRotator: %v", err)
	}
	DCRP.Info("still the first file")

	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "still the first file") {
		t.Fatalf("second InitRotator replaced the rotator, got: %q", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "second.log")); !os.IsNotExist(err) {
		t.Fatal("second InitRotator opened a file it should have skipped")
	}
	CloseRotator()
	CloseRotator()
}

func TestSetDebugLevel(t *testing.T) {
	t.Cleanup(func() { SetDebugLevel("info") })

	if err := SetDebugLevel("info,MSIG=debug"); err != nil {
		t.Fatalf("SetDebugLevel: %v", err)
	}
	if got := MSIG.Level(); got != slog.LevelDebug {
		t.Fatalf("MSIG level = %v, want debug", got)
	}
	if got := WLLT.Level(); got != slog.LevelInfo {
		t.Fatalf("WLLT level = %v, want info", got)
	}

	if err := SetDebugLevel("info,NOPE=debug"); err == nil {
		t.Fatal("unknown subsystem accepted")
	}
	if err := SetDebugLevel("bogus"); err == nil {
		t.Fatal("unknown level accepted")
	}
}

// TestConcurrentWrites is the guard for the rotator's unsynchronised size
// accounting: it only misbehaves under -race with more than one writer.
func TestConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "dcrpulse.log")
	if err := InitRotator(path); err != nil {
		t.Fatalf("InitRotator: %v", err)
	}
	defer CloseRotator()
	RedirectStdLog(DCRP)
	defer stdlog.SetOutput(os.Stderr)

	errLog := StdErrorLogger(HTTP)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				WLLT.Infof("subsystem %d/%d", n, j)
				stdlog.Printf("bridged %d/%d", n, j)
				errLog.Printf("server %d/%d", n, j)
			}
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	for _, want := range []string{"[INF] WLLT:", "[INF] DCRP: bridged", "[ERR] HTTP: server"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("log file missing %q", want)
		}
	}
	// The stdlib bridge must not leave its own timestamp or a blank line.
	if strings.Contains(string(data), "\n\n") {
		t.Fatal("bridged lines left blank lines behind")
	}
}
