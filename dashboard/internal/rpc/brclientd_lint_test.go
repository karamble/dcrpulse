// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// builderFile is the only file allowed to assemble a brclientd URL.
const builderFile = "brclientd_url.go"

// pathArgs names, for each function that receives a request path, which of its
// arguments carry it.
var pathArgs = map[string][]int{
	"brclientdPostJSON":      {1},
	"brclientdPostJSONRaw":   {1},
	"brclientdGetRaw":        {1},
	"brclientdGetRawLimit":   {1},
	"brclientdDoPostJSONRaw": {2},
	"brclientdUpload":        {2},
	"brclientdEndpoint":      {1},
	"brclientdWSEndpoint":    {0},
	"brclientdURL":           {2},
	"brclientdRoute":         {0, 2},
	"brclientdPostJSONID":    {1, 3},
	"brclientdGetRawID":      {1, 3},
}

// parsePackage returns the package's non-test files. Tests are excluded; they
// drive the builder with computed values on purpose.
func parsePackage(t *testing.T) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	files := map[string]*ast.File{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		files[filepath.Base(path)] = f
	}
	if len(files) == 0 {
		t.Fatal("parsed no files; this test would pass by inspecting nothing")
	}
	return fset, files
}

// TestBrclientdPathArgsAreNotComputed rejects a path argument built at the call
// site: a concatenation, or any call including a brPath conversion. A bare
// identifier passes, since a brPath variable can only come from a literal or
// from brclientdRoute.
//
// Kills: brclientdPostJSON(ctx, brPath("/gc/"+gcid+"/message"), nil), and the
// fmt.Sprintf equivalent, anywhere in the package.
func TestBrclientdPathArgsAreNotComputed(t *testing.T) {
	fset, files := parsePackage(t)

	inspected := 0
	for name, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			// A conversion would turn a runtime string into a path.
			if fn.Name == "brPath" && name != builderFile {
				t.Errorf("%s: brPath conversion outside %s puts a runtime string in a URL",
					fset.Position(call.Pos()), builderFile)
				return true
			}
			positions, ok := pathArgs[fn.Name]
			if !ok {
				return true
			}
			for _, i := range positions {
				if i >= len(call.Args) {
					continue
				}
				inspected++
				switch call.Args[i].(type) {
				case *ast.BasicLit, *ast.Ident:
					// A literal, or a brPath being forwarded.
				default:
					t.Errorf("%s: argument %d of %s is computed; a path is written "+
						"out or validated, never assembled at the call site",
						fset.Position(call.Args[i].Pos()), i, fn.Name)
				}
			}
			return true
		})
	}

	// The package has well over a hundred such call sites; a collapse means
	// the walk stopped matching.
	if inspected < 60 {
		t.Fatalf("inspected only %d path arguments; the walk is not reaching the call sites", inspected)
	}
}

// TestNoHandRolledBrclientdURLs keeps a brclientd URL from being spelled
// outside the builder.
//
// Kills: adding an endpoint with its own fmt.Sprintf("https://%s:%s/foo", ...),
// which skips the two-port split and the IPv6 bracketing.
func TestNoHandRolledBrclientdURLs(t *testing.T) {
	entries, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	scanned, builderMatched := 0, false
	for _, path := range entries {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		scanned++
		// Both spellings of "builds a URL here": a full prefix literal, and a
		// url.URL with its own Scheme.
		hit := strings.Contains(string(src), `"https://`) ||
			strings.Contains(string(src), `"wss://`) ||
			strings.Contains(string(src), `Scheme: "`)
		if path == builderFile {
			// Sentinel on the construction rather than on the banned pattern:
			// the builder assembles a url.URL and never writes a scheme prefix,
			// so matching that proves the file was read.
			builderMatched = strings.Contains(string(src), "url.URL{")
			continue
		}
		// Other daemons share this package and are out of scope.
		if hit && strings.HasPrefix(path, "brclientd") {
			t.Errorf("%s spells a brclientd scheme itself; %s is where a URL is made", path, builderFile)
		}
	}

	if scanned == 0 {
		t.Fatal("scanned no files; a broken glob must fail rather than pass")
	}
	if !builderMatched {
		t.Fatalf("%s builds no url.URL, so the scan is reading the wrong file", builderFile)
	}
}
