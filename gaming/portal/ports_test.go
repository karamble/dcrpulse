package main

import "testing"

// A game keeps the port it was given. The dashboard reads the port out of this
// portal's own report, so a game that moved between polls would be one a user
// could not accept an invitation into.
func TestAGameKeepsThePortItWasGiven(t *testing.T) {
	p := &portal{ports: make(map[string]int)}

	first := p.portFor("poker")
	if first < basePort || first >= basePort+maxGames {
		t.Fatalf("port %d is outside the range", first)
	}
	if again := p.portFor("poker"); again != first {
		t.Fatalf("the same game was given %d and then %d", first, again)
	}
}

func TestTwoGamesNeverShareAPort(t *testing.T) {
	p := &portal{ports: make(map[string]int)}

	seen := make(map[int]string)
	for _, id := range []string{"poker", "chess", "go", "backgammon"} {
		port := p.portFor(id)
		if other, taken := seen[port]; taken {
			t.Fatalf("%s and %s were both given port %d", other, id, port)
		}
		seen[port] = id
	}
}

// Running out is reported as zero rather than by handing out a port already in
// use, which would make two games answer for each other.
func TestRunningOutOfPortsIsRefusedRatherThanShared(t *testing.T) {
	p := &portal{ports: make(map[string]int)}
	for i := range maxGames {
		if got := p.portFor(string(rune('a' + i))); got == 0 {
			t.Fatalf("ran out after %d games, want %d", i, maxGames)
		}
	}
	if got := p.portFor("one-too-many"); got != 0 {
		t.Fatalf("handed out port %d past the end of the range", got)
	}
}
