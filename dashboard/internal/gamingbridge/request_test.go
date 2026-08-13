// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"context"
	"errors"
	"testing"
	"time"

	"dcrpulse/internal/gamingpb"
)

// acceptInviteReq is the console asking a game to take a seat.
func acceptInviteReq() *gamingpb.BridgeRequest {
	return &gamingpb.BridgeRequest{
		Req: &gamingpb.BridgeRequest_AcceptInvite{
			AcceptInvite: &gamingpb.AcceptInvite{Invite: "gaming://poker/table?sid=abc", Gcid: "gc"},
		},
	}
}

// A request reaches the game and its answer reaches the caller.
//
// Without the round trip the console can push but never learn what happened, so
// a person clicking Accept is told nothing about whether they have a seat.
func TestARequestCarriesTheGamesAnswerBack(t *testing.T) {
	r := newBridgeRig(t)
	client := r.dial(t, r.register(t, "poker"))
	stream, err := client.Subscribe(t.Context(), &gamingpb.SubscribeRequest{})
	if err != nil {
		t.Fatalf("open a subscription: %v", err)
	}
	if ev, err := recvWithin(t, stream); err != nil || ev.GetStart() == nil {
		t.Fatalf("the stream did not open: %v", err)
	}

	type answer struct {
		reply *gamingpb.RespondRequest
		err   error
	}
	done := make(chan answer, 1)
	go func() {
		ctx, cancel := callCtx(t)
		defer cancel()
		reply, err := r.srv.Request(ctx, "poker", acceptInviteReq())
		done <- answer{reply, err}
	}()

	ev, err := recvWithin(t, stream)
	if err != nil {
		t.Fatalf("the game never received the request: %v", err)
	}
	got := ev.GetRequest()
	if got.GetAcceptInvite() == nil {
		t.Fatalf("the game received %T instead of an invitation to accept", ev.GetEvent())
	}
	if got.GetRequestId() == "" {
		t.Fatal("the request carried no id, so no answer to it could ever be matched to it")
	}

	ctx, cancel := callCtx(t)
	defer cancel()
	if _, err := client.Respond(ctx, &gamingpb.RespondRequest{
		RequestId: got.GetRequestId(),
		Ok:        true,
		Result:    &gamingpb.RespondRequest_AcceptInvite{AcceptInvite: &gamingpb.AcceptInviteResult{Sid: "seat-1"}},
	}); err != nil {
		t.Fatalf("the game could not answer: %v", err)
	}

	select {
	case a := <-done:
		if a.err != nil {
			t.Fatalf("the caller got an error rather than the answer the game gave: %v", a.err)
		}
		if sid := a.reply.GetAcceptInvite().GetSid(); sid != "seat-1" {
			t.Fatalf("the caller was handed sid %q, not the one the game reported", sid)
		}
	case <-time.After(dialDeadline):
		t.Fatal("the game answered but the caller was never woken, so the console would wait forever")
	}
}

// A request to a game that is not connected fails at once.
//
// Waiting out the deadline instead would leave a person watching a spinner for
// an answer that was never going to come.
func TestARequestToADisconnectedGameFailsAtOnce(t *testing.T) {
	r := newBridgeRig(t)
	r.register(t, "poker")

	ctx, cancel := callCtx(t)
	defer cancel()
	if _, err := r.srv.Request(ctx, "poker", acceptInviteReq()); !errors.Is(err, ErrGameNotConnected) {
		t.Fatalf("a request to a game holding no stream returned %v, not ErrGameNotConnected", err)
	}
}

// One game's answer must not satisfy another game's request.
//
// A request id is the only thing naming the waiter, so without the check a
// connected game could answer for a table it was never at.
func TestAGameCannotAnswerAnothersRequest(t *testing.T) {
	r := newBridgeRig(t)
	poker := r.dial(t, r.register(t, "poker"))
	chess := r.dial(t, r.register(t, "chess"))

	stream, err := poker.Subscribe(t.Context(), &gamingpb.SubscribeRequest{})
	if err != nil {
		t.Fatalf("open a subscription: %v", err)
	}
	if ev, err := recvWithin(t, stream); err != nil || ev.GetStart() == nil {
		t.Fatalf("the stream did not open: %v", err)
	}

	done := make(chan error, 1)
	reqCtx, cancelReq := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancelReq()
	go func() {
		_, err := r.srv.Request(reqCtx, "poker", acceptInviteReq())
		done <- err
	}()

	ev, err := recvWithin(t, stream)
	if err != nil {
		t.Fatalf("the game never received the request: %v", err)
	}

	ctx, cancel := callCtx(t)
	defer cancel()
	if _, err := chess.Respond(ctx, &gamingpb.RespondRequest{
		RequestId: ev.GetRequest().GetRequestId(),
		Ok:        true,
		Result:    &gamingpb.RespondRequest_AcceptInvite{AcceptInvite: &gamingpb.AcceptInviteResult{Sid: "stolen"}},
	}); err != nil {
		t.Fatalf("the other game's call failed for an unrelated reason: %v", err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("another game's answer satisfied this request, so a game can report a seat at a table it was never asked about")
		}
	case <-time.After(dialDeadline):
		t.Fatal("the request neither completed nor timed out")
	}
}
