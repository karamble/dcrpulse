// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"context"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/decred/dcrd/certgen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"dcrpulse/internal/gamingpb"
)

// The rig is one whole bridge: the real service, on a real loopback socket,
// behind real mutual TLS, with credentials minted by the same code the
// dashboard's button calls.
//
// None of that is scenery. A harness that faked the transport would assert
// nothing about the transport, and the promises being made here are almost all
// transport promises: that a stranger's certificate is refused, that a revoked
// one stops working, that a game is told what it is rather than asked. An
// in-memory pipe would hand the client a dialer and quietly assume away the
// part under test.
//
// What a game is given is an address and a credential, and the rig hands over
// exactly that and nothing else - which is the whole of the claim that a game
// needs no inbound port and no route back to it.

// dialDeadline bounds anything that could hang. It is never the assertion: a
// test that passes because something took less than a second is a test that
// will fail on a slow machine for no reason.
const dialDeadline = 10 * time.Second

// What the stood-in dependencies answer. Named so an assertion can say which
// value it expected rather than repeating a literal.
const (
	testNetwork     = "mainnet"
	testTipHeight   = int64(900123)
	testTipHash     = "0000000000000000148a2d1f1e0e3a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5"
	testPerTableCap = int64(100_000_000)
	testPerDayCap   = int64(500_000_000)
)

// gameCreds is everything a game is ever handed.
type gameCreds struct {
	Game       string
	CertPEM    []byte
	KeyPEM     []byte
	BridgeCert []byte
}

type bridgeRig struct {
	Addr       string
	BridgeCert []byte

	srv   *Server
	allow *Allowlist

	// appPassword and enabled are the two halves of whether the bridge is
	// answering, flippable mid-test because that is exactly what an operator
	// does.
	appPassword atomic.Bool
	enabled     atomic.Bool

	// missed stands in for the fan-out having dropped frames for a table
	// while nothing was connected.
	missedMu sync.Mutex
	missed   map[string][]string

	spendMu  sync.Mutex
	spendSeq int
	spends   map[string]*gamingpb.Spend
}

// loseFrames records that a game's table lost frames, as the fan-out would.
func (r *bridgeRig) loseFrames(game, gcid string) {
	r.missedMu.Lock()
	defer r.missedMu.Unlock()
	if r.missed == nil {
		r.missed = map[string][]string{}
	}
	r.missed[game] = append(r.missed[game], gcid)
}

func (r *bridgeRig) takeMissed(game string) []string {
	r.missedMu.Lock()
	defer r.missedMu.Unlock()
	out := r.missed[game]
	delete(r.missed, game)
	return out
}

// requestSpend records a request the way the policy would, and refuses the
// ones a cap would refuse.
func (r *bridgeRig) requestSpend(game, address string, atoms int64, reason string) (*gamingpb.Spend, error) {
	if atoms > testPerTableCap {
		return nil, fmt.Errorf("%w: %d atoms is over %q's per-table cap of %d",
			ErrSpendOverCap, atoms, game, testPerTableCap)
	}

	r.spendMu.Lock()
	defer r.spendMu.Unlock()
	r.spendSeq++
	spend := &gamingpb.Spend{
		Id:          fmt.Sprintf("spend-%d", r.spendSeq),
		Game:        game,
		Address:     address,
		AmountAtoms: atoms,
		Reason:      reason,
		// Pending, and nothing else. A person has not seen it yet, and this
		// bridge holds no passphrase with which to pay it if they had.
		State: "pending",
	}
	if r.spends == nil {
		r.spends = map[string]*gamingpb.Spend{}
	}
	r.spends[spend.GetId()] = spend
	return spend, nil
}

// spendStatus answers only about the asking game's own requests.
func (r *bridgeRig) spendStatus(game, id string) (*gamingpb.Spend, error) {
	r.spendMu.Lock()
	defer r.spendMu.Unlock()
	spend, ok := r.spends[id]
	if !ok || spend.GetGame() != game {
		return nil, ErrSpendNotFound
	}
	return spend, nil
}

