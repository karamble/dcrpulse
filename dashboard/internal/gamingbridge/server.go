// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"dcrpulse/internal/gamingpb"
)

// Config is everything the bridge needs to answer a port.
//
// Handed in rather than read from package state: this listener is assembled in
// one place at startup and in another by its own tests, and a dependency that
// arrives through the constructor can be different in each without either
// having to put anything back.
type Config struct {
	// Addr is where to listen. ":0" takes any free port, which is what the
	// tests want and production never asks for.
	Addr string

	// ServerCert and ServerKey are this bridge's own identity, the pair a
	// game pins.
	ServerCert, ServerKey []byte

	// AppPasswordActive reports whether the dashboard is behind its App
	// Password right now.
	//
	// A function rather than a value, because it is asked on every call. The
	// caps and the approval a spend waits on are worth exactly as much as
	// the certainty that the person answering is the operator, so the bridge
	// stops answering the moment that stops being true - not at the next
	// restart.
	AppPasswordActive func() bool

	// Enabled reports the operator's stored intent to run the bridge.
	Enabled func() bool

	// Allow is which credentials may connect and what each one is.
	Allow *Allowlist

	// Frames subscribes to the Bison Relay frames addressed to a game, and
	// returns a function that unsubscribes.
	//
	// Injected, like everything else here, so this package goes on depending
	// on nothing: the bridge is handed a game's traffic rather than reaching
	// for it.
	Frames func(game string, buf int) (<-chan Frame, func())

	// TakeMissed reports the tables whose frames could not be delivered since
	// it was last asked, and forgets them.
	//
	// A game learns what it missed only at the start of a stream, so this is
	// what turns a silent loss into one table's resync instead of every
	// table's.
	TakeMissed func(game string) []string

	// Network is the chain this bridge is on, which a game must be told
	// before it builds anything: a game on the wrong network makes scripts
	// nobody can spend and pays real money into them.
	Network func() (string, bool)

	// Policy is a game's caps, so it can say "that buy-in will be refused"
	// before asking a person to try. A courtesy, never the enforcement.
	Policy func(game string) (perTable, perDay int64, accountBound bool)

	// SendFrame carries a frame the game built out to a table.
	SendFrame func(ctx context.Context, game, gcid, frame string) error

	// RequestSpend, SpendStatus and Broadcast are the money. They are named
	// here and implemented elsewhere for the same reason as everything else
	// on this struct: what a spend is allowed to be is the operator's
	// policy, and this package is not where it lives.
	RequestSpend func(game, address string, amountAtoms int64, reason string) (*gamingpb.Spend, error)
	SpendStatus  func(game, id string) (*gamingpb.Spend, error)
	Broadcast    func(ctx context.Context, game, rawTxHex string) (string, error)

	// ChainTip, BlockHash and Outpoint are the chain reads a game needs to
	// agree deadlines and find its own money.
	ChainTip  func(ctx context.Context) (height int64, hash string, err error)
	BlockHash func(ctx context.Context, height int64) (string, error)
	Outpoint  func(ctx context.Context, txid string, vout uint32, includeMempool bool) (Outpoint, error)
}

// ErrSpendOverCap and ErrSpendNotFound are what the injected money functions
// report so this package can answer with the right code without importing the
// package that owns the policy.
var (
	ErrSpendOverCap  = errors.New("over a cap")
	ErrSpendNotFound = errors.New("no such spend request")
)

// Outpoint is what a game is told about one of its outputs.
type Outpoint struct {
	Found         bool
	ValueAtoms    int64
	PkScriptHex   string
	Confirmations int64
	Coinbase      bool
}

// Server is the bridge's listener.
//
// It answers the contract in internal/gamingpb. Every method it does not
// implement answers Unimplemented, which is what the embedded generated type is
// for - and while the transport is being built that is most of them.
type Server struct {
	gamingpb.UnimplementedBridgeServiceServer

	cfg   Config
	allow *Allowlist
	reg   *registry

	mu   sync.Mutex
	grpc *grpc.Server
	lis  net.Listener
}

