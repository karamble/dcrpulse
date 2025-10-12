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

	pb "decred.org/dcrwallet/v4/rpc/walletrpc"
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

	// WalletGrpcConn is the gRPC connection (kept for cleanup)
	WalletGrpcConn *grpc.ClientConn
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

// InitDcrdClient initializes the dcrd RPC client
func InitDcrdClient(config Config) error {
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

	log.Println("Successfully connected to dcrd RPC with TLS")
	return nil
}

// InitWalletClient initializes the dcrwallet RPC client
func InitWalletClient(config Config) error {
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
	} else {
		log.Println("Successfully connected to dcrwallet RPC with TLS")
	}

	return nil
}

// InitWalletGrpcClient initializes the dcrwallet gRPC client for streaming with mutual TLS
func InitWalletGrpcClient(config GrpcConfig) error {
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
	cert, err := tls.LoadX509KeyPair(config.GrpcCert, "/certs/rpc.key")
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

	log.Println("dcrwallet gRPC client initialized with mutual TLS authentication")
	return nil
}

// CloseGrpcConnection closes the gRPC connection
func CloseGrpcConnection() {
	if WalletGrpcConn != nil {
		WalletGrpcConn.Close()
		log.Println("gRPC connection closed")
	}
}