// newBridgeRig stands the bridge up and tears it down again.
func newBridgeRig(t *testing.T) *bridgeRig {
	t.Helper()

	certPEM, keyPEM, err := certgen.NewTLSCertPair(
		elliptic.P256(), "dcrpulse gaming bridge test", time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatalf("mint the bridge's own certificate: %v", err)
	}

	r := &bridgeRig{BridgeCert: certPEM, allow: NewAllowlist()}
	r.appPassword.Store(true)
	r.enabled.Store(true)

	srv, err := New(Config{
		Addr:              "127.0.0.1:0",
		ServerCert:        certPEM,
		ServerKey:         keyPEM,
		AppPasswordActive: r.appPassword.Load,
		Enabled:           r.enabled.Load,
		Allow:             r.allow,

		// The dependencies the bridge is handed in production, stood in for
		// here. They are supplied, never simplified: each returns what the
		// real one would, so what is asserted below is the bridge's own
		// behaviour rather than a stub's.
		Network: func() (string, bool) { return testNetwork, true },
		ChainTip: func(context.Context) (int64, string, error) {
			return testTipHeight, testTipHash, nil
		},
		Policy: func(string) (int64, int64, bool) {
			return testPerTableCap, testPerDayCap, true
		},
		TakeMissed: func(game string) []string { return r.takeMissed(game) },

		// The money, stood in for. The cap arithmetic itself belongs to the
		// policy and is tested where it lives; what is asserted here is that
		// this bridge attributes a request to the game on the certificate,
		// refuses an over-cap one in the words a cap deserves, and never
		// hands one game another's answer.
		RequestSpend: r.requestSpend,
		SpendStatus:  r.spendStatus,
	})
	if err != nil {
		t.Fatalf("prepare the bridge: %v", err)
	}
	r.srv = srv

	serving := make(chan error, 1)
	go func() { serving <- srv.Serve() }()

	// Serve binds before it blocks, but not before it returns, so wait for
	// the address rather than guessing at a delay.
	deadline := time.Now().Add(dialDeadline)
	for r.srv.Addr() == "" {
		select {
		case err := <-serving:
			t.Fatalf("the bridge stopped before it listened: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the bridge never started listening")
		}
		time.Sleep(time.Millisecond)
	}
	r.Addr = r.srv.Addr()

	t.Cleanup(func() { srv.Stop() })
	return r
}

// register issues a game its credential the way the dashboard does.
//
// Through the production path on purpose: a credential minted specially for a
// test would let cert-as-identity pass against a certificate shape no operator
// will ever hold.
func (r *bridgeRig) register(t *testing.T, game string) gameCreds {
	t.Helper()
	c, err := GenerateGameCredential(game)
	if err != nil {
		t.Fatalf("issue a credential for %q: %v", game, err)
	}
	if err := r.allow.Add(c); err != nil {
		t.Fatalf("admit %q: %v", game, err)
	}
	return gameCreds{Game: game, CertPEM: c.CertPEM, KeyPEM: c.KeyPEM, BridgeCert: r.BridgeCert}
}

func (r *bridgeRig) revoke(game string) { r.allow.Revoke(game) }

// deliver stands in for Bison Relay: it is the bridge learning something and
// handing it to the game it is addressed to.
func (r *bridgeRig) Deliver(game string, req *gamingpb.BridgeRequest) {
	r.srv.Deliver(game, req)
}

// clientTLS is what a game configures itself with: its own credential, and the
// bridge's certificate to pin.
func clientTLS(t *testing.T, c gameCreds) *tls.Config {
	t.Helper()
	pair, err := tls.X509KeyPair(c.CertPEM, c.KeyPEM)
	if err != nil {
		t.Fatalf("load the credential: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(c.BridgeCert) {
		t.Fatal("the bridge's certificate did not parse")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{pair},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS12,
		// The bridge is pinned by its certificate, and an operator reaches
		// it by address rather than by a name anything issued.
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			leaf, err := x509.ParseCertificate(raw[0])
			if err != nil {
				return err
			}
			_, err = leaf.Verify(x509.VerifyOptions{Roots: pool})
			return err
		},
	}
}

// dial connects as a game would.
func (r *bridgeRig) dial(t *testing.T, c gameCreds) gamingpb.BridgeServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(r.Addr,
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS(t, c))))
	if err != nil {
		t.Fatalf("dial the bridge: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return gamingpb.NewBridgeServiceClient(conn)
}

// callCtx bounds one call. The deadline is a hang guard; what is asserted is
// always what came back.
func callCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), dialDeadline)
}
