#!/bin/sh
# Copyright (c) 2015-2026 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

set -e

mkdir -p /app-data/dcrd

# dcrd writes its own RPC certificate on first start, valid ten years, as
# Decrediton lets it; these are the names the other containers dial.
export DCRD_ALT_DNSNAMES="${DCRD_ALT_DNSNAMES:-dcrd,dcrwallet}"

# dcrd is wallet-agnostic, so this supervisor watches only the shared Tor
# pointer the dashboard writes and relaunches dcrd when the toggle changes.
# The container stays up; only the inner dcrd process cycles. The pointer is
# parsed with sed (the image has no jq); proxy endpoint comes from env, the
# on/off decision from the pointer (absent pointer means Tor disabled).
set +e

TOR_POINTER="/app-data/control/tor.json"
STATE="/app-data/dcrd/control-state.json"
CHILD_PID=""
RUNNING_TOR_REV="__none__"

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
    [ "$(tor_field isolation true)" = "true" ] && TOR_ARGS="${TOR_ARGS} --torisolation"
    if [ "$(tor_field dcrdOnion false)" = "true" ] && [ -f /app-data/tor/dcrd-hs/hostname ]; then
        host=$(cat /app-data/tor/dcrd-hs/hostname 2>/dev/null)
        # --proxy alone makes dcrd set DisableListen, so also pass --listen to
        # actually accept inbound on the advertised onion. 0.0.0.0 so the tor
        # container can reach it over the Docker network (dcrd:9108).
        [ -n "${host}" ] && TOR_ARGS="${TOR_ARGS} --externalip=${host}:9108 --listen=0.0.0.0:9108"
    fi
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

start_dcrd() {
    build_tor_args
    if [ -n "${TOR_ARGS}" ]; then
        echo "Starting dcrd over Tor (${TOR_ARGS})"
    else
        echo "Starting dcrd on clearnet"
    fi
    # shellcheck disable=SC2086
    dcrd --appdata=/app-data/dcrd "$@" ${DCRD_EXTRA_ARGS} ${TOR_ARGS} &
    CHILD_PID=$!
}

write_state() {
    pid="${CHILD_PID:-0}"
    tor_on=false
    [ -n "${CHILD_PID}" ] && [ -n "${TOR_ARGS}" ] && tor_on=true
    cat > "${STATE}.tmp" <<EOF
{"pid":${pid:-0},"tor":${tor_on},"torRev":"${RUNNING_TOR_REV}"}
EOF
    mv "${STATE}.tmp" "${STATE}"
}

shutdown() {
    CHILD_STATUS=0
    stop_child
    exit "${CHILD_STATUS:-0}"
}
trap shutdown INT TERM

while true; do
    TOR_REV=$(tor_field rev 0)

    if [ -n "${CHILD_PID}" ] && ! kill -0 "${CHILD_PID}" 2>/dev/null; then
        wait "${CHILD_PID}" 2>/dev/null
        CHILD_PID=""
        RUNNING_TOR_REV="__none__"
    fi

    if [ "${TOR_REV}" != "${RUNNING_TOR_REV}" ] || [ -z "${CHILD_PID}" ]; then
        if [ -n "${CHILD_PID}" ]; then
            echo "Applying Tor change (rev ${TOR_REV})"
            stop_child
        fi
        start_dcrd "$@"
        RUNNING_TOR_REV="${TOR_REV}"
    fi

    write_state
    sleep 2 & wait $!
done
