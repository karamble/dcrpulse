// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/rpcclient/v8"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	// DcrdClient is the RPC client for dcrd
	DcrdClient *rpcclient.Client

	// WalletClient is the RPC client for dcrwallet (JSON-RPC)
	WalletClient *rpcclient.Client

	// WalletGrpcClient is the gRPC client for dcrwallet (for streaming)
	WalletGrpcClient pb.WalletServiceClient

	// WalletLoaderClient is the gRPC client for wallet lifecycle management
	WalletLoaderClient pb.WalletLoaderServiceClient

	// SeedServiceClient is the gRPC client for seed generation
	SeedServiceClient pb.SeedServiceClient

	// DecodeMessageClient is the gRPC client for decoding raw transactions
	DecodeMessageClient pb.DecodeMessageServiceClient

	// AccountMixerClient is the gRPC client for running the P2P CoinJoin mixer
	AccountMixerClient pb.AccountMixerServiceClient

	// TicketBuyerClient is the gRPC client for the ticket autobuyer (v2)
	TicketBuyerClient pb.TicketBuyerServiceClient

	// VotingClient is the gRPC client for agenda voting
	VotingClient pb.VotingServiceClient

	// WalletGrpcConn is the gRPC connection (kept for cleanup)
	WalletGrpcConn *grpc.ClientConn

	// WalletGrpcCfg stores the gRPC connection details so the connection can
	// be rebuilt after dcrwallet relaunches against a different wallet.
	WalletGrpcCfg GrpcConfig

	// DcrdConfig stores the dcrd connection details for RpcSync
	DcrdConfig Config

	// WalletConfig stores the dcrwallet JSON-RPC connection details, used to
	// report whether that connection is encrypted.
	WalletConfig Config
)

// Config holds the RPC connection configuration
type Config struct {
	RPCHost     string
	RPCPort     string
	RPCUser     string
	RPCPassword string
	RPCCert     string
}

// GrpcConfig holds the gRPC connection configuration
type GrpcConfig struct {
	GrpcHost string
	GrpcPort string
	GrpcCert string
}

// DcrdUsesTLS reports whether the dcrd JSON-RPC connection is configured with
// TLS (a cert was provided). When false the connection is plaintext.
func DcrdUsesTLS() bool { return DcrdConfig.RPCCert != "" }

// WalletUsesTLS reports whether the dcrwallet JSON-RPC connection is configured
// with TLS (a cert was provided). When false the connection is plaintext.
func WalletUsesTLS() bool { return WalletConfig.RPCCert != "" }

// InitDcrdClient initializes the dcrd RPC client
func InitDcrdClient(config Config) error {
	// Store config for later use (e.g., RpcSync)
	DcrdConfig = config

	// Read the TLS certificate if provided
	var certs []byte
	var err error

	if config.RPCCert != "" {
		log.Printf("Reading TLS certificate from: %s", config.RPCCert)
		certs, err = ioutil.ReadFile(config.RPCCert)
		if err != nil {
			return fmt.Errorf("failed to read RPC certificate: %v", err)
		}
		log.Printf("Successfully loaded TLS certificate (%d bytes)", len(certs))
	}

	connCfg := &rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", config.RPCHost, config.RPCPort),
		Endpoint:     "ws",
		User:         config.RPCUser,
		Pass:         config.RPCPassword,
		HTTPPostMode: true,
		DisableTLS:   config.RPCCert == "", // Disable TLS only if no cert provided
		Certificates: certs,
	}

	DcrdClient, err = rpcclient.New(connCfg, nil)
	if err != nil {
		return fmt.Errorf("failed to create RPC client: %v", err)
	}

	// Test connection
	ctx := context.Background()
	_, err = DcrdClient.GetBlockCount(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to dcrd: %v", err)
	}

	if config.RPCCert == "" {
		log.Println("WARNING: dcrd RPC connection is NOT using TLS; the RPC username, password, and all traffic are sent in cleartext. Set DCRD_RPC_CERT to enable TLS.")
	} else {
		log.Println("Successfully connected to dcrd RPC with TLS")
	}
	return nil
}

