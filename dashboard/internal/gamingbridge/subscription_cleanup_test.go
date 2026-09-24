// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package gamingbridge

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"dcrpulse/internal/gamingpb"
)

func TestSubscriptionStopsFrameFeed(t *testing.T) {
	for _, mode := range []string{"context", "rotation", "shutdown", "send-error", "feed-closed"} {
		t.Run(mode, func(t *testing.T) {
			s := rotationServer(t)
			cred := rotationCredential(t, s.allow, "poker")
			ctx, cancel := context.WithCancel(rotationContext(t, s, cred))
			defer cancel()
			ready := make(chan struct{})
			release := make(chan struct{})
			frames := make(chan Frame, 1)
			var stops atomic.Int32
			s.cfg.Frames = func(string, uint64, int) (<-chan Frame, func()) {
				close(ready)
				<-release
				return frames, func() { stops.Add(1) }
			}
			done := make(chan error, 1)
			go func() {
				done <- s.Subscribe(&gamingpb.SubscribeRequest{}, rotationStream{ctx: ctx, send: func(ev *gamingpb.BridgeEvent) error {
					if ev.GetFrame() != nil {
						return errors.New("test send failure")
					}
					return nil
				}})
			}()
			<-ready
			switch mode {
			case "context":
				cancel()
			case "rotation":
				rotationCredential(t, s.allow, "poker")
			case "shutdown":
				s.reg.closeGame("poker")
			case "send-error":
				frames <- Frame{Seq: 1}
			case "feed-closed":
				close(frames)
			}
			close(release)
			if err := waitRotation(t, done); err == nil {
				t.Fatal("subscription unexpectedly succeeded")
			}
			if stops.Load() != 1 || s.reg.count("poker") != 0 {
				t.Fatalf("stops=%d active=%d", stops.Load(), s.reg.count("poker"))
			}
		})
	}
}
