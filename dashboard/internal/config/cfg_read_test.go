// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Absent means "not written yet"; an empty file is never written, so it is a
// broken file and must not read as an empty configuration.
func TestReadRawJSONEmptyIsUnreadable(t *testing.T) {
	dir := t.TempDir()
	if raw, err := readRawJSON(filepath.Join(dir, "absent.json")); err != nil || len(raw) != 0 {
		t.Fatalf("absent: %v, %v; want an empty document", raw, err)
	}
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRawJSON(empty); err == nil {
		t.Fatal("an empty file read as a document")
	}
	obj := filepath.Join(dir, "obj.json")
	if err := os.WriteFile(obj, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if raw, err := readRawJSON(obj); err != nil || len(raw) != 0 {
		t.Fatalf("{}: %v, %v; want an empty document", raw, err)
	}
}
