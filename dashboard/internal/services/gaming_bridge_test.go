package services

import (
	"testing"

	"dcrpulse/internal/types"
)

const testFrame = `--gaming[v=1,game=poker,gv=1,sid=0123456789abcdef,mid=0123456789abcdef,seq=1/1,exp=1783000000]--eyJhY3Rpb24iOiJmb2xkIn0=`

// The bridge routes on the game key and reads nothing else.
func TestParseGamingFrameReadsTheRoutingKey(t *testing.T) {
	f, ok := parseGamingFrame(testFrame)
	if !ok {
		t.Fatal("a real frame must parse")
	}
	if f.Game != "poker" {
		t.Fatalf("routing key is %q, want poker", f.Game)
	}
	if f.Text != testFrame {
		t.Fatal("the frame must be carried whole, not rewritten")
	}
}

// A game this host has never heard of is still a frame. Recognising it is what
// lets the host drop it as unroutable instead of showing it as chat.
func TestParseGamingFrameAcceptsUnknownGamesAndVersions(t *testing.T) {
	for _, f := range []string{
		`--gaming[v=1,game=chess,gv=1,sid=ab,mid=cd,seq=1/1,exp=0]--QUJD`,
		`--gaming[v=99,game=poker,gv=42,sid=ab,mid=cd,seq=1/1,exp=0]--QUJD`,
	} {
		if _, ok := parseGamingFrame(f); !ok {
			t.Errorf("frame not recognised: %q", f)
		}
	}
}

// Getting this wrong means somebody's message being swallowed by the tunnel.
func TestParseGamingFrameLeavesChatAlone(t *testing.T) {
	for _, text := range []string{
		"",
		"hello",
		"look at " + testFrame,
		testFrame + " what do you think",
		// Frame-shaped, but the payload is prose rather than base64.
		`--gaming[v=1,game=poker,gv=1,sid=ab,mid=cd,seq=1/1,exp=0]--QUJD trailing words`,
		// No routing key: nothing to route it to.
		`--gaming[v=1,gv=1,sid=ab,mid=cd,seq=1/1,exp=0]--QUJD`,
		// Empty payload carries nothing.
		`--gaming[v=1,game=poker]--`,
		// Siblings on the same thread.
		`--mcp[v=1,sid=ab,mid=cd,seq=1/1,exp=0]--QUJD`,
		`--embed[type=image/png,data=AAAA]--`,
	} {
		if _, ok := parseGamingFrame(text); ok {
			t.Errorf("ordinary message taken for a frame: %q", text)
		}
	}
}

// One game must not see another's traffic, and a listener that stalls must not
// stall the stream every table shares.
func TestGamingBusRoutesPerGame(t *testing.T) {
	bus := &GamingBus{subs: make(map[*gamingSubscriber]struct{})}

	poker, cancelPoker := bus.Subscribe("poker", 4)
	defer cancelPoker()
	chess, cancelChess := bus.Subscribe("chess", 4)
	defer cancelChess()

	bus.broadcast(GamingFrameEvent{Game: "poker", GCID: "aa", From: "bb", Frame: testFrame})

	select {
	case ev := <-poker:
		if ev.Frame != testFrame {
			t.Fatal("frame altered in transit")
		}
	default:
		t.Fatal("poker did not receive its own frame")
	}

	select {
	case ev := <-chess:
		t.Fatalf("chess received another game's frame: %+v", ev)
	default:
	}
}

// A game that stops draining loses frames rather than blocking every other
// table on the same stream.
func TestGamingBusDropsRatherThanBlocks(t *testing.T) {
	bus := &GamingBus{subs: make(map[*gamingSubscriber]struct{})}
	_, cancel := bus.Subscribe("poker", 1)
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			bus.broadcast(GamingFrameEvent{Game: "poker", Frame: testFrame})
		}
		close(done)
	}()
	<-done // would deadlock if broadcast waited on a full subscriber
}

// Unsubscribing must not leave a closed channel being written to.
func TestGamingBusUnsubscribeStopsDelivery(t *testing.T) {
	bus := &GamingBus{subs: make(map[*gamingSubscriber]struct{})}
	ch, cancel := bus.Subscribe("poker", 1)
	cancel()

	bus.broadcast(GamingFrameEvent{Game: "poker", Frame: testFrame})
	if _, open := <-ch; open {
		t.Fatal("a cancelled subscription should be closed and empty")
	}
}

// The token is the game's identity. Everything the host enforces hangs off what
// it resolves to, so resolution is the security boundary.
func TestGamingGameForToken(t *testing.T) {
	tokens, err := carryGameTokens(nil, []string{"poker", "chess"})
	if err != nil {
		t.Fatalf("mint tokens: %v", err)
	}
	s := types.GamingSettings{Enabled: true, GameTokens: tokens}

	for game, tok := range tokens {
		got, ok := gamingGameForToken(s, tok)
		if !ok || got != game {
			t.Fatalf("token for %s resolved to (%q, %v)", game, got, ok)
		}
	}
	// One game's token must never resolve to another.
	if got, _ := gamingGameForToken(s, tokens["poker"]); got == "chess" {
		t.Fatal("a token resolved to the wrong game")
	}

	for _, bad := range []string{"", "wrong", tokens["poker"] + "x", tokens["poker"][:len(tokens["poker"])-1]} {
		if _, ok := gamingGameForToken(s, bad); ok {
			t.Errorf("token %q should not resolve", bad)
		}
	}
}

// A disabled section resolves nothing, so the tunnel answers as though it is
// not there rather than admitting it exists and refusing.
func TestDisabledSectionResolvesNoToken(t *testing.T) {
	tokens, err := carryGameTokens(nil, []string{"poker"})
	if err != nil {
		t.Fatalf("mint tokens: %v", err)
	}
	off := types.GamingSettings{Enabled: false, GameTokens: tokens}
	if _, ok := gamingGameForToken(off, tokens["poker"]); ok {
		t.Fatal("a disabled gaming section must resolve no tokens")
	}
}

// Tokens survive unrelated edits and die with the game they identify.
func TestCarryGameTokens(t *testing.T) {
	first, err := carryGameTokens(nil, []string{"poker"})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if first["poker"] == "" {
		t.Fatal("installing a game must mint it a token")
	}

	// An unrelated policy edit must not rotate it, or every save would cut
	// off every running game.
	again, err := carryGameTokens(first, []string{"poker"})
	if err != nil {
		t.Fatalf("carry: %v", err)
	}
	if again["poker"] != first["poker"] {
		t.Fatal("a token must survive an unrelated settings write")
	}

	// Adding a game mints only the new one.
	added, err := carryGameTokens(first, []string{"poker", "chess"})
	if err != nil {
		t.Fatalf("carry: %v", err)
	}
	if added["poker"] != first["poker"] || added["chess"] == "" || added["chess"] == added["poker"] {
		t.Fatalf("adding a game disturbed the others: %+v", added)
	}

	// Uninstalling revokes rather than hides.
	removed, err := carryGameTokens(added, []string{"chess"})
	if err != nil {
		t.Fatalf("carry: %v", err)
	}
	if _, still := removed["poker"]; still {
		t.Fatal("an uninstalled game must lose its token")
	}
}
