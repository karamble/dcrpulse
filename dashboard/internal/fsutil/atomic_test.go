// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

// onlyEntry fails the test unless dir holds exactly want, so a leftover temp
// file shows up whatever it is named.
func onlyEntry(t *testing.T, dir, want string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != want {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("directory holds %q, want only %q", names, want)
	}
}

func TestWriteFileAtomicReplacesWithMode(t *testing.T) {
	for _, perm := range []os.FileMode{0o600, 0o644} {
		dir := t.TempDir()
		path := filepath.Join(dir, "tor.json")
		for _, content := range []string{`{"rev":1}`, `{"rev":2}`} {
			if err := WriteFileAtomic(path, []byte(content), perm); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != content {
				t.Fatalf("read back %q, %v; want %q", got, err, content)
			}
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != perm {
			t.Fatalf("mode %v, want %v", fi.Mode().Perm(), perm)
		}
		onlyEntry(t, dir, "tor.json")
	}
}

// A write that cannot be renamed into place leaves the directory as it was.
func TestWriteFileAtomicCleansUpOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.json")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(target, []byte("{}"), 0o600); err == nil {
		t.Fatal("renaming over a directory succeeded")
	}
	onlyEntry(t, dir, "config.json")
}
