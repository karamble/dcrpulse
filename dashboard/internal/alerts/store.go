// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package alerts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"dcrpulse/internal/config"
)

const storeSchemaVersion = 1

// alertsFile is the on-disk JSON shape, entries in append order (oldest
// first).
type alertsFile struct {
	SchemaVersion int      `json:"schemaVersion"`
	Entries       []*Entry `json:"entries"`
}

func loadEntries(path string) ([]*Entry, error) {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("read alerts file %s: %w", path, err)
	case len(data) == 0:
		return nil, nil
	}
	var af alertsFile
	if err := json.Unmarshal(data, &af); err != nil {
		return nil, fmt.Errorf("parse alerts file %s: %w", path, err)
	}
	return af.Entries, nil
}

// saveLocked rewrites the whole file atomically. Caller holds e.mu. Save
// failures are logged, not returned: producers fire and forget, and the
// engine keeps working from memory.
func (e *engine) saveLocked() {
	af := alertsFile{SchemaVersion: storeSchemaVersion, Entries: e.entries}
	data, err := json.MarshalIndent(af, "", "  ")
	if err != nil {
		log.Printf("alerts: encode: %v", err)
		return
	}
	path := config.AlertsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		log.Printf("alerts: create dir: %v", err)
		return
	}
	if err := atomicWriteJSON(path, data); err != nil {
		log.Printf("alerts: save: %v", err)
	}
}

// atomicWriteJSON writes data to path via temp file + rename, mode 0600. Mirrors
// config.atomicWriteJSON (kept local so this package stays self-contained).
func atomicWriteJSON(path string, data []byte) error {
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
	return nil
}
