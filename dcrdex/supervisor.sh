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
RUNNING_TOR_REV="__none__"

# Tor is toggled at runtime via the shared pointer the dashboard writes; the
# proxy endpoint comes from env. bisonw routes its DEX server connections
# through the SOCKS proxy. Parsed with sed (matches the rest of this file).
TOR_POINTER="/app-data/control/tor.json"

tor_field() {
    [ -f "${TOR_POINTER}" ] || { echo "$2"; return; }
    v=$(sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\{0,1\}\([A-Za-z0-9]*\).*/\1/p" "${TOR_POINTER}" | head -1)
    [ -n "${v}" ] && echo "${v}" || echo "$2"
}

build_tor_args() {
    TOR_ARGS=""
    [ "$(tor_field enabled false)" = "true" ] || return
    [ -n "${TOR_PROXY_IP}" ] && [ -n "${TOR_PROXY_PORT}" ] || return
    TOR_ARGS="--torproxy=${TOR_PROXY_IP}:${TOR_PROXY_PORT}"
    [ "$(tor_field isolation true)" = "true" ] && TOR_ARGS="${TOR_ARGS} --torisolation"
}

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
    tor_on=false
    [ -n "${CHILD_PID}" ] && [ "$(tor_field enabled false)" = "true" ] && tor_on=true
    cat > "${STATE}.tmp" <<EOF
{"running":"$1","appdata":"$2","pid":${pid:-0},"tor":${tor_on},"torRev":"${RUNNING_TOR_REV}"}
EOF
    mv "${STATE}.tmp" "${STATE}"
}

launch() {
    appdata="$1"
    mkdir -p "${appdata}"
    build_tor_args
    # shellcheck disable=SC2086
    ./bisonw \
        --appdata="${appdata}" \
        --rpc \
        --rpcaddr=0.0.0.0:5757 \
        --rpcuser="${RPC_USER}" \
        --rpcpass="${RPC_PASS}" \
        --rpccert="${appdata}/rpc.cert" \
        --rpckey="${appdata}/rpc.key" \
        --webaddr=0.0.0.0:5758 \
        --webtls ${TOR_ARGS} &
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

    TOR_REV=$(tor_field rev 0)
    if [ "${APPDATA}" != "${RUNNING_DIR}" ] || [ "${TOR_REV}" != "${RUNNING_TOR_REV}" ] || [ -z "${CHILD_PID}" ]; then
        if [ -n "${CHILD_PID}" ]; then
            echo "Restarting bisonw for wallet '${NAME}' (tor rev ${TOR_REV})"
            stop_child
        fi
        echo "Starting bisonw for wallet '${NAME}' (appdata ${APPDATA})"
        launch "${APPDATA}"
        RUNNING_DIR="${APPDATA}"
        RUNNING_TOR_REV="${TOR_REV}"
    fi

    write_state "${NAME}" "${APPDATA}"
    sleep 3
done
