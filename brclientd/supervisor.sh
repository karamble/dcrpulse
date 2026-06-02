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
    cat > "${STATE}.tmp" <<EOF
{"running":"$1","appdata":"$2","pid":${pid:-0}}
EOF
    mv "${STATE}.tmp" "${STATE}"
}

launch() {
    appdata="$1"
    lndir="$2"
    mkdir -p "${appdata}"
    "${BIN}" \
        --appdata="${appdata}" \
        --dcrlnd.rpchost=dcrlnd:10009 \
        --dcrlnd.tlscertpath="${lndir}/tls.cert" \
        --dcrlnd.macaroonpath="${lndir}/admin.macaroon" &
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

    if [ "${APPDATA}" != "${RUNNING_DIR}" ] || [ -z "${CHILD_PID}" ]; then
        if [ -n "${CHILD_PID}" ]; then
            echo "Switching brclientd to wallet '${NAME}'"
            stop_child
        fi
        echo "Starting brclientd for wallet '${NAME}' (appdata ${APPDATA})"
        launch "${APPDATA}" "${LNDIR}"
        RUNNING_DIR="${APPDATA}"
    fi

    write_state "${NAME}" "${APPDATA}"
    sleep 3
done
