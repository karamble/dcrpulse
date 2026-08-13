// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"dcrpulse/internal/gamingpb"
)

// These are the promises the feature exists to keep, and they are red until the
// bridge keeps them. Every RPC still answers Unimplemented, so each one fails
// here at the first thing it asks for - which is the point: written afterwards,
// a test describes whatever got built.
//
// Each failure names what a person loses if the bridge behaves the way it
// currently does, rather than reporting the code that came back.

// recvWithin takes the next event off a stream, or reports why not.
//
// The deadline is a hang guard and never the assertion: a subscription that
// legitimately has nothing to say yet would otherwise be indistinguishable from
// a broken one, and a test that passed because something arrived quickly would
// fail on a loaded machine for no reason at all.
func recvWithin(t *testing.T, stream grpc.ServerStreamingClient[gamingpb.BridgeEvent]) (*gamingpb.BridgeEvent, error) {
	t.Helper()
	type received struct {
		ev  *gamingpb.BridgeEvent
		err error
	}
	ch := make(chan received, 1)
	go func() {
		ev, err := stream.Recv()
		ch <- received{ev, err}
	}()
	select {
	case r := <-ch:
		return r.ev, r.err
	case <-time.After(dialDeadline):
		t.Fatal("nothing came back from the subscription at all, so this asserts nothing")
		return nil, nil
	}
}

// subscribe opens a game's stream and reports the error rather than ending the
// test, so the caller's own message can say what the silence costs.
func (r *bridgeRig) subscribe(t *testing.T, game string) (grpc.ServerStreamingClient[gamingpb.BridgeEvent], error) {
	t.Helper()
	client := r.dial(t, r.register(t, game))
	ctx, cancel := callCtx(t)
	t.Cleanup(cancel)
	return client.Subscribe(ctx, &gamingpb.SubscribeRequest{})
}

// A game is told what it is; it does not announce it.
//
// The credential decides, and the game learns the answer from Hello. Without
// it, an operator who copied the wrong credential onto a machine gets a game
// quietly acting as something else, spending against caps set for a different
// table - a configuration mistake that should be loud and instead is invisible.
func TestAGameIsToldWhatItIs(t *testing.T) {
	r := newBridgeRig(t)
	c := r.dial(t, r.register(t, "poker"))

	ctx, cancel := callCtx(t)
	defer cancel()
	// A game claiming to be something else on purpose: the reply must correct
	// it rather than take its word.
	reply, err := c.Hello(ctx, &gamingpb.HelloRequest{GameId: "not-poker", ClientVersion: "test"})
	if err != nil {
		t.Fatalf("a game cannot find out what this bridge considers it to be, "+
			"so a misplaced credential stays invisible: %v", err)
	}
	if reply.GetGame() != "poker" {
		t.Fatalf("the bridge agreed the caller was %q; a game that names itself can spend against "+
			"another game's caps", reply.GetGame())
	}
}

// A game learns which chain it is on before it builds anything.
//
// A game built for one network talking to a bridge on another produces scripts
// nobody can ever spend, and pays real money into them. The game is required to
// refuse on a mismatch, which it can only do if the bridge says.
func TestAGameLearnsTheNetworkBeforeItSpends(t *testing.T) {
	r := newBridgeRig(t)
	c := r.dial(t, r.register(t, "poker"))

	ctx, cancel := callCtx(t)
	defer cancel()
	reply, err := c.Hello(ctx, &gamingpb.HelloRequest{GameId: "poker", ClientVersion: "test"})
	if err != nil {
		t.Fatalf("a game cannot learn which chain this bridge is on, so it can only find out "+
			"by paying into a script nobody can spend: %v", err)
	}
	if reply.GetNetwork() == "" {
		t.Fatal("the bridge named no network, so a mainnet game and a testnet bridge look identical " +
			"until the money is gone")
	}
}

