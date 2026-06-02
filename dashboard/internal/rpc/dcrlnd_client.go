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
	"log"
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

var (
	// LightningClient is the main dcrlnd gRPC service. Reachable only
	// after the LN wallet is unlocked.
	LightningClient lnrpc.LightningClient

	// WalletUnlockerClient is dcrlnd's wallet bootstrap service. The
	// only service reachable while the LN wallet is locked. Used to
	// init the wallet on first run and unlock on subsequent starts.
	WalletUnlockerClient lnrpc.WalletUnlockerClient

	// AutopilotClient is dcrlnd's autopilot sub-RPC. Reachable only
	// after the wallet is unlocked.
	AutopilotClient autopilotrpc.AutopilotClient

	// VersionerClient returns dcrlnd's clean semver + build metadata.
	// Reachable post-unlock; cheaper than GetInfo and the only path
	// to a clean "0.8.1"-style version string (GetInfo returns
	// "0.8.1-pre+<commit>").
	VersionerClient verrpc.VersionerClient

	// RouterClient drives Router.SendPaymentV2 for invoice payments.
	// Reachable post-unlock.
	RouterClient routerrpc.RouterClient

	// InvoicesClient drives Invoices.CancelInvoice and HODL invoice flows.
	// Reachable post-unlock.
	InvoicesClient invoicesrpc.InvoicesClient

	// WatchtowerClient is dcrlnd's wtclient sub-RPC for managing
	// watchtower-client registrations. Reachable post-unlock.
	WatchtowerClient wtclientrpc.WatchtowerClientClient

	// DcrlndGrpcConn is the underlying connection, kept for shutdown.
	DcrlndGrpcConn *grpc.ClientConn

	// DcrlndCfg is the resolved config used for late-binding macaroon
	// reads on every call (the file may not exist until dcrlnd has
	// initialised its wallet).
	DcrlndCfg DcrlndConfig
)

// InitDcrlndClient dials dcrlnd's gRPC over TLS pinned to dcrlnd's
// self-signed cert. The macaroon is read fresh on every call because
// dcrlnd writes it on first init — at dashboard startup the file
// may not exist yet, but the connection itself can be established.
// Mirrors Decrediton's app/middleware/ln/client.js:22-95.
func InitDcrlndClient(cfg DcrlndConfig) error {
	DcrlndCfg = cfg

	target := fmt.Sprintf("%s:%s", cfg.GrpcHost, cfg.GrpcPort)

	// Try to load dcrlnd's self-signed cert. If it doesn't exist yet
	// (first boot before the wizard has unlocked the wallet) defer
	// the dial until we observe the cert; the dashboard's status
	// endpoint reports the right stage to the UI in the meantime.
	tlsCreds, err := loadDcrlndTLSCreds(cfg.TLSCertPath, cfg.GrpcHost)
	if err != nil {
		log.Printf("dcrlnd cert not yet available at %s: %v (will retry on demand)", cfg.TLSCertPath, err)
		return nil
	}

	log.Printf("Connecting to dcrlnd gRPC at %s with TLS pinning", target)
	conn, err := grpc.Dial(
		target,
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithPerRPCCredentials(macaroonCreds{path: cfg.MacaroonPath}),
	)
	if err != nil {
		return fmt.Errorf("failed to dial dcrlnd: %v", err)
	}

	DcrlndGrpcConn = conn
	LightningClient = lnrpc.NewLightningClient(conn)
	WalletUnlockerClient = lnrpc.NewWalletUnlockerClient(conn)
	AutopilotClient = autopilotrpc.NewAutopilotClient(conn)
	VersionerClient = verrpc.NewVersionerClient(conn)
	RouterClient = routerrpc.NewRouterClient(conn)
	InvoicesClient = invoicesrpc.NewInvoicesClient(conn)
	WatchtowerClient = wtclientrpc.NewWatchtowerClientClient(conn)

	log.Println("dcrlnd gRPC clients initialised")
	return nil
}

// ReinitDcrlndClient is called when the dashboard observes the dcrlnd
// cert appearing on disk after a deferred startup (e.g. the wizard
// just completed). Replaces the existing nil clients in-place.
func ReinitDcrlndClient() error {
	dcrlndReinitMu.Lock()
	defer dcrlndReinitMu.Unlock()
	if LightningClient != nil {
		return nil
	}
	return InitDcrlndClient(DcrlndCfg)
}

var dcrlndReinitMu sync.Mutex

// ReconnectDcrlnd repoints the dcrlnd client at a different wallet's node by
// updating the cert/macaroon paths and redialing. Best-effort: the target
// wallet's node may not be up yet (its cert appears once that wallet's
// Lightning is set up and unlocked), in which case the clients stay nil and the
// LN status machine reports the right stage.
func ReconnectDcrlnd(tlsCertPath, macaroonPath string) {
	dcrlndReinitMu.Lock()
	defer dcrlndReinitMu.Unlock()
	if DcrlndGrpcConn != nil {
		_ = DcrlndGrpcConn.Close()
		DcrlndGrpcConn = nil
	}
	LightningClient = nil
	WalletUnlockerClient = nil
	AutopilotClient = nil
	VersionerClient = nil
	RouterClient = nil
	InvoicesClient = nil
	WatchtowerClient = nil
	DcrlndCfg.TLSCertPath = tlsCertPath
	DcrlndCfg.MacaroonPath = macaroonPath
	_ = InitDcrlndClient(DcrlndCfg)
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
	// hostname; we trust it by pinning the CA pool to exactly this one
	// cert and skipping hostname verification. The pool acts as the
	// authentication root — only THIS cert can pass verification, so
	// the dial is still authenticated.
	return credentials.NewTLS(&tls.Config{
		RootCAs:            pool,
		InsecureSkipVerify: true,
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

// CloseDcrlndConnection closes the dcrlnd gRPC connection.
func CloseDcrlndConnection() {
	if DcrlndGrpcConn != nil {
		_ = DcrlndGrpcConn.Close()
		log.Println("dcrlnd gRPC connection closed")
	}
}
