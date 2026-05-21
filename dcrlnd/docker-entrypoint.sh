#!/bin/sh
# Copyright (c) 2015-2025 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

set -e

DCRLND_DIR="/app-data/dcrlnd"
ACCOUNT_FILE="${DCRLND_DIR}/.account"
WALLET_CERT="${WALLET_CERT:-/app-data/dcrd/rpc.cert}"
WALLET_KEY="${WALLET_KEY:-${WALLET_CERT%.cert}.key}"

mkdir -p "${DCRLND_DIR}/logs" "${DCRLND_DIR}/data"

# Wait for dcrwallet to publish its TLS cert (shared via the /app-data
# volume). Without this we'd fail the gRPC handshake on first start.
if [ ! -f "${WALLET_CERT}" ]; then
    echo "Waiting for dcrwallet rpc.cert..."
    for i in $(seq 1 120); do
        if [ -f "${WALLET_CERT}" ]; then
            break
        fi
        sleep 1
    done
    if [ ! -f "${WALLET_CERT}" ]; then
        echo "ERROR: dcrwallet rpc.cert never appeared at ${WALLET_CERT}"
        exit 1
    fi
fi
echo "✓ Found dcrwallet rpc.cert"

# Wait for the dashboard's setup wizard to create the dedicated lightning
# account in dcrwallet and write its number here. Decrediton spawns dcrlnd
# only after the user picks an account; the docker-compose equivalent is
# this sentinel file. Polled forever — exiting would just restart-loop us.
if [ ! -f "${ACCOUNT_FILE}" ]; then
    echo "Waiting for Lightning setup wizard to create ${ACCOUNT_FILE}..."
    while [ ! -f "${ACCOUNT_FILE}" ]; do
        sleep 5
    done
fi
LN_ACCOUNT=$(cat "${ACCOUNT_FILE}")
echo "✓ Using dcrwallet account ${LN_ACCOUNT} for Lightning funds"

# Optional testnet override. Decrediton appends --testnet conditionally
# based on user state; we drive it from a compose env var.
NETWORK_ARGS=""
if [ "${LN_TESTNET}" = "true" ]; then
    NETWORK_ARGS="--testnet"
    echo "✓ Running on testnet"
fi

# Decrediton's exact flag set from app/main_dev/launch.js:863-878, with
# two changes:
#   --rpclisten=0.0.0.0:10009 (Decrediton uses default localhost; we need
#                              the dashboard container to reach this one),
#   account number sourced from the sentinel above.
exec dcrlnd \
    --nolisten \
    --norest \
    --logdir="${DCRLND_DIR}/logs" \
    --datadir="${DCRLND_DIR}/data" \
    --tlscertpath="${DCRLND_DIR}/tls.cert" \
    --tlskeypath="${DCRLND_DIR}/tls.key" \
    --configfile="${DCRLND_DIR}/dcrlnd.conf" \
    --adminmacaroonpath="${DCRLND_DIR}/admin.macaroon" \
    --node=dcrw \
    --dcrwallet.grpchost="${DCRWALLET_HOST:-dcrwallet}:${DCRWALLET_GRPC_PORT:-9111}" \
    --dcrwallet.certpath="${WALLET_CERT}" \
    --dcrwallet.clientcertpath="${WALLET_CERT}" \
    --dcrwallet.clientkeypath="${WALLET_KEY}" \
    --dcrwallet.accountnumber="${LN_ACCOUNT}" \
    --rpclisten=0.0.0.0:10009 \
    --tlsextradomain="${DCRLND_TLS_EXTRA_DOMAIN:-dcrlnd}" \
    --wtclient.active \
    --wtclient.sweep-fee-rate=10000000 \
    ${NETWORK_ARGS} \
    "$@"