// What the operator asked for reaches the game it was addressed to.
//
// This is the entire point of the stream. A subscription that cannot be held is
// a console whose buttons do nothing: an invite the operator accepted never
// gets accepted, and a reclaim never comes home.
func TestWhatIsAddressedToAGameReachesIt(t *testing.T) {
	r := newBridgeRig(t)

	stream, err := r.subscribe(t, "poker")
	if err != nil {
		t.Fatalf("a registered game cannot hold a subscription, so nothing the operator asks for "+
			"ever reaches it: %v", err)
	}

	// The first event is always StreamStart, which is the only place a gap is
	// declared - a game that inferred one from its own reconnect loop would
	// resync every table on every failed dial.
	first, err := recvWithin(t, stream)
	if err != nil {
		t.Fatalf("the subscription produced no opening event: %v", err)
	}
	if first.GetStart() == nil {
		t.Fatalf("the stream opened with %T instead of StreamStart, so the game is never told "+
			"whether it missed anything", first.GetEvent())
	}

	r.Deliver("poker", &gamingpb.BridgeRequest{
		RequestId: "req-1",
		Req:       &gamingpb.BridgeRequest_RefreshState{RefreshState: &gamingpb.RefreshState{}},
	})

	next, err := recvWithin(t, stream)
	if err != nil {
		t.Fatalf("the request the operator made never arrived: %v", err)
	}
	if next.GetRequest().GetRequestId() != "req-1" {
		t.Fatalf("the game was sent %v instead of the request addressed to it", next.GetEvent())
	}
}

// One game's traffic never reaches another.
//
// Games registered on one bridge are separate programs run by separate people,
// and a table's frames name who is at it and what they staked. Delivery that
// leaked across games would hand one operator's table to another's program.
//
// Proved by sequencing rather than by absence: a tracer addressed to the second
// game is sent after the frame addressed to the first, and the second game's
// first event after its opener must be the tracer. Waiting to see whether the
// wrong thing turns up can only ever time out, which proves nothing and flakes.
func TestOneGamesTrafficNeverReachesAnother(t *testing.T) {
	r := newBridgeRig(t)

	poker, err := r.subscribe(t, "poker")
	if err != nil {
		t.Fatalf("a registered game cannot hold a subscription, so isolation between games "+
			"cannot be shown: %v", err)
	}
	dice, err := r.subscribe(t, "dice")
	if err != nil {
		t.Fatalf("a second game cannot hold a subscription, so two games cannot run at once: %v", err)
	}

	for name, s := range map[string]grpc.ServerStreamingClient[gamingpb.BridgeEvent]{"poker": poker, "dice": dice} {
		if _, err := recvWithin(t, s); err != nil {
			t.Fatalf("%s never got its opening event: %v", name, err)
		}
	}

	r.Deliver("poker", &gamingpb.BridgeRequest{
		RequestId: "for-poker",
		Req:       &gamingpb.BridgeRequest_RefreshState{RefreshState: &gamingpb.RefreshState{}},
	})
	r.Deliver("dice", &gamingpb.BridgeRequest{
		RequestId: "tracer",
		Req:       &gamingpb.BridgeRequest_RefreshState{RefreshState: &gamingpb.RefreshState{}},
	})

	ev, err := recvWithin(t, dice)
	if err != nil {
		t.Fatalf("the second game received nothing: %v", err)
	}
	if got := ev.GetRequest().GetRequestId(); got != "tracer" {
		t.Fatalf("the second game's first event was %q, not the tracer: one game is being handed "+
			"another's table traffic", got)
	}
}

