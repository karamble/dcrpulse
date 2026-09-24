// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package gamingbridge

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"dcrpulse/internal/gamingpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type rotationStream struct {
	grpc.ServerStream
	ctx  context.Context
	send func(*gamingpb.BridgeEvent) error
}

func (s rotationStream) Context() context.Context            { return s.ctx }
func (s rotationStream) Send(ev *gamingpb.BridgeEvent) error { return s.send(ev) }

func rotationServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(Config{Allow: NewAllowlist(), Enabled: func() bool { return true }, AppPasswordActive: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func rotationCredential(t *testing.T, a *Allowlist, game string) Credential {
	t.Helper()
	c, err := GenerateGameCredential(game)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Add(c); err != nil {
		t.Fatal(err)
	}
	return c
}

// Use the production interceptor to capture exactly the identity a TLS peer gets.
func rotationContext(t *testing.T, s *Server, c Credential) context.Context {
	t.Helper()
	var ctx context.Context
	err := s.streamIdentity(nil, rotationStream{ctx: peerCtx(parseCert(t, c.CertPEM))}, nil, func(_ any, ss grpc.ServerStream) error { ctx = ss.Context(); return nil })
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}
func waitRotation(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("subscription failed to exit")
		return nil
	}
}
func requireRetired(t *testing.T, l *credentialAdmission) {
	t.Helper()
	if l.valid() {
		t.Fatal("retired lifetime is still valid")
	}
}

func TestCredentialLifetimeReplacementRules(t *testing.T) {
	a := NewAllowlist()
	c := rotationCredential(t, a, "poker")
	entry, _ := a.resolve(c.Fingerprint)
	other := rotationCredential(t, a, "chess")
	otherEntry, _ := a.resolve(other.Fingerprint)
	if err := a.Add(c); err != nil {
		t.Fatal(err)
	}
	same, _ := a.resolve(c.Fingerprint)
	if same.lifetime != entry.lifetime || !same.lifetime.valid() {
		t.Fatal("identical add invalidated its lifetime")
	}
	if err := a.Add(Credential{Game: "poker", CertPEM: []byte("invalid")}); err == nil {
		t.Fatal("invalid certificate accepted")
	}
	if !entry.lifetime.valid() {
		t.Fatal("failed replacement invalidated old credential")
	}
	replacement := rotationCredential(t, a, "poker")
	fresh, _ := a.resolve(replacement.Fingerprint)
	requireRetired(t, entry.lifetime)
	if !fresh.lifetime.valid() || !otherEntry.lifetime.valid() {
		t.Fatal("replacement disturbed a current credential")
	}
	a.Revoke("poker")
	a.Revoke("poker")
	requireRetired(t, fresh.lifetime)
	if err := a.Add(replacement); err != nil {
		t.Fatal(err)
	}
	readded, _ := a.resolve(replacement.Fingerprint)
	if readded.lifetime == fresh.lifetime || !readded.lifetime.valid() {
		t.Fatal("re-add revived revoked admission")
	}
	replacement.Game = "chess"
	if err := a.Add(replacement); err != nil {
		t.Fatal(err)
	}
	requireRetired(t, readded.lifetime)
	requireRetired(t, otherEntry.lifetime)
	mapped, _ := a.resolve(replacement.Fingerprint)
	if mapped.game != "chess" || !mapped.lifetime.valid() {
		t.Fatal("fingerprint reassignment failed")
	}
}

func TestRotationBetweenAuthenticationAndRegistration(t *testing.T) {
	s := rotationServer(t)
	old := rotationCredential(t, s.allow, "poker")
	ctx := rotationContext(t, s, old)
	rotationCredential(t, s.allow, "poker")
	s.cfg.OnPresence = func(string) { t.Error("stale registration changed presence") }
	s.cfg.OnConnect = func(string) { t.Error("stale registration invoked connect") }
	s.cfg.Frames = func(string, uint64, int) (<-chan Frame, func()) {
		t.Error("stale registration subscribed to frames")
		return nil, func() {}
	}
	err := s.Subscribe(&gamingpb.SubscribeRequest{}, rotationStream{ctx: ctx, send: func(*gamingpb.BridgeEvent) error { t.Error("stale registration sent event"); return nil }})
	if !errors.Is(err, errNotHere) || s.reg.count("poker") != 0 {
		t.Fatalf("stale registration: %v", err)
	}
}

func TestRotationRejectsQueuedFramesAndEvents(t *testing.T) {
	for _, mode := range []string{"frames", "events", "both"} {
		t.Run(mode, func(t *testing.T) {
			// Exercise select with both invalidation and queued data ready. The send
			// gate must reject data even when select chooses it instead of invalidation.
			for n := 0; n < 32; n++ {
				s := rotationServer(t)
				c := rotationCredential(t, s.allow, "poker")
				ctx := rotationContext(t, s, c)
				frames := make(chan Frame, 1)
				ready := make(chan struct{})
				release := make(chan struct{})
				var sends, stops atomic.Int32
				s.cfg.Frames = func(string, uint64, int) (<-chan Frame, func()) {
					close(ready)
					<-release
					return frames, func() { stops.Add(1) }
				}
				done := make(chan error, 1)
				go func() {
					done <- s.Subscribe(&gamingpb.SubscribeRequest{}, rotationStream{ctx: ctx, send: func(*gamingpb.BridgeEvent) error { sends.Add(1); return nil }})
				}()
				<-ready
				if mode != "events" {
					frames <- Frame{Seq: 1, Frame: "queued"}
				}
				if mode != "frames" {
					s.Deliver("poker", &gamingpb.BridgeRequest{RequestId: "queued"})
				}
				rotationCredential(t, s.allow, "poker")
				close(release)
				if err := waitRotation(t, done); !errors.Is(err, errNotHere) {
					t.Fatal(err)
				}
				if sends.Load() != 1 || stops.Load() != 1 || s.reg.count("poker") != 0 {
					t.Fatalf("sends=%d stops=%d active=%d", sends.Load(), stops.Load(), s.reg.count("poker"))
				}
			}
		})
	}
}

func TestRotationDuringAdmittedSendDoesNotBlockReplacement(t *testing.T) {
	s := rotationServer(t)
	c := rotationCredential(t, s.allow, "poker")
	ctx := rotationContext(t, s, c)
	entered := make(chan struct{})
	release := make(chan struct{})
	var sends, connected atomic.Int32
	s.cfg.OnConnect = func(string) { connected.Add(1) }
	done := make(chan error, 1)
	go func() {
		done <- s.Subscribe(&gamingpb.SubscribeRequest{}, rotationStream{ctx: ctx, send: func(*gamingpb.BridgeEvent) error { sends.Add(1); close(entered); <-release; return nil }})
	}()
	<-entered
	replacement, err := GenerateGameCredential("poker")
	if err != nil {
		t.Fatal(err)
	}
	rotated := make(chan error, 1)
	go func() { rotated <- s.allow.Add(replacement) }()
	// Rotation must return even though the admitted network send has not.
	err = waitRotation(t, rotated)
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	if err := waitRotation(t, done); !errors.Is(err, errNotHere) {
		t.Fatal(err)
	}
	if sends.Load() != 1 || connected.Load() != 0 {
		t.Fatal("retired stream did more work after its admitted send")
	}
	fresh, _ := s.allow.resolve(replacement.Fingerprint)
	if !fresh.lifetime.valid() {
		t.Fatal("retired stream closed replacement")
	}
}

func TestRotationEndsTLSStreamsAndPreservesReplacement(t *testing.T) {
	r := newBridgeRig(t)
	oldCred := r.register(t, "poker")
	oldClient := r.dial(t, oldCred)
	ctx, cancel := callCtx(t)
	defer cancel()
	streams := make([]grpc.ServerStreamingClient[gamingpb.BridgeEvent], 0, 2)
	for i := 0; i < 2; i++ {
		stream, err := oldClient.Subscribe(ctx, &gamingpb.SubscribeRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = recvWithin(t, stream); err != nil {
			t.Fatal(err)
		}
		streams = append(streams, stream)
	}
	if err := r.allow.Add(Credential{Game: "poker", CertPEM: oldCred.CertPEM}); err != nil {
		t.Fatal(err)
	}
	r.Deliver("poker", &gamingpb.BridgeRequest{RequestId: "before"})
	for _, stream := range streams {
		ev, err := recvWithin(t, stream)
		if err != nil || ev.GetRequest().GetRequestId() != "before" {
			t.Fatal("legitimate control failed", err)
		}
	}
	other, err := r.subscribe(t, "chess")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = recvWithin(t, other); err != nil {
		t.Fatal(err)
	}
	newCred := r.register(t, "poker")
	newClient := r.dial(t, newCred)
	fresh, err := newClient.Subscribe(ctx, &gamingpb.SubscribeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = recvWithin(t, fresh); err != nil {
		t.Fatal(err)
	}
	for _, stream := range streams {
		if _, err := recvWithin(t, stream); status.Code(err) != codes.Unimplemented {
			t.Fatalf("old stream: %v", err)
		}
	}
	if _, err = oldClient.ChainTip(ctx, &gamingpb.ChainTipRequest{}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("old connection unary: %v", err)
	}
	stale, err := oldClient.Subscribe(ctx, &gamingpb.SubscribeRequest{})
	if err == nil {
		_, err = recvWithin(t, stale)
	}
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("old connection stream: %v", err)
	}
	r.Deliver("poker", &gamingpb.BridgeRequest{RequestId: "fresh"})
	if ev, err := recvWithin(t, fresh); err != nil || ev.GetRequest().GetRequestId() != "fresh" {
		t.Fatal("replacement failed", err)
	}
	r.Deliver("chess", &gamingpb.BridgeRequest{RequestId: "unaffected"})
	if ev, err := recvWithin(t, other); err != nil || ev.GetRequest().GetRequestId() != "unaffected" {
		t.Fatal("other game affected", err)
	}
	r.revoke("poker")
	if _, err := recvWithin(t, fresh); status.Code(err) != codes.Unimplemented {
		t.Fatalf("explicit revoke: %v", err)
	}
}
