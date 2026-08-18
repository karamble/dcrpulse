// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/decred/dcrlnd/lnrpc"
	"github.com/decred/dcrlnd/lnrpc/autopilotrpc"
	"github.com/decred/dcrlnd/lnrpc/invoicesrpc"
	"github.com/decred/dcrlnd/lnrpc/routerrpc"
	"github.com/decred/dcrlnd/lnrpc/verrpc"
	"github.com/decred/dcrlnd/lnrpc/wtclientrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// DcrlndConfig holds the dcrlnd gRPC + macaroon paths.
type DcrlndConfig struct {
	GrpcHost     string
	GrpcPort     string
	TLSCertPath  string
	MacaroonPath string
}

// DcrlndClients is one immutable snapshot of every dcrlnd gRPC stub, built
// from a single connection and published as a whole: a reader sees a complete
// generation or an empty one, never a mix, and never a torn interface value.
type DcrlndClients struct {
	// Lightning is the main dcrlnd service. Reachable only after the LN
	// wallet is unlocked.
	Lightning lnrpc.LightningClient

	// WalletUnlocker is dcrlnd's wallet bootstrap service. The only service
	// reachable while the LN wallet is locked. Used to init the wallet on
	// first run and unlock on subsequent starts.
	WalletUnlocker lnrpc.WalletUnlockerClient

	// Autopilot is dcrlnd's autopilot sub-RPC. Reachable post-unlock.
	Autopilot autopilotrpc.AutopilotClient

	// Versioner returns dcrlnd's clean semver + build metadata. Reachable
	// post-unlock; cheaper than GetInfo and the only path to a clean
	// "0.8.1"-style version string (GetInfo returns "0.8.1-pre+<commit>").
	Versioner verrpc.VersionerClient

	// Router drives Router.SendPaymentV2 for invoice payments. Post-unlock.
	Router routerrpc.RouterClient

	// Invoices drives Invoices.CancelInvoice and HODL invoice flows.
	// Post-unlock.
	Invoices invoicesrpc.InvoicesClient

	// Watchtower is dcrlnd's wtclient sub-RPC for managing watchtower-client
	// registrations. Post-unlock.
	Watchtower wtclientrpc.WatchtowerClientClient
}

var (
	// dcrlndMu guards the published snapshot, the connection and the config.
	// Readers come in through Dcrlnd(); nothing reads the fields directly.
	dcrlndMu      sync.RWMutex
	dcrlndClients DcrlndClients

	// dcrlndConn is the underlying connection, kept for cleanup on reconnect.
	dcrlndConn *grpc.ClientConn

	// dcrlndCfg is the resolved config used for late-binding macaroon reads
	// on every call (the file may not exist until dcrlnd has initialised its
	// wallet).
	dcrlndCfg DcrlndConfig
)

// Dcrlnd returns the current dcrlnd client snapshot. Callers take one
// snapshot per operation and use it throughout, so a reconnect mid-operation
// cannot hand them clients from two different generations.
func Dcrlnd() DcrlndClients {
	dcrlndMu.RLock()
	defer dcrlndMu.RUnlock()
	return dcrlndClients
}

// SwapDcrlndClients publishes a whole client set and returns the previous
// one. Tests install fakes through it; the dialer publishes under the same
// lock internally.
func SwapDcrlndClients(set DcrlndClients) DcrlndClients {
	dcrlndMu.Lock()
	defer dcrlndMu.Unlock()
	prev := dcrlndClients
	dcrlndClients = set
	return prev
}

// InitDcrlndClient dials dcrlnd's gRPC over TLS pinned to dcrlnd's
// self-signed cert. The macaroon is read fresh on every call because
// dcrlnd writes it on first init — at dashboard startup the file
// may not exist yet, but the connection itself can be established.
// Mirrors Decrediton's app/middleware/ln/client.js:22-95.
func InitDcrlndClient(cfg DcrlndConfig) error {
	dcrlndMu.Lock()
	defer dcrlndMu.Unlock()
	return initDcrlndLocked(cfg)
}

