// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// cfgWriteMu serializes the read-modify-write of every config document.
// Each writer loads its own instance, so the per-instance mutex cannot do it:
// without this, two writers that overlap silently drop one set of keys.
var cfgWriteMu sync.Mutex

// readRawJSON decodes a config document. An absent or empty file yields an
// empty document, matching the lazily-created files both config types use.
func readRawJSON(path string) (map[string]json.RawMessage, error) {
	raw := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return raw, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if len(data) == 0 {
		return raw, nil
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return raw, nil
}

// mergeSave rewrites path with the document currently on disk, overlaid with
// the keys this instance staged: those present in raw are written, those absent
// were deleted. Keys another writer added meanwhile are left alone. Returns the
// merged document so the caller can adopt it.
//
// Callers must hold cfgWriteMu.
func mergeSave(path string, raw map[string]json.RawMessage, dirty map[string]bool) (map[string]json.RawMessage, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	merged, err := readRawJSON(path)
	if err != nil {
		return nil, err
	}
	for key := range dirty {
		if v, ok := raw[key]; ok {
			merged[key] = v
		} else {
			delete(merged, key)
		}
	}
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode config %s: %w", path, err)
	}
	if err := atomicWriteJSON(path, data); err != nil {
		return nil, err
	}
	return merged, nil
}
