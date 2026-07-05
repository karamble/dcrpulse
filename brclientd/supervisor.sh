#!/bin/sh
# Copyright (c) 2015-2026 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

# Multi-wallet supervisor for brclientd. Each wallet has its own Bison Relay
# identity under its own appdata, paying through that wallet's dcrlnd node. The
# brclientd image is built from an external repo, so this wrapper is mounted as
# the container entrypoint rather than baked in. It relaunches brclientd against
# the wallet named in the shared control pointer. Bison Relay requires Lightning,
# so it idles until the active wallet's dcrlnd node has published its cert.
# Parses the pointer with sed (the image has no jq).
set +e

DEFAULT_WALLET_NAME="default-wallet"
SELECTED="/app-data/control/selected.json"
BR_ROOT="/app-data/brclientd"
DCRLND_ROOT="/app-data/dcrlnd"
STATE="${BR_ROOT}/control-state.json"
BIN="${BRCLIENTD_BIN:-/usr/local/bin/brclientd}"

CHILD_PID=""
RUNNING_DIR="__none__"
RUNNING_TOR_REV="__none__"

# Tor is toggled at runtime via the shared pointer the dashboard writes; the
# proxy endpoint comes from env. These flags route brclientd's Bison Relay
# relay/seeder connection through Tor (the dcrlnd gRPC connection is local and
# stays direct). Mirrors dcrd. Parsed with sed (the image has no jq).
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
    TOR_ARGS="--proxy=${TOR_PROXY_IP}:${TOR_PROXY_PORT}"
    [ "$(tor_field isolation true)" = "true" ] && TOR_ARGS="${TOR_ARGS} --torisolation --circuitlimit=$(tor_field circuitLimit 32)"
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
        echo "${2}"
    else
        echo "${2}/wallets/$1"
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
    lndir="$2"
    mkdir -p "${appdata}"
    build_tor_args
    # shellcheck disable=SC2086
    "${BIN}" \
        --appdata="${appdata}" \
        --dcrlnd.rpchost=dcrlnd:10009 \
        --dcrlnd.tlscertpath="${lndir}/tls.cert" \
        --dcrlnd.macaroonpath="${lndir}/admin.macaroon" \
        --mcp.mcplisten=0.0.0.0:8891 \
        ${TOR_ARGS} &
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

    APPDATA=$(resolve_dir "${NAME}" "${BR_ROOT}")
    LNDIR=$(resolve_dir "${NAME}" "${DCRLND_ROOT}")

    if [ -n "${CHILD_PID}" ] && ! kill -0 "${CHILD_PID}" 2>/dev/null; then
        wait "${CHILD_PID}" 2>/dev/null
        CHILD_PID=""
        RUNNING_DIR="__none__"
    fi

    # Bison Relay needs the active wallet's Lightning node. Idle until its cert
    # exists (set up + first run complete).
    if [ ! -f "${LNDIR}/tls.cert" ]; then
        [ -n "${CHILD_PID}" ] && stop_child
        RUNNING_DIR="__none__"
        write_state "${NAME}" ""
        sleep 3
        continue
    fi

    TOR_REV=$(tor_field rev 0)
    if [ "${APPDATA}" != "${RUNNING_DIR}" ] || [ "${TOR_REV}" != "${RUNNING_TOR_REV}" ] || [ -z "${CHILD_PID}" ]; then
        if [ -n "${CHILD_PID}" ]; then
            echo "Restarting brclientd for wallet '${NAME}' (tor rev ${TOR_REV})"
            stop_child
        fi
        echo "Starting brclientd for wallet '${NAME}' (appdata ${APPDATA})"
        launch "${APPDATA}" "${LNDIR}"
        RUNNING_DIR="${APPDATA}"
        RUNNING_TOR_REV="${TOR_REV}"
    fi

    write_state "${NAME}" "${APPDATA}"
    sleep 3
done
