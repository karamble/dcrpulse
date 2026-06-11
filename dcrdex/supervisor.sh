#!/bin/sh
# Copyright (c) 2015-2026 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

# Multi-wallet supervisor for bisonw (DCRDEX). Each wallet has its own DEX app
# seed, accounts, and bonds under its own appdata, trading from that wallet's
# "dex" account in dcrwallet. The bisonw image is built from upstream dcrdex, so
# this wrapper is mounted as the container entrypoint rather than baked in. It
# relaunches bisonw against the wallet named in the shared control pointer; the
# DEX onboarding stages (needs-init / needs-wallet / ready) resolve per wallet.
# Parses the pointer with sed (the image has no jq).
set +e

DEFAULT_WALLET_NAME="default-wallet"
SELECTED="/app-data/control/selected.json"
DEX_ROOT="/dex/.dexc"
STATE="${DEX_ROOT}/control-state.json"
RPC_USER="${DCRDEX_RPC_USER:-dcrdex}"
RPC_PASS="${DCRDEX_RPC_PASS:-dcrdexpass}"

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
        echo "${DEX_ROOT}"
    else
        echo "${DEX_ROOT}/wallets/$1"
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
    appdata="$1"
    mkdir -p "${appdata}"
    ./bisonw \
        --appdata="${appdata}" \
        --rpc \
        --rpcaddr=0.0.0.0:5757 \
        --rpcuser="${RPC_USER}" \
        --rpcpass="${RPC_PASS}" \
        --rpccert="${appdata}/rpc.cert" \
        --rpckey="${appdata}/rpc.key" \
        --webaddr=0.0.0.0:5758 \
        --webtls &
    CHILD_PID=$!
}

shutdown() {
    stop_child
    exit 0
}
trap shutdown INT TERM

while true; do
    NAME=$(read_selected)

    if [ -z "${NAME}" ]; then
        [ -n "${CHILD_PID}" ] && stop_child
        RUNNING_DIR="__none__"
        write_state "" ""
        sleep 3
        continue
    fi

    APPDATA=$(resolve_dir "${NAME}")

    if [ -n "${CHILD_PID}" ] && ! kill -0 "${CHILD_PID}" 2>/dev/null; then
        wait "${CHILD_PID}" 2>/dev/null
        CHILD_PID=""
        RUNNING_DIR="__none__"
    fi

    if [ "${APPDATA}" != "${RUNNING_DIR}" ] || [ -z "${CHILD_PID}" ]; then
        if [ -n "${CHILD_PID}" ]; then
            echo "Switching bisonw to wallet '${NAME}'"
            stop_child
        fi
        echo "Starting bisonw for wallet '${NAME}' (appdata ${APPDATA})"
        launch "${APPDATA}"
        RUNNING_DIR="${APPDATA}"
    fi

    write_state "${NAME}" "${APPDATA}"
    sleep 3
done
