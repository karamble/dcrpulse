// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"

	"dcrpulse/internal/handlers"
	"dcrpulse/internal/rpc"
)

//go:embed web/dist
var embeddedFiles embed.FS

func main() {
	// Load dcrd configuration from environment variables
	dcrdConfig := rpc.Config{
		RPCHost:     getEnv("DCRD_RPC_HOST", "localhost"),
		RPCPort:     getEnv("DCRD_RPC_PORT", "9109"),
		RPCUser:     getEnv("DCRD_RPC_USER", ""),
		RPCPassword: getEnv("DCRD_RPC_PASS", ""),
		RPCCert:     getEnv("DCRD_RPC_CERT", ""),
	}

	// Try to initialize dcrd RPC client if credentials are provided
	if dcrdConfig.RPCUser != "" && dcrdConfig.RPCPassword != "" {
		if err := rpc.InitDcrdClient(dcrdConfig); err != nil {
			log.Printf("Warning: Could not connect to dcrd on startup: %v", err)
			log.Println("RPC connection can be configured via API")
		}
	} else {
		log.Println("No dcrd RPC credentials provided. Use /api/connect endpoint to configure.")
	}

	// Load dcrwallet configuration from environment variables
	walletConfig := rpc.Config{
		RPCHost:     getEnv("DCRWALLET_RPC_HOST", "localhost"),
		RPCPort:     getEnv("DCRWALLET_RPC_PORT", "9110"),
		RPCUser:     getEnv("DCRWALLET_RPC_USER", ""),
		RPCPassword: getEnv("DCRWALLET_RPC_PASS", ""),
		RPCCert:     getEnv("DCRWALLET_RPC_CERT", ""),
	}

	// Try to initialize wallet RPC client if credentials are provided
	if walletConfig.RPCUser != "" && walletConfig.RPCPassword != "" {
		if err := rpc.InitWalletClient(walletConfig); err != nil {
			log.Printf("Warning: Could not connect to dcrwallet on startup: %v", err)
			log.Println("Wallet features will be unavailable")
		}
	} else {
		log.Println("No dcrwallet RPC credentials provided. Wallet features disabled.")
	}

	// Initialize wallet gRPC client for streaming
	grpcConfig := rpc.GrpcConfig{
		GrpcHost: getEnv("DCRWALLET_RPC_HOST", "localhost"),
		GrpcPort: getEnv("DCRWALLET_GRPC_PORT", "9111"),
		GrpcCert: getEnv("DCRWALLET_RPC_CERT", ""),
	}

	if grpcConfig.GrpcCert != "" {
		if err := rpc.InitWalletGrpcClient(grpcConfig); err != nil {
			log.Printf("Warning: Could not connect to dcrwallet gRPC on startup: %v", err)
			log.Println("Streaming features will be unavailable")
		}
	} else {
		log.Println("No gRPC certificate provided. Streaming features disabled.")
	}

	// Setup router
	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api").Subrouter()

	// Node/dcrd routes
	api.HandleFunc("/health", handlers.HealthCheckHandler).Methods("GET")
	api.HandleFunc("/dashboard", handlers.GetDashboardDataHandler).Methods("GET")
	api.HandleFunc("/node/status", handlers.GetNodeStatusHandler).Methods("GET")
	api.HandleFunc("/blockchain/info", handlers.GetBlockchainInfoHandler).Methods("GET")
	api.HandleFunc("/network/peers", handlers.GetPeersHandler).Methods("GET")
	api.HandleFunc("/connect", handlers.ConnectRPCHandler).Methods("POST")

	// Wallet routes
	api.HandleFunc("/wallet/status", handlers.GetWalletStatusHandler).Methods("GET")
	api.HandleFunc("/wallet/dashboard", handlers.GetWalletDashboardHandler).Methods("GET")
	api.HandleFunc("/wallet/transactions", handlers.ListTransactionsHandler).Methods("GET")
	api.HandleFunc("/wallet/importxpub", handlers.ImportXpubHandler).Methods("POST")
	api.HandleFunc("/wallet/rescan", handlers.RescanWalletHandler).Methods("POST")
	api.HandleFunc("/wallet/sync-progress", handlers.GetSyncProgressHandler).Methods("GET")

	// WebSocket streaming routes (log-based monitoring, does not start rescans)
	api.HandleFunc("/wallet/stream-rescan-progress", handlers.StreamRescanProgressHandler).Methods("GET")
	api.HandleFunc("/wallet/grpc/stream-rescan", handlers.StreamRescanGrpcHandler).Methods("GET")

	// Explorer routes
	api.HandleFunc("/explorer/search", handlers.SearchHandler).Methods("GET")
	api.HandleFunc("/explorer/blocks/recent", handlers.GetRecentBlocksHandler).Methods("GET")
	api.HandleFunc("/explorer/blocks/{height:[0-9]+}", handlers.GetBlockByHeightHandler).Methods("GET")
	api.HandleFunc("/explorer/blocks/hash/{hash}", handlers.GetBlockByHashHandler).Methods("GET")
	api.HandleFunc("/explorer/transactions/{txhash}", handlers.GetTransactionHandler).Methods("GET")
	api.HandleFunc("/explorer/address/{address}", handlers.GetAddressHandler).Methods("GET")

	// Treasury/Governance routes
	api.HandleFunc("/treasury/info", handlers.GetTreasuryInfoHandler).Methods("GET")
	api.HandleFunc("/treasury/scan-history", handlers.TriggerTSpendScanHandler).Methods("POST")
	api.HandleFunc("/treasury/scan-progress", handlers.GetTSpendScanProgressHandler).Methods("GET")
	api.HandleFunc("/treasury/scan-results", handlers.GetTSpendScanResultsHandler).Methods("GET")
	api.HandleFunc("/treasury/mempool", handlers.GetMempoolTSpendsHandler).Methods("GET")
	api.HandleFunc("/treasury/votes/{txhash}/progress", handlers.GetVoteParsingProgressHandler).Methods("GET")

	// Serve embedded static files for frontend
	distFS, err := fs.Sub(embeddedFiles, "web/dist")
	if err != nil {
		log.Printf("Warning: Could not load embedded frontend files: %v", err)
		log.Println("Frontend will not be available. This is expected in development mode.")
	} else {
		// Serve static files with SPA fallback
		r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			path := req.URL.Path

			// Skip API routes
			if strings.HasPrefix(path, "/api") {
				http.NotFound(w, req)
				return
			}

			// Try to serve the requested file
			if path != "/" {
				filePath := strings.TrimPrefix(path, "/")
				if _, err := distFS.Open(filePath); err == nil {
					http.FileServer(http.FS(distFS)).ServeHTTP(w, req)
					return
				}
			}

			// Fallback to index.html for SPA routing
			req.URL.Path = "/"
			http.FileServer(http.FS(distFS)).ServeHTTP(w, req)
		})
	}

	// Start server
	port := getEnv("PORT", "8080")
	address := fmt.Sprintf(":%s", port)

	log.Printf("Starting dcrpulse Dashboard server on %s", address)
	log.Println("Node endpoints: /api/dashboard, /api/node/*, /api/blockchain/*, /api/network/*")
	log.Println("Wallet endpoints: /api/wallet/status, /api/wallet/dashboard, /api/wallet/importxpub")
	log.Println("Wallet gRPC endpoints: /api/wallet/grpc/stream-rescan (real-time streaming)")
	log.Println("Explorer endpoints: /api/explorer/search, /api/explorer/blocks/*, /api/explorer/transactions/*")
	log.Println("Treasury endpoints: /api/treasury/info, /api/treasury/scan-history, /api/treasury/scan-progress")
	log.Println("Frontend: Embedded static files served at /")
	log.Fatal(http.ListenAndServe(address, r))
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
