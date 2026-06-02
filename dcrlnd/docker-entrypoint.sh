#!/bin/sh
# Copyright (c) 2015-2026 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

# Multi-wallet supervisor for dcrlnd. dcrlnd uses dcrwallet as its chain backend
# and funds channels from a per-wallet "lightning" account, so each wallet gets
# its own LN node under its own data directory. The dashboard writes the selected
# wallet to the shared control pointer; this loop relaunches dcrlnd against that
# wallet's directory. Only one wallet's node runs at a time; the container stays
# up while the inner dcrlnd process cycles. Parsing avoids jq (busybox sed only).
set +e

DEFAULT_WALLET_NAME="default-wallet"
SELECTED="/app-data/control/selected.json"
DCRLND_ROOT="/app-data/dcrlnd"
STATE="${DCRLND_ROOT}/control-state.json"
WALLET_CERT="${WALLET_CERT:-/app-data/dcrd/rpc.cert}"
WALLET_KEY="${WALLET_KEY:-${WALLET_CERT%.cert}.key}"

CHILD_PID=""
RUNNING_DIR="__none__"

read_selected() {
    if [ ! -f "${SELECTED}" ]; then
        echo "${DEFAULT_WALLET_NAME}"
        return
    fi
    sed -n 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "${SELECTED}" | head -1
}

resolve_dir() {
    if [ "$1" = "${DEFAULT_WALLET_NAME}" ]; then
        echo "${DCRLND_ROOT}"
    else
        echo "${DCRLND_ROOT}/wallets/$1"
    fi
}

stop_child() {
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
{"running":"$1","appdata":"$2","pid":${pid:-0}}
EOF
    mv "${STATE}.tmp" "${STATE}"
}

launch() {
    dir="$1"
    acct="$2"
    mkdir -p "${dir}/logs" "${dir}/data"
    NETWORK_ARGS=""
    [ "${LN_TESTNET}" = "true" ] && NETWORK_ARGS="--testnet"
    # shellcheck disable=SC2086
    dcrlnd \
        --nolisten \
        --norest \
        --logdir="${dir}/logs" \
        --datadir="${dir}/data" \
        --tlscertpath="${dir}/tls.cert" \
        --tlskeypath="${dir}/tls.key" \
        --configfile="${dir}/dcrlnd.conf" \
        --adminmacaroonpath="${dir}/admin.macaroon" \
        --node=dcrw \
        --dcrwallet.grpchost="${DCRWALLET_HOST:-dcrwallet}:${DCRWALLET_GRPC_PORT:-9111}" \
        --dcrwallet.certpath="${WALLET_CERT}" \
        --dcrwallet.clientcertpath="${WALLET_CERT}" \
        --dcrwallet.clientkeypath="${WALLET_KEY}" \
        --dcrwallet.accountnumber="${acct}" \
        --rpclisten=0.0.0.0:10009 \
        --tlsextradomain="${DCRLND_TLS_EXTRA_DOMAIN:-dcrlnd}" \
        --wtclient.active \
        --wtclient.sweep-fee-rate=10000000 \
        ${NETWORK_ARGS} &
    CHILD_PID=$!
}

shutdown() {
    stop_child
    exit 0
}
trap shutdown INT TERM

# Wait once for dcrwallet's shared TLS cert; without it the gRPC handshake fails.
if [ ! -f "${WALLET_CERT}" ]; then
    echo "Waiting for dcrwallet rpc.cert..."
    i=0
    while [ ! -f "${WALLET_CERT}" ]; do
        i=$((i + 1))
        [ "${i}" -ge 120 ] && break
        sleep 1
    done
fi

while true; do
    NAME=$(read_selected)

    # No wallet selected: idle with no LN node running.
    if [ -z "${NAME}" ]; then
        [ -n "${CHILD_PID}" ] && stop_child
        RUNNING_DIR="__none__"
        write_state "" ""
        sleep 3
        continue
    fi

    DIR=$(resolve_dir "${NAME}")
    ACCOUNT_FILE="${DIR}/.account"

    # Reap an unexpectedly exited child so it is relaunched below.
    if [ -n "${CHILD_PID}" ] && ! kill -0 "${CHILD_PID}" 2>/dev/null; then
        wait "${CHILD_PID}" 2>/dev/null
        CHILD_PID=""
        RUNNING_DIR="__none__"
    fi

    # Lightning not set up for this wallet yet: the dashboard writes .account
    # after the user creates the dedicated account. Idle until then.
    if [ ! -f "${ACCOUNT_FILE}" ]; then
        [ -n "${CHILD_PID}" ] && stop_child
        RUNNING_DIR="__none__"
        write_state "${NAME}" ""
        sleep 3
        continue
    fi

    if [ "${DIR}" != "${RUNNING_DIR}" ] || [ -z "${CHILD_PID}" ]; then
        if [ -n "${CHILD_PID}" ]; then
            echo "Switching dcrlnd to wallet '${NAME}'"
            stop_child
        fi
        LN_ACCOUNT=$(cat "${ACCOUNT_FILE}")
        echo "Starting dcrlnd for wallet '${NAME}' (dir ${DIR}, account ${LN_ACCOUNT})"
        launch "${DIR}" "${LN_ACCOUNT}"
        RUNNING_DIR="${DIR}"
    fi

    write_state "${NAME}" "${DIR}"
    sleep 3
done
