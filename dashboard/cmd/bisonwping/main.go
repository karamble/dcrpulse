// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Command bisonwping is a connectivity check for the backend-only bisonw RPC
// server. It calls the "version" route (which does not require an initialized
// app) and prints the result, proving end-to-end transport, TLS and auth.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"dcrpulse/pkg/bisonw"
)

func main() {
	cfg := bisonw.Config{
		Addr:     env("DCRDEX_RPC_ADDR", "dcrdex:"+bisonw.DefaultRPCPort),
		User:     env("DCRDEX_RPC_USER", "dcrdex"),
		Pass:     env("DCRDEX_RPC_PASS", "dcrdexpass"),
		CertPath: env("DCRDEX_RPC_CERT", "/app-data/dcrdex/rpc.cert"),
	}
	client, err := bisonw.New(cfg)
	if err != nil {
		fail(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	v, err := client.Version(ctx)
	if err != nil {
		fail(err)
	}
	fmt.Printf("OK bisonw reachable at %s\n  rpcServerVersion=%s\n  dexcVersion=%s\n",
		cfg.Addr, v.RPCServerVersion, v.BisonwVersion)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "bisonwping:", err)
	os.Exit(1)
}
