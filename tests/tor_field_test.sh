#!/bin/sh
# Checks tor_field, which every daemon wrapper uses to read the dashboard's Tor
# setting: the five copies stay identical, and a damaged setting reads as on.
set -u
cd "$(dirname "$0")/.."
WRAPPERS="dcrd/docker-entrypoint.sh dcrwallet/docker-entrypoint.sh dcrlnd/docker-entrypoint.sh dcrdex/supervisor.sh brclientd/supervisor.sh"
TMP=$(mktemp -d)
trap 'chmod -R u+rw "${TMP}"; rm -rf "${TMP}"' EXIT
fail=0

first=""
for w in ${WRAPPERS}; do
    sed -n '/^tor_field() {$/,/^}$/p' "${w}" > "${TMP}/fn"
    [ -s "${TMP}/fn" ] || { echo "FAIL: no tor_field in ${w}"; fail=1; continue; }
    if [ -z "${first}" ]; then
        first="${w}"
        cp "${TMP}/fn" "${TMP}/tor_field.sh"
    elif ! cmp -s "${TMP}/fn" "${TMP}/tor_field.sh"; then
        echo "FAIL: tor_field in ${w} differs from ${first}"; fail=1
    fi
done
. "${TMP}/tor_field.sh"

check() {
    got=$(tor_field "$2" "$3")
    [ "${got}" = "$4" ] || { echo "FAIL: $1: tor_field $2 $3 = '${got}', want '$4'"; fail=1; }
}
pointer() {
    TOR_POINTER="${TMP}/tor.json"
    rm -f "${TOR_POINTER}"
    [ "$1" = absent ] || printf '%s' "$1" > "${TOR_POINTER}"
}

pointer absent;                             check "absent" enabled false false
pointer '{"enabled":true,"rev":7}';         check "complete" enabled false true; check "complete" rev 0 7
pointer '{"enabled":false,"rev":8}';        check "off" enabled false false
pointer '';                                 check "empty" enabled false true
pointer '{"enabled":tr';                    check "torn" enabled false true
pointer '{"enabled":"yes"}';                check "unclear word" enabled false true
if [ "$(id -u)" != 0 ]; then
    pointer '{"enabled":false}'; chmod 000 "${TOR_POINTER}"
    check "unreadable" enabled false true
    chmod 600 "${TOR_POINTER}"
fi

[ "${fail}" = 0 ] && echo "tor_field: ok"
exit "${fail}"