// Revoking ends the stream the credential is holding.
//
// A revocation that only stopped the next connection would leave the withdrawn
// machine receiving every frame from every table for as long as it kept the one
// it already had open - which is precisely the situation an operator revokes in.
func TestRevokingEndsTheStreamItIsHolding(t *testing.T) {
	r := newBridgeRig(t)

	stream, err := r.subscribe(t, "poker")
	if err != nil {
		t.Fatalf("a registered game cannot hold a subscription, so there is no live stream for "+
			"revocation to end: %v", err)
	}
	if _, err := recvWithin(t, stream); err != nil {
		t.Fatalf("the subscription never opened: %v", err)
	}

	r.revoke("poker")

	if _, err := recvWithin(t, stream); err == nil {
		t.Fatal("the stream survived the revocation, so a withdrawn credential goes on receiving " +
			"every table's traffic until it chooses to disconnect")
	}
}

// A spend over the cap is refused before anyone is asked.
//
// The cap is the operator's standing decision, and its value is that it holds
// when nobody is looking. A bridge that raised an approval for an over-cap
// request would turn the cap into a prompt - and prompts get accepted.
func TestAnOverCapSpendIsRefusedBeforeAnyoneIsAsked(t *testing.T) {
	r := newBridgeRig(t)
	c := r.dial(t, r.register(t, "poker"))

	ctx, cancel := callCtx(t)
	defer cancel()
	_, err := c.RequestSpend(ctx, &gamingpb.RequestSpendRequest{
		Address:     "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu",
		AmountAtoms: 1 << 40, // far past any cap an operator would set
		Reason:      "a buy-in nobody agreed to",
	})
	if err == nil {
		t.Fatal("an over-cap spend was accepted, so the cap is advice rather than a limit")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.ResourceExhausted {
		t.Fatalf("an over-cap spend was refused as %v rather than as a cap, so an operator reading "+
			"the console cannot tell a limit from a fault: %v", st.Code(), st.Message())
	}
}

// Nothing is paid without a person.
//
// The bridge holds the keys and the game does not, and the whole of what that
// buys is that a spend waits for a human. A RequestSpend that came back already
// paid would mean the game had spent the operator's money by asking.
func TestNothingIsPaidWithoutAPerson(t *testing.T) {
	r := newBridgeRig(t)
	c := r.dial(t, r.register(t, "poker"))

	ctx, cancel := callCtx(t)
	defer cancel()
	spend, err := c.RequestSpend(ctx, &gamingpb.RequestSpendRequest{
		Address:     "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu",
		AmountAtoms: 100000,
		Reason:      "table buy-in",
	})
	if err != nil {
		t.Fatalf("a game cannot ask for a spend at all, so no table can ever be funded: %v", err)
	}
	if spend.GetState() != "pending" {
		t.Fatalf("the spend came back %q rather than pending: a game asking is being treated as "+
			"the operator agreeing", spend.GetState())
	}
	if spend.GetTxid() != "" {
		t.Fatalf("the spend was already broadcast as %s before anybody approved it", spend.GetTxid())
	}
}

// A game is told about its own spends and no others.
//
// Spend ids are the bridge's, not the game's, and a game that could read
// another's would learn what that operator paid, to whom, and for what. Answered
// as not-found rather than as forbidden, because forbidden would confirm the id
// is real.
func TestAGameCannotReadAnotherGamesSpend(t *testing.T) {
	r := newBridgeRig(t)
	poker := r.dial(t, r.register(t, "poker"))
	dice := r.dial(t, r.register(t, "dice"))

	ctx, cancel := callCtx(t)
	defer cancel()
	spend, err := poker.RequestSpend(ctx, &gamingpb.RequestSpendRequest{
		Address:     "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu",
		AmountAtoms: 100000,
		Reason:      "table buy-in",
	})
	if err != nil {
		t.Fatalf("a game cannot ask for a spend, so there is no spend to keep private: %v", err)
	}

	_, err = dice.SpendStatus(ctx, &gamingpb.SpendStatusRequest{Id: spend.GetId()})
	if err == nil {
		t.Fatal("one game read another's spend, so every game on this bridge can see what the " +
			"others paid and to whom")
	}
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Errorf("the refusal was %v rather than not-found, which confirms to the asking game that "+
			"the spend exists", st.Code())
	}
}