// initDcrlndLocked dials and publishes the whole client set in one store.
// Callers hold dcrlndMu.
func initDcrlndLocked(cfg DcrlndConfig) error {
	dcrlndCfg = cfg

	target := fmt.Sprintf("%s:%s", cfg.GrpcHost, cfg.GrpcPort)

	// Try to load dcrlnd's self-signed cert. If it doesn't exist yet
	// (first boot before the wizard has unlocked the wallet) defer
	// the dial until we observe the cert; the dashboard's status
	// endpoint reports the right stage to the UI in the meantime.
	tlsCreds, err := loadDcrlndTLSCreds(cfg.TLSCertPath, cfg.GrpcHost)
	if err != nil {
		rpccLog.Warnf("dcrlnd cert not yet available at %s: %v (will retry on demand)", cfg.TLSCertPath, err)
		return nil
	}

	rpccLog.Infof("Connecting to dcrlnd gRPC at %s with TLS pinning", target)
	conn, err := grpc.Dial(
		target,
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithPerRPCCredentials(macaroonCreds{path: cfg.MacaroonPath}),
	)
	if err != nil {
		return fmt.Errorf("failed to dial dcrlnd: %v", err)
	}

	dcrlndConn = conn
	dcrlndClients = DcrlndClients{
		Lightning:      lnrpc.NewLightningClient(conn),
		WalletUnlocker: lnrpc.NewWalletUnlockerClient(conn),
		Autopilot:      autopilotrpc.NewAutopilotClient(conn),
		Versioner:      verrpc.NewVersionerClient(conn),
		Router:         routerrpc.NewRouterClient(conn),
		Invoices:       invoicesrpc.NewInvoicesClient(conn),
		Watchtower:     wtclientrpc.NewWatchtowerClientClient(conn),
	}

	rpccLog.Info("dcrlnd gRPC clients initialised")
	return nil
}

// ReinitDcrlndClient is called when the dashboard observes the dcrlnd
// cert appearing on disk after a deferred startup (e.g. the wizard
// just completed). The check and the dial share one critical section.
func ReinitDcrlndClient() error {
	dcrlndMu.Lock()
	defer dcrlndMu.Unlock()
	if dcrlndClients.Lightning != nil {
		return nil
	}
	return initDcrlndLocked(dcrlndCfg)
}

// ReconnectDcrlnd repoints the dcrlnd client at a different wallet's node by
// updating the cert/macaroon paths and redialing. Best-effort: the target
// wallet's node may not be up yet (its cert appears once that wallet's
// Lightning is set up and unlocked), in which case the clients stay empty and
// the LN status machine reports the right stage. Readers observe the old set,
// the empty set, or the new set - never a mix.
func ReconnectDcrlnd(tlsCertPath, macaroonPath string) {
	dcrlndMu.Lock()
	defer dcrlndMu.Unlock()
	if dcrlndConn != nil {
		_ = dcrlndConn.Close()
		dcrlndConn = nil
	}
	dcrlndClients = DcrlndClients{}
	dcrlndCfg.TLSCertPath = tlsCertPath
	dcrlndCfg.MacaroonPath = macaroonPath
	_ = initDcrlndLocked(dcrlndCfg)
}

func loadDcrlndTLSCreds(certPath, _ string) (credentials.TransportCredentials, error) {
	pem, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("failed to parse dcrlnd cert at %s", certPath)
	}
	// dcrlnd's self-signed cert ships with SANs that vary by container
	// hostname, so Go's default hostname verification cannot be used. We pin
	// the cert in the pool and authenticate the peer leaf against it via
	// VerifyPeerCertificate (see tlspin.go). InsecureSkipVerify disables only
	// Go's built-in chain+hostname check; the callback still runs and is what
	// authenticates the dial, so the pinned cert remains the trust root.
	return credentials.NewTLS(&tls.Config{
		RootCAs:               pool,
		InsecureSkipVerify:    true,
		VerifyPeerCertificate: pinnedLeafVerifier(pool),
	}), nil
}

// macaroonCreds is a grpc.PerRPCCredentials that reads dcrlnd's admin
// macaroon from disk on every call and attaches it as the
// `macaroon` metadata header in hex (Decrediton's exact format at
// app/middleware/ln/client.js:56-64). Reading on every call (instead
// of caching) means a wizard-driven macaroon rotation is picked up
// without restarting the dashboard.
type macaroonCreds struct {
	path string
}

func (m macaroonCreds) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	raw, err := os.ReadFile(m.path)
	if err != nil {
		// The macaroon file does not exist until after dcrlnd's
		// wallet is initialised. WalletUnlocker calls run BEFORE that
		// point and expect no macaroon — Decrediton passes null for
		// these (LNActions.js:137). Mirror that by returning empty
		// metadata instead of an error; the server's auth interceptor
		// permits unlocker methods to proceed without it and rejects
		// protected methods with a clear gRPC error.
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read dcrlnd macaroon: %w", err)
	}
	return map[string]string{"macaroon": hex.EncodeToString(raw)}, nil
}

func (m macaroonCreds) RequireTransportSecurity() bool { return true }
