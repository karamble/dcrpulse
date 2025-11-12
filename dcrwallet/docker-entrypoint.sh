#!/bin/sh
# Copyright (c) 2015-2025 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

set -e

WALLET_DIR="/app-data/dcrwallet"
CERT_DIR="/app-data/certs"
RPC_CERT="${CERT_DIR}/rpc.cert"
RPC_KEY="${CERT_DIR}/rpc.key"

mkdir -p "${WALLET_DIR}"

if [ ! -f "${RPC_CERT}" ] || [ ! -f "${RPC_KEY}" ]; then
    echo "Waiting for dcrd to generate certificates..."
    
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

echo "Starting dcrwallet..."
echo "  RPC: 0.0.0.0:9110"
echo "  gRPC: 0.0.0.0:9111"
echo "  dcrd: ${DCRD_RPC_HOST:-dcrd}:9109"
echo ""
echo "Wallet creation available via web interface"
echo ""

exec dcrwallet \
    --appdata="${WALLET_DIR}" \
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