// InitWalletClient initializes the dcrwallet RPC client
func InitWalletClient(config Config) error {
	// Store config so the TLS status can be reported later.
	WalletConfig = config

	// Read the TLS certificate if provided
	var certs []byte
	var err error

	if config.RPCCert != "" {
		log.Printf("Reading wallet TLS certificate from: %s", config.RPCCert)
		certs, err = ioutil.ReadFile(config.RPCCert)
		if err != nil {
			return fmt.Errorf("failed to read wallet RPC certificate: %v", err)
		}
		log.Printf("Successfully loaded wallet TLS certificate (%d bytes)", len(certs))
	}

	connCfg := &rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", config.RPCHost, config.RPCPort),
		Endpoint:     "ws",
		User:         config.RPCUser,
		Pass:         config.RPCPassword,
		HTTPPostMode: true,
		DisableTLS:   config.RPCCert == "", // Disable TLS only if no cert provided
		Certificates: certs,
	}

	WalletClient, err = rpcclient.New(connCfg, nil)
	if err != nil {
		return fmt.Errorf("failed to create wallet RPC client: %v", err)
	}

	// Test connection with getinfo
	ctx := context.Background()
	_, err = WalletClient.GetInfo(ctx)
	if err != nil {
		// Wallet might be locked or not initialized, but connection is OK
		log.Printf("Wallet RPC connected but getinfo failed (may be locked): %v", err)
	}
	if config.RPCCert == "" {
		log.Println("WARNING: dcrwallet RPC connection is NOT using TLS; the RPC username, password, and all traffic are sent in cleartext. Set DCRWALLET_RPC_CERT to enable TLS.")
	} else if err == nil {
		log.Println("Successfully connected to dcrwallet RPC with TLS")
	}

	return nil
}

// InitWalletGrpcClient initializes the dcrwallet gRPC client for streaming with mutual TLS
func InitWalletGrpcClient(config GrpcConfig) error {
	WalletGrpcCfg = config
	return dialWalletGrpc(config)
}

// dialWalletGrpc dials dcrwallet's gRPC server with mutual TLS and (re)assigns
// every package-level client. Shared by InitWalletGrpcClient and
// ReconnectWalletGrpc.
func dialWalletGrpc(config GrpcConfig) error {
	// Load the certificate as both CA (to verify server) and client cert (to present to server)
	certPool := x509.NewCertPool()
	certPEM, err := os.ReadFile(config.GrpcCert)
	if err != nil {
		return fmt.Errorf("failed to read certificate: %v", err)
	}
	if !certPool.AppendCertsFromPEM(certPEM) {
		return fmt.Errorf("failed to add certificate to pool")
	}

	// Load the client certificate and key (same files used by dcrwallet)
	// We use the same cert/key that dcrwallet uses, enabling mutual TLS
	// Derive key path from cert path (replace .cert with .key)
	keyPath := strings.Replace(config.GrpcCert, ".cert", ".key", 1)
	cert, err := tls.LoadX509KeyPair(config.GrpcCert, keyPath)
	if err != nil {
		return fmt.Errorf("failed to load client certificate/key pair: %v", err)
	}

	// Create TLS config with both client certificate and server CA
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert}, // Client certificate to present
		RootCAs:      certPool,                // CA to verify server certificate
		ServerName:   config.GrpcHost,         // Expected server name
	}

	creds := credentials.NewTLS(tlsConfig)

	// Dial the gRPC server (non-blocking)
	target := fmt.Sprintf("%s:%s", config.GrpcHost, config.GrpcPort)
	log.Printf("Connecting to dcrwallet gRPC at %s with mutual TLS (non-blocking)", target)

	conn, err := grpc.Dial(
		target,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return fmt.Errorf("failed to create wallet gRPC connection: %v", err)
	}

	WalletGrpcConn = conn
	WalletGrpcClient = pb.NewWalletServiceClient(conn)
	WalletLoaderClient = pb.NewWalletLoaderServiceClient(conn)
	SeedServiceClient = pb.NewSeedServiceClient(conn)
	DecodeMessageClient = pb.NewDecodeMessageServiceClient(conn)
	AccountMixerClient = pb.NewAccountMixerServiceClient(conn)
	TicketBuyerClient = pb.NewTicketBuyerServiceClient(conn)
	VotingClient = pb.NewVotingServiceClient(conn)

	log.Println("dcrwallet gRPC clients initialized with mutual TLS authentication")
	return nil
}

// ReconnectWalletGrpc tears down the existing gRPC connection and re-dials.
// Required after dcrwallet is relaunched against a different wallet: the prior
// ClientConn points at a process that has exited, so its streams are dead and
// the loader client must be re-pointed. Must only be called while the RpcSync
// supervisor is paused (no concurrent gRPC calls in flight).
func ReconnectWalletGrpc() error {
	if WalletGrpcCfg.GrpcCert == "" {
		return fmt.Errorf("wallet gRPC not configured")
	}
	if WalletGrpcConn != nil {
		WalletGrpcConn.Close()
	}
	return dialWalletGrpc(WalletGrpcCfg)
}

// WaitForWalletDaemon blocks until the dcrwallet daemon answers a WalletExists
// loader call or ctx expires. Used after a relaunch to confirm the new process
// is listening before opening the wallet.
func WaitForWalletDaemon(ctx context.Context) error {
	for {
		if WalletLoaderClient != nil {
			callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			_, err := WalletLoaderClient.WalletExists(callCtx, &pb.WalletExistsRequest{})
			cancel()
			if err == nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

// CloseGrpcConnection closes the gRPC connection
func CloseGrpcConnection() {
	if WalletGrpcConn != nil {
		WalletGrpcConn.Close()
		log.Println("gRPC connection closed")
	}
}
