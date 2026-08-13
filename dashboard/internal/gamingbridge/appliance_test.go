// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These guard the shape of the appliance rather than its behaviour.
//
// Be clear about what they are worth. Running code cannot prove that this
// appliance never executes a game: a test can only show that the machinery for
// doing so is absent, and machinery can be added back. Nor can it prove anything
// about NAT, which is a property of a deployment and not of a program. What
// these do is make the deletion permanent - the sandbox was removed on purpose,
// and a change that quietly reintroduced it would otherwise look like a feature.

// repoRoot is the checkout these tests are running out of.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("locate the checkout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "docker-compose.yml")); err != nil {
		t.Skipf("not running out of a checkout, so there is nothing to inspect: %v", err)
	}
	return root
}

// The appliance carries no game.
//
// A game is somebody else's program, and the reason this bridge is worth having
// is that it holds keys while running none of it. A tree of game code shipped
// inside the appliance would put the two back in the same trust boundary, which
// is the arrangement this conversion exists to end.
//
// This asserts the absence of the tree, which is not the same as proving no
// game can ever run here.
func TestTheApplianceCarriesNoGame(t *testing.T) {
	root := repoRoot(t)
	for _, gone := range []string{"gaming", "dashboard/internal/gaming"} {
		if _, err := os.Stat(filepath.Join(root, gone)); err == nil {
			t.Errorf("%s is back: the appliance is carrying game code again, so a game's bugs are "+
				"once more the wallet's problem", gone)
		}
	}
}

// The stack starts no game.
//
// The compose file is the whole of what an operator runs. A game service in it
// would mean installing dcrpulse installs a game, downloaded and started by
// something the operator did not choose.
func TestTheStackStartsNoGame(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read the compose file: %v", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		// A service, volume or network named for gaming. The bridge's own
		// published port is a port line, not a name, so it does not match.
		if strings.Contains(trimmed, "gaming") && strings.HasSuffix(trimmed, ":") {
			t.Errorf("the stack declares %q: installing dcrpulse would start a game the operator "+
				"never chose", trimmed)
		}
	}
}

// The bridge runs nothing.
//
// The appliance holds the keys. A path from a connected game to starting a
// process on this host would make the entire policy model - caps, approval, the
// App Password gate - a thing to be walked around rather than through.
func TestTheApplianceRunsNothing(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range []string{"dashboard/cmd", "dashboard/internal", "dashboard/pkg"} {
		for file, imports := range goImports(t, filepath.Join(root, dir)) {
			for _, imp := range imports {
				if imp == "os/exec" {
					t.Errorf("%s runs processes: a bridge that can start programs is a bridge whose "+
						"spending limits can be stepped around", rel(root, file))
				}
			}
		}
	}
}

// The bridge never reaches out to a game.
//
// Games connect in, and that direction is the claim: a game needs no inbound
// port, no forwarding and no route back to it, and the appliance keeps no list
// of addresses to try. A dialer here would quietly restore the arrangement the
// conversion removed, where the appliance went looking for game processes it
// expected to find.
func TestTheBridgeNeverReachesOutToAGame(t *testing.T) {
	root := repoRoot(t)
	dialers := []string{"net.Dial", "grpc.NewClient", "grpc.Dial", "http.Get", "http.Post"}
	dir := filepath.Join(root, "dashboard/internal/gamingbridge")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read the bridge package: %v", err)
	}
	for _, e := range entries {
		// The tests dial on purpose - they are standing in for the game.
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, d := range dialers {
			if strings.Contains(string(b), d) {
				t.Errorf("%s calls %s: the bridge is reaching out to something, so a game would "+
					"need a reachable address after all", e.Name(), d)
			}
		}
	}
}

// goImports maps each Go file under dir to what it imports.
func goImports(t *testing.T, dir string) map[string][]string {
	t.Helper()
	out := make(map[string][]string)
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil // generated or unparseable files are not what this is about
		}
		for _, imp := range f.Imports {
			out[path] = append(out[path], strings.Trim(imp.Path.Value, `"`))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}

func rel(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return r
	}
	return path
}
