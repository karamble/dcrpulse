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
RUNNING_MAC_STAMP="__none__"

# Tor is toggled at runtime via the shared pointer the dashboard writes; the
# proxy endpoint comes from env. These flags route brclientd's Bison Relay
# relay/seeder connection through Tor (the dcrlnd gRPC connection is local and
# stays direct). Mirrors dcrd. Parsed with sed (the image has no jq).
TOR_POINTER="/app-data/control/tor.json"

tor_field() {
    [ -f "${TOR_POINTER}" ] || { echo "$2"; return; }
    v=$(sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\{0,1\}\([A-Za-z0-9]*\).*/\1/p" "${TOR_POINTER}" 2>/dev/null | head -1)
    # A pointer that exists but gives no clear answer is damaged; read it as
    # Tor on, as the dashboard does, rather than dropping to clearnet.
    if [ "$1" = enabled ] && [ "${v}" != true ] && [ "${v}" != false ]; then echo true; return; fi
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

# The dashboard marks a deleted wallet here but cannot write this volume, so
# its tree goes on the next pass; never the selected or default wallet.
PURGE_DIR="/app-data/control/purge"
purge_deleted() {
    for f in "${PURGE_DIR}"/*; do
        [ -f "${f}" ] || continue
        n=${f##*/}
        case "${n}" in *[!A-Za-z0-9_-]*|"${DEFAULT_WALLET_NAME}"|"${NAME}") continue ;; esac
        d="${BR_ROOT}/wallets/${n}"
        [ -d "${d}" ] && rm -rf "${d}" && echo "Removed data of deleted wallet '${n}'"
    done
}

stop_child() {
    [ -z "${CHILD_PID}" ] && return
    # Decrediton's stop: interrupt, interrupt again after two seconds, then wait
    # as long as the daemon needs; docker's stop_grace_period is the only limit.
    kill -INT "${CHILD_PID}" 2>/dev/null
    i=0
    while [ "${i}" -lt 2 ] && kill -0 "${CHILD_PID}" 2>/dev/null; do
        i=$((i + 1))
        sleep 1 & wait $!
    done
    kill -INT "${CHILD_PID}" 2>/dev/null
    while :; do
        wait "${CHILD_PID}" 2>/dev/null
        CHILD_STATUS=$?
        kill -0 "${CHILD_PID}" 2>/dev/null || break
    done
    CHILD_PID=""
}

write_state() {
    pid="${CHILD_PID:-0}"
    tor_on=false
    [ -n "${CHILD_PID}" ] && [ -n "${TOR_ARGS}" ] && tor_on=true
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
    CHILD_STATUS=0
    stop_child
    exit "${CHILD_STATUS:-0}"
}
trap shutdown INT TERM

while true; do
    NAME=$(read_selected)
    purge_deleted

    if [ -z "${NAME}" ]; then
        [ -n "${CHILD_PID}" ] && stop_child
        RUNNING_DIR="__none__"
        write_state "" ""
        sleep 3 & wait $!
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
        sleep 3 & wait $!
        continue
    fi

    TOR_REV=$(tor_field rev 0)
    # brclientd loads the dcrlnd macaroon once at startup, so follow the file:
    # dcrlnd rebakes it after a passphrase rotation and the old copy is then
    # rejected with a signature mismatch. Absent file (mid-rebake or fresh
    # setup) does not trigger; the restart fires once the new one lands.
    MAC_STAMP=$(stat -c %Y "${LNDIR}/admin.macaroon" 2>/dev/null || echo 0)
    MAC_CHANGED=""
    [ "${MAC_STAMP}" != "0" ] && [ "${MAC_STAMP}" != "${RUNNING_MAC_STAMP}" ] && MAC_CHANGED=1
    if [ "${APPDATA}" != "${RUNNING_DIR}" ] || [ "${TOR_REV}" != "${RUNNING_TOR_REV}" ] || [ -n "${MAC_CHANGED}" ] || [ -z "${CHILD_PID}" ]; then
        if [ -n "${CHILD_PID}" ]; then
            echo "Restarting brclientd for wallet '${NAME}' (tor rev ${TOR_REV})"
            stop_child
        fi
        echo "Starting brclientd for wallet '${NAME}' (appdata ${APPDATA})"
        launch "${APPDATA}" "${LNDIR}"
        RUNNING_DIR="${APPDATA}"
        RUNNING_TOR_REV="${TOR_REV}"
        RUNNING_MAC_STAMP="${MAC_STAMP}"
    fi

    write_state "${NAME}" "${APPDATA}"
    sleep 3 & wait $!
done