// New prepares a bridge. It does not listen; Serve does.
func New(cfg Config) (*Server, error) {
	if cfg.Allow == nil {
		return nil, fmt.Errorf("a bridge with no allowlist would answer nobody")
	}
	if cfg.AppPasswordActive == nil || cfg.Enabled == nil {
		return nil, fmt.Errorf("a bridge has to be able to ask whether it should be running")
	}
	s := &Server{cfg: cfg, allow: cfg.Allow, reg: newRegistry()}

	// Withdrawing a credential has to end the stream it is holding, and the
	// allowlist is where withdrawing happens - including when the operator
	// does it through the console rather than through this server.
	cfg.Allow.OnRevoke(s.reg.closeGame)
	return s, nil
}

// live reports whether the bridge should be answering at all.
//
// Both halves, every time. The stored switch is the operator's intent and the
// App Password is what makes an approval mean anything; a stored intent that
// outlived the gate would leave the money routes reachable by whoever got to
// the port first.
func (s *Server) live() bool {
	return s.cfg.Enabled() && s.cfg.AppPasswordActive()
}

// Serve starts listening and blocks until the bridge is stopped.
func (s *Server) Serve() error {
	cert, err := tls.X509KeyPair(s.cfg.ServerCert, s.cfg.ServerKey)
	if err != nil {
		return fmt.Errorf("load the bridge's own certificate: %w", err)
	}

	lis, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.cfg.Addr, err)
	}

	srv := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(serverTLSConfig(cert, s.allow))),
		grpc.ChainUnaryInterceptor(s.unaryIdentity),
		grpc.ChainStreamInterceptor(s.streamIdentity),
		// The same ceiling the browser API puts on a request body. A game
		// is no more trusted than a browser is.
		grpc.MaxRecvMsgSize(1<<20),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 20 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			// The default here is five minutes, which would hang up on a
			// game that pings at any sensible interval and look like a
			// mysterious disconnect loop.
			MinTime: 10 * time.Second,
			// A game between subscriptions is still a client.
			PermitWithoutStream: true,
		}),
	)
	gamingpb.RegisterBridgeServiceServer(srv, s)

	s.mu.Lock()
	s.grpc, s.lis = srv, lis
	s.mu.Unlock()

	return srv.Serve(lis)
}

// Addr is where the bridge actually ended up listening, which is the only way
// to learn it when the port was left to the operating system to choose.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lis == nil {
		return ""
	}
	return s.lis.Addr().String()
}

// Stop ends the bridge.
//
// Graceful first, then not. A graceful stop waits for calls in flight, and the
// subscription stream never ends on its own - so waiting for it without a
// deadline is waiting forever.
func (s *Server) Stop() {
	s.mu.Lock()
	srv := s.grpc
	s.grpc, s.lis = nil, nil
	s.mu.Unlock()
	if srv == nil {
		return
	}

	done := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		srv.Stop()
	}
}

// SubscriberCount is how many streams a game is holding open.
//
// It is what the console reports as connected, and it is also the only honest
// way to know a subscription has been established rather than merely asked for.
func (s *Server) SubscriberCount(game string) int { return s.reg.count(game) }

// Deliver hands a game something that arrived for it.
//
// This is the only way into a game's stream, and it takes the game as an
// argument rather than a stream, so the routing decision is made here from the
// address on the request - a caller that could pick the stream itself could
// deliver one game's traffic to another by accident.
func (s *Server) Deliver(game string, req *gamingpb.BridgeRequest) {
	ev := &gamingpb.BridgeEvent{Event: &gamingpb.BridgeEvent_Request{Request: req}}
	if !s.reg.push(game, ev) {
		gameLog.Warnf("nothing is connected as %q, so request %s reached nobody", game, req.GetRequestId())
	}
}
