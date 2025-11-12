#!/bin/sh
# Copyright (c) 2015-2025 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

set -e

WALLET_DIR="/home/dcrwallet/.dcrwallet"
CERT_DIR="/certs"
RPC_CERT="${CERT_DIR}/rpc.cert"
RPC_KEY="${CERT_DIR}/rpc.key"

# Create wallet directory if it doesn't exist
mkdir -p "${WALLET_DIR}"

# Verify RPC certificates exist (shared from dcrd)
if [ ! -f "${RPC_CERT}" ] || [ ! -f "${RPC_KEY}" ]; then
    echo "WARNING: RPC certificates not found at ${CERT_DIR}"
    echo "Waiting for dcrd to generate certificates..."
    
    # Wait up to 60 seconds for certificates
    for i in $(seq 1 60); do
        if [ -f "${RPC_CERT}" ] && [ -f "${RPC_KEY}" ]; then
            echo "✓ RPC certificates found"
            break
        fi
        sleep 1
        if [ $i -eq 60 ]; then
            echo "ERROR: Timeout waiting for RPC certificates"
            exit 1
        fi
    done
else
    echo "✓ Using RPC certificates from dcrd"
fi

# Start dcrwallet with configuration
echo "Starting dcrwallet..."
echo "Configuration:"
echo "  RPC Listen: 0.0.0.0:9110"
echo "  gRPC Listen: 0.0.0.0:9111"
echo "  dcrd Connection: ${DCRD_RPC_HOST:-dcrd}:9109"
echo ""
echo "Wallet will be created via the web interface"
echo ""

# Execute dcrwallet with all provided arguments
# Note: Wallet is created via gRPC LoaderService API from frontend
exec dcrwallet \
    --username="${DCRWALLET_RPC_USER}" \
    --password="${DCRWALLET_RPC_PASS}" \
    --rpclisten=0.0.0.0:9110 \
    --rpccert="${RPC_CERT}" \
    --rpckey="${RPC_KEY}" \
    --dcrdusername="${DCRD_RPC_USER}" \
    --dcrdpassword="${DCRD_RPC_PASS}" \
    --rpcconnect="${DCRD_RPC_HOST:-dcrd}:9109" \
    --cafile="${RPC_CERT}" \
    --grpclisten=0.0.0.0:9111 \
    --clientcafile="${RPC_CERT}" \
    --noinitialload \
    "$@"
