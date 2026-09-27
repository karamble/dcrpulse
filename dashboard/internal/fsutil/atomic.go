// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package fsutil holds the shared atomic file write. Four stores carried their
// own copy of the temp-write-chmod-rename sequence, which is exactly the kind
// of path that drifts silently.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriteJSON writes data to path via a temp file in the same directory,
// chmods it 0600, syncs it and renames it into place, then syncs the directory,
// so a crash leaves either the old file or the new one, never a torn or lost
// write. The directory must already exist.
func AtomicWriteJSON(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
