// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"errors"
	"testing"
)

// Where a game is reached comes from the portal's own report, never from
// anything a caller supplies: the sandbox has no route out, so what it says
// about itself is the only account there is.
func TestAGameIsReachedWhereTheSandboxSaysItIs(t *testing.T) {
	st := GamingState{
		Running: map[string]int{"poker": 42},
		Ports:   map[string]int{"poker": 8790},
	}
	got, err := gamingGameURL(st, "poker", true)
	if err != nil {
		t.Fatalf("poker: %v", err)
	}
	if want := "http://gaming:8790"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAGameThatIsNotUpHasNowhereToBeReached(t *testing.T) {
	for _, tc := range []struct {
		name      string
		state     GamingState
		installed bool
		want      error
	}{
		{"not added", GamingState{}, false, ErrGamingGameNotInstalled},
		{"added, nothing running", GamingState{}, true, ErrGamingGameNotRunning},
		{
			"running but no port reported",
			GamingState{Running: map[string]int{"poker": 42}},
			true, ErrGamingGameNotRunning,
		},
		{
			"port reported but not running",
			GamingState{Ports: map[string]int{"poker": 8790}},
			true, ErrGamingGameNotRunning,
		},
		{
			"reported with no pid",
			GamingState{
				Running: map[string]int{"poker": 0},
				Ports:   map[string]int{"poker": 8790},
			},
			true, ErrGamingGameNotRunning,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := gamingGameURL(tc.state, "poker", tc.installed)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got (%q, %v), want %v", got, err, tc.want)
			}
			if got != "" {
				t.Fatalf("refused but still returned %q", got)
			}
		})
	}
}
