#!/bin/sh
# Copyright (c) 2015-2025 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

set -e

WALLET_DIR="/app-data/dcrwallet"
CERT_DIR="/app-data/dcrd"
RPC_CERT="${CERT_DIR}/rpc.cert"
RPC_KEY="${CERT_DIR}/rpc.key"

mkdir -p "${WALLET_DIR}"
# Pre-create mountpoints inside the shared /app-data volume so downstream
# containers (dcrlnd, brclientd, dashboard) can bind their own data on top
# of the read-only outer /app-data mount. Required for both fresh installs
# and upgrades where new containers add nested mounts.
mkdir -p /app-data/dcrlnd /app-data/brclientd /app-data/dcrdex

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

# Check for Tor proxy configuration
TOR_ARGS=""
if [ -n "$TOR_PROXY_IP" ] && [ -n "$TOR_PROXY_PORT" ]; then
    TOR_ARGS="--proxy=${TOR_PROXY_IP}:${TOR_PROXY_PORT} --torisolation --nodcrdproxy"
    echo "✓ Using Tor proxy at ${TOR_PROXY_IP}:${TOR_PROXY_PORT} with stream isolation (dcrd connections excluded)"
fi

echo ""
echo "Wallet creation available via web interface"
echo ""

# Multi-wallet supervisor. dcrwallet serves one wallet per process, with the
# wallet directory fixed by --appdata at launch, so switching wallets means
# relaunching the daemon against a different appdata. The dashboard cannot
# control this container, so it writes the selected wallet to a pointer file in
# the shared control directory; this loop launches dcrwallet for that wallet and
# relaunches it when the selection changes. The container stays up; only the
# inner dcrwallet process cycles.
# The supervisor must survive transient errors (a malformed pointer, a guarded
# signal to a dead pid), so relax the fail-fast set in the preamble.
set +e

DEFAULT_WALLET_NAME="default-wallet"
CONTROL_DIR="${WALLET_DIR}/control"
# Shared, dashboard-written selected-wallet pointer read by every service
# supervisor. State stays per service.
SELECTED="/app-data/control/selected.json"
STATE="${CONTROL_DIR}/state.json"
mkdir -p "${CONTROL_DIR}"

CHILD_PID=""
RUNNING_NAME=""
RUNNING_APPDATA=""

start_wallet() {
    appdata="$1"
    mkdir -p "${appdata}"
    # shellcheck disable=SC2086
    dcrwallet \
        --appdata="${appdata}" \
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
        --gaplimit="${DCRWALLET_GAP_LIMIT:-400}" \
        --noinitialload \
        --mixing \
        $TOR_ARGS &
    CHILD_PID=$!
}

stop_wallet() {
    [ -z "${CHILD_PID}" ] && return
    kill -INT "${CHILD_PID}" 2>/dev/null
    i=0
    while kill -0 "${CHILD_PID}" 2>/dev/null; do
        i=$((i + 1))
        if [ "${i}" -ge 30 ]; then
            kill -KILL "${CHILD_PID}" 2>/dev/null
            break
        fi
        sleep 1
    done
    wait "${CHILD_PID}" 2>/dev/null
    CHILD_PID=""
}

write_state() {
    pid="${CHILD_PID:-0}"
    cat > "${STATE}.tmp" <<EOF
{"running":"${RUNNING_NAME}","appdata":"${RUNNING_APPDATA}","pid":${pid:-0},"epoch":${1:-0}}
EOF
    mv "${STATE}.tmp" "${STATE}"
}

shutdown() {
    stop_wallet
    exit 0
}
trap shutdown INT TERM

while true; do
    # Default to the legacy single-wallet appdata so fresh installs and
    # upgraded watch-only deployments come up exactly as before until the
    # dashboard writes a selection.
    DESIRED_NAME="${DEFAULT_WALLET_NAME}"
    DESIRED_APPDATA="${WALLET_DIR}"
    EPOCH=0
    if [ -f "${SELECTED}" ]; then
        DESIRED_NAME=$(jq -r '.name // ""' "${SELECTED}" 2>/dev/null)
        sel_appdata=$(jq -r '.appdata // ""' "${SELECTED}" 2>/dev/null)
        EPOCH=$(jq -r '.epoch // 0' "${SELECTED}" 2>/dev/null)
        [ -n "${sel_appdata}" ] && DESIRED_APPDATA="${sel_appdata}"
    fi

    # Empty name means the user closed the wallet: idle with no child running.
    if [ -z "${DESIRED_NAME}" ]; then
        [ -n "${CHILD_PID}" ] && stop_wallet
        RUNNING_NAME=""
        RUNNING_APPDATA=""
        write_state "${EPOCH}"
        sleep 2
        continue
    fi

    # Reap an unexpectedly exited child so it is relaunched below.
    if [ -n "${CHILD_PID}" ] && ! kill -0 "${CHILD_PID}" 2>/dev/null; then
        wait "${CHILD_PID}" 2>/dev/null
        CHILD_PID=""
        RUNNING_APPDATA=""
    fi

    if [ "${DESIRED_APPDATA}" != "${RUNNING_APPDATA}" ] || [ -z "${CHILD_PID}" ]; then
        if [ -n "${CHILD_PID}" ]; then
            echo "Switching wallet: '${RUNNING_NAME}' -> '${DESIRED_NAME}'"
            stop_wallet
        fi
        echo "Starting dcrwallet for wallet '${DESIRED_NAME}' (appdata ${DESIRED_APPDATA})"
        start_wallet "${DESIRED_APPDATA}"
        RUNNING_NAME="${DESIRED_NAME}"
        RUNNING_APPDATA="${DESIRED_APPDATA}"
    fi

    write_state "${EPOCH}"
    sleep 2
done
