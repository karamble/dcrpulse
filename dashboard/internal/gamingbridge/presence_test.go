package gamingbridge

import (
	"context"
	"dcrpulse/internal/gamingpb"
	"google.golang.org/grpc"
	"testing"
	"time"
)

type presenceStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s presenceStream) Context() context.Context         { return s.ctx }
func (s presenceStream) Send(*gamingpb.BridgeEvent) error { return nil }

func TestPresenceNotifiesAfterRegistryChange(t *testing.T) {
	server := &Server{reg: newRegistry()}
	counts := make(chan int, 8)
	server.cfg.OnPresence = func(game string) { counts <- server.reg.count(game) }
	expect := func(want int) {
		t.Helper()
		select {
		case got := <-counts:
			if got != want {
				t.Fatalf("connected streams=%d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("presence notification missing")
		}
	}
	start := func() (context.CancelFunc, <-chan error) {
		ctx, cancel := context.WithCancel(caller("stakewars"))
		done := make(chan error, 1)
		go func() { done <- server.Subscribe(&gamingpb.SubscribeRequest{}, presenceStream{ctx: ctx}) }()
		t.Cleanup(cancel)
		return cancel, done
	}
	cancelA, doneA := start()
	expect(1)
	cancelB, doneB := start()
	expect(2)
	cancelA()
	expect(1)
	<-doneA
	cancelB()
	expect(0)
	<-doneB
}
