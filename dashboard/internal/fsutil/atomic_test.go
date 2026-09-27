package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteJSONReplacesInPlaceAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gaming-spends.json")
	for _, body := range []string{`{"spends":[1]}`, `{"spends":[1,2]}`} {
		if err := AtomicWriteJSON(path, []byte(body)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != body {
			t.Fatalf("read back %q %v, want %q", got, err, body)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v %v", info, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("directory holds %v %v", entries, err)
	}
	if err := AtomicWriteJSON(filepath.Join(dir, "missing", "x.json"), []byte("{}")); err == nil {
		t.Fatal("wrote into a directory that does not exist")
	}
}
