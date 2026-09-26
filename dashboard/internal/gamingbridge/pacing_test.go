// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"dcrpulse/internal/gamingpb"
)

// pacingRig is a server whose money calls always succeed, so the only thing
// that can refuse below is the pacing itself. Built directly rather than
// through the TLS harness: what is under test is the limiter in front of the
// handler, not the transport behind it.
func pacingRig() *Server {
	ok := &gamingpb.Spend{Id: "x", State: "pending"}
	return &Server{
		cfg: Config{
			VerifiedSpend: func(context.Context, string, *gamingpb.RequestSpendRequest) (*gamingpb.Spend, error) {
				return ok, nil
			},
			SpendStatus: func(string, string) (*gamingpb.Spend, error) { return ok, nil },
			SendFrame:   func(context.Context, string, string, string) error { return nil },
		},
		reqLim:    map[string]*rate.Limiter{},
		statusLim: map[string]*rate.Limiter{},
		frameLim:  map[string]*rate.Limiter{},
	}
}

// caller plants a game identity the way the interceptor would.
func caller(game string) context.Context {
	return context.WithValue(context.Background(), callerKey{}, game)
}

// The pacing exists for floods, and only floods may meet it. A refused
// status poll answers Unavailable - never ResourceExhausted, which the
// deployed game renders as "refused by the spending limit" - and a refused
// request answers ResourceExhausted, which is exactly a limit to back off
// from.
//
// Kills: the two refusal codes swapped; the bursts drifting under what an
// honest game does in one breath.
func TestOnlyAFloodMeetsThePacingAndHearsTheRightNo(t *testing.T) {
	s := pacingRig()
	ctx := caller("poker")

	// Eight requests in one breath is the host's own outstanding ceiling;
	// all pass here so the ceiling, not the pacing, is what answers.
	for i := 0; i < 8; i++ {
		if _, err := s.RequestSpend(ctx, &gamingpb.RequestSpendRequest{DepositId: "verified-test-deposit"}); err != nil {
			t.Fatalf("request %d of 8 was paced: %v", i+1, err)
		}
	}
	_, err := s.RequestSpend(ctx, &gamingpb.RequestSpendRequest{DepositId: "verified-test-deposit"})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("the ninth request in one breath came back %v, want ResourceExhausted", err)
	}

	// Twenty status polls back to back cover a reconnecting game asking
	// about everything it remembers; the twenty-first is paced, and told
	// so in the words of a passing condition.
	for i := 0; i < 20; i++ {
		if _, err := s.SpendStatus(ctx, &gamingpb.SpendStatusRequest{}); err != nil {
			t.Fatalf("poll %d of 20 was paced: %v", i+1, err)
		}
	}
	_, err = s.SpendStatus(ctx, &gamingpb.SpendStatusRequest{})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("a paced poll came back %v, want Unavailable", err)
	}
	if status.Code(err) == codes.ResourceExhausted {
		t.Fatal("a paced poll reads as a spending-limit refusal")
	}

	// One game's flood is not another's problem.
	if _, err := s.RequestSpend(caller("chess"), &gamingpb.RequestSpendRequest{DepositId: "verified-test-deposit"}); err != nil {
		t.Fatalf("poker's flood paced chess: %v", err)
	}
}

// Frames are paced only against a flood: a burst far beyond any game's play
// passes, the next is refused before it reaches the host, and another game
// is unaffected.
func TestOnlyAFrameFloodIsPaced(t *testing.T) {
	s := pacingRig()
	sent := 0
	s.cfg.SendFrame = func(context.Context, string, string, string) error { sent++; return nil }
	ctx := caller("poker")
	for i := 0; i < sendFrameBurst; i++ {
		if _, err := s.SendFrame(ctx, &gamingpb.SendFrameRequest{}); err != nil {
			t.Fatalf("frame %d of %d was paced: %v", i+1, sendFrameBurst, err)
		}
	}
	if _, err := s.SendFrame(ctx, &gamingpb.SendFrameRequest{}); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("frame past the burst came back %v, want ResourceExhausted", err)
	}
	if sent != sendFrameBurst {
		t.Fatalf("host saw %d frames, want %d", sent, sendFrameBurst)
	}
	if _, err := s.SendFrame(caller("chess"), &gamingpb.SendFrameRequest{}); err != nil {
		t.Fatalf("poker's flood paced chess: %v", err)
	}
	if sendFrameBurst != 120 || sendFrameEvery != 250*time.Millisecond {
		t.Fatalf("pacing drifted: %d per %v", sendFrameBurst, sendFrameEvery)
	}
}
