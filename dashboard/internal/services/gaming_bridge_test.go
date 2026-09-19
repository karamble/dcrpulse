package services

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dcrpulse/internal/types"
)

func TestBridgeAuthoredEnvelopeMatchesWireV2Golden(t *testing.T) {
	got, err := buildGamingFrame("poker", 5, "0123456789abcdef", []byte(`{"action":"fold"}`), time.Unix(1783000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := `--gaming[v=2,game=poker,gv=5,sid=0123456789abcdef,mid=388d6729aa34aa0da30879de821236cddb5efc55e839bd40f530ad7c3a3e2a23,seq=1/1,exp=1783000000]--eyJhY3Rpb24iOiJmb2xkIn0=`
	if got != want {
		t.Fatalf("bridge v2 envelope = %q, want %q", got, want)
	}
}

const testFrame = `--gaming[v=2,game=poker,gv=1,sid=0123456789abcdef,mid=5736684151c34f0a17823de6822769dfafeb3170477c2079dec9d72e35aa5c5f,seq=1/1,exp=1783000000]--eyJhY3Rpb24iOiJmb2xkIn0=`

func TestDeliverFrameRecognisesGCMessageEnvelope(t *testing.T) {
	// A bus of its own, so the singleton is neither read nor replaced. The
	// routing decision is a method on the bus and needs nothing global.
	bus := &GamingBus{subs: make(map[*gamingSubscriber]struct{})}
	payload, err := json.Marshal(map[string]string{
		"gcid": "aa", "from": "bb", "message": testFrame,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The test game need not be registered to establish the routing decision:
	// an unknown gaming envelope is still protocol traffic and must not appear
	// as chat.
	if !bus.deliverFrame(payload) {
		t.Fatal("valid --gaming envelope in gc-message was not consumed")
	}
	payload, err = json.Marshal(map[string]string{
		"gcid": "aa", "from": "bb", "message": "ordinary chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	if bus.deliverFrame(payload) {
		t.Fatal("ordinary gc-message was consumed by the gaming bridge")
	}
}

// The bridge routes on the game key and reads nothing else.
func TestParseGamingFrameReadsTheRoutingKey(t *testing.T) {
	f, ok := parseGamingFrame(testFrame)
	if !ok {
		t.Fatal("a real frame must parse")
	}
	if f.Game != "poker" {
		t.Fatalf("routing key is %q, want poker", f.Game)
	}
	if f.Version != "2" || f.MID == "" || f.Part != "1/1" {
		t.Fatalf("wire identity was not parsed: %+v", f)
	}
	if f.Text != testFrame {
		t.Fatal("the frame must be carried whole, not rewritten")
	}
}

// A game this host has never heard of is still a frame. Recognising it is what
// lets the host drop it as unroutable instead of showing it as chat.
func TestParseGamingFrameAcceptsUnknownGamesAndRejectsOldVersions(t *testing.T) {
	unknown := strings.Replace(testFrame, "game=poker", "game=chess", 1)
	if _, ok := parseGamingFrame(unknown); !ok {
		t.Errorf("unknown game frame not recognised: %q", unknown)
	}
	old := strings.Replace(testFrame, "v=2", "v=1", 1)
	if _, ok := parseGamingFrame(old); ok {
		t.Errorf("retired wire version was accepted: %q", old)
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

	bus.broadcast(GamingFrameEvent{Seq: 1, Game: "poker", GCID: "aa", From: "bb", Frame: testFrame})

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
			bus.broadcast(GamingFrameEvent{Seq: uint64(i + 1), Game: "poker", Frame: testFrame})
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

	bus.broadcast(GamingFrameEvent{Seq: 1, Game: "poker", Frame: testFrame})
	if _, open := <-ch; open {
		t.Fatal("a cancelled subscription should be closed and empty")
	}
}

// A credential survives an unrelated edit and dies with the game it identifies.
//
// Both halves matter. If saving the section rotated credentials, every routine
// policy edit would cut off every connected game. If unregistering left one
// behind, removing a game would hide it rather than revoke it, and a machine
// still running that game would go on being admitted.
func TestCarryGameCredentials(t *testing.T) {
	first := map[string]types.GameCredential{
		"poker": {Fingerprint: "poker-fp", CertPEM: "poker-cert", IssuedAt: 1},
		"chess": {Fingerprint: "chess-fp", CertPEM: "chess-cert", IssuedAt: 2},
	}

	kept := carryGameCredentials(first, []string{"poker", "chess"})
	if kept["poker"] != first["poker"] || kept["chess"] != first["chess"] {
		t.Fatalf("an unrelated settings write disturbed a credential: %+v", kept)
	}

	// Unregistering revokes rather than hides.
	removed := carryGameCredentials(first, []string{"chess"})
	if _, still := removed["poker"]; still {
		t.Fatal("an unregistered game kept its credential, so removing it only hid it")
	}
	if removed["chess"] != first["chess"] {
		t.Fatal("removing one game disturbed another's credential")
	}
}

// Registering a game does not mint it a credential.
//
// This is the difference from the tokens this replaced. Issuing hands the
// operator a private key that exists nowhere else, so it has to be something
// they asked for and were shown the result of - a credential minted quietly by
// a settings write would be one nobody ever received.
func TestRegisteringMintsNoCredential(t *testing.T) {
	fresh := carryGameCredentials(nil, []string{"poker"})
	if _, minted := fresh["poker"]; minted {
		t.Fatal("registering a game minted a credential nobody was ever shown")
	}
}
