// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A game must not be shown what brclientd wrote. brclientdPostJSON puts the
// upstream response body in its error text, so an unmarked error carrying one
// is the shape this has to stop.
//
// Kills: passing err.Error() straight to status.Error in any bridge method.
func TestAGameIsNotToldWhatTheDaemonSaid(t *testing.T) {
	leak := errors.New(`brclientd /gc/aa/message: HTTP 200: {"contacts":[{"nick":"alice","uid":"beef"}]}`)

	err := gameErr("poker", "SendFrame", codes.InvalidArgument, leak)
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("not a status error: %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("code is %s, want Unavailable", st.Code())
	}
	for _, secret := range []string{"contacts", "alice", "beef", "HTTP", "brclientd", "/gc/"} {
		if strings.Contains(st.Message(), secret) {
			t.Errorf("the message carries %q: %s", secret, st.Message())
		}
	}
}

// The other half: an error written for a game still reaches it, at the code the
// caller chose. Without this the next pass replaces everything with one string
// and games lose the diagnostics they can act on.
//
// Kills: a gameErr that genericises unconditionally, and one that returns the
// outer wrapper's text rather than the marked error's.
func TestAnErrorWrittenForAGameStillReachesIt(t *testing.T) {
	err := gameErr("poker", "SendFrame", codes.InvalidArgument, ErrSpendOverCap)
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code is %s, want the one the caller chose", st.Code())
	}
	if st.Message() != "over a cap" {
		t.Errorf("message is %q, want the marked text", st.Message())
	}

	// A marked error wrapped by a caller that never vetted the added words:
	// only the marked subtree may be disclosed.
	wrapped := fmt.Errorf("writing /app-data/control/spends.json: %w", GameSafe(errors.New("over a cap")))
	st, _ = status.FromError(gameErr("poker", "RequestSpend", codes.ResourceExhausted, wrapped))
	if strings.Contains(st.Message(), "app-data") {
		t.Errorf("an unvetted wrapper reached the game: %s", st.Message())
	}
}

// The two refusals a game can act on keep their codes; anything else does not
// become one of them.
//
// Kills: routing the sentinel branches through the default arm, which would
// tell a game its request was over a limit when the section is simply not set
// up to pay.
func TestSpendRefusalsKeepTheirCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"over cap", ErrSpendOverCap, codes.ResourceExhausted},
		{"not found", ErrSpendNotFound, codes.NotFound},
		{"anything else", errors.New("write /app-data/control/gaming-spends.json: permission denied"), codes.Unavailable},
	}
	for _, tc := range cases {
		st, _ := status.FromError(spendErr("poker", tc.err))
		if st.Code() != tc.want {
			t.Errorf("%s: code is %s, want %s", tc.name, st.Code(), tc.want)
		}
		if strings.Contains(st.Message(), "app-data") {
			t.Errorf("%s: a host path reached the game: %s", tc.name, st.Message())
		}
	}
}

// TestNoMethodHandsAGameAnErrorText makes the rule mechanical: every error a
// game receives goes through gameErr, so a method added later cannot forward
// a daemon's words by writing the obvious line.
//
// Kills: adding status.Error(codes.Internal, err.Error()) to any method.
func TestNoMethodHandsAGameAnErrorText(t *testing.T) {
	const gate = "gameerr.go"

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	fset := token.NewFileSet()
	inspected, sawGate := 0, false
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		if path == gate {
			sawGate = true
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "status" {
				return true
			}
			if sel.Sel.Name != "Error" && sel.Sel.Name != "Errorf" {
				return true
			}
			inspected++
			for _, arg := range call.Args {
				inner, ok := arg.(*ast.CallExpr)
				if !ok {
					continue
				}
				if s, ok := inner.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "Error" {
					t.Errorf("%s: an error's own text goes to a game; route it through gameErr",
						fset.Position(arg.Pos()))
				}
			}
			return true
		})
	}

	if !sawGate {
		t.Fatalf("%s is missing, so the rule has nothing to point at", gate)
	}
	if inspected < 3 {
		t.Fatalf("inspected only %d status calls; the walk is not reaching them", inspected)
	}
}
