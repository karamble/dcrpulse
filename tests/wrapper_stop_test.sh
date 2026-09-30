#!/bin/sh
# Checks how every daemon wrapper stops its daemon: the five copies stay
# identical, a stop signal is handled at once, the daemon gets two interrupts
# and all the time it needs, and the wrapper exits with the daemon's status.
set -u
cd "$(dirname "$0")/.."
WRAPPERS="dcrd/docker-entrypoint.sh dcrwallet/docker-entrypoint.sh dcrlnd/docker-entrypoint.sh dcrdex/supervisor.sh brclientd/supervisor.sh"
TMP=$(mktemp -d)
trap 'rm -rf "${TMP}"' EXIT
fail=0

first=""
for w in ${WRAPPERS}; do
    sed -n '/^stop_child() {$/,/^}$/p; /^shutdown() {$/,/^}$/p' "${w}" > "${TMP}/fn"
    grep -q '^shutdown() {$' "${TMP}/fn" && grep -q '^stop_child() {$' "${TMP}/fn" ||
        { echo "FAIL: no stop_child or shutdown in ${w}"; fail=1; continue; }
    if [ -z "${first}" ]; then
        first="${w}"
        cp "${TMP}/fn" "${TMP}/stop.sh"
    elif ! cmp -s "${TMP}/fn" "${TMP}/stop.sh"; then
        echo "FAIL: stop_child or shutdown in ${w} differs from ${first}"; fail=1
    fi
    # A foreground sleep would hold the trap until it ends.
    if sed -n '/^trap shutdown INT TERM$/,$p' "${w}" | grep -Eq '^[[:space:]]+sleep [0-9]+$'; then
        echo "FAIL: ${w} sleeps in the foreground after its trap"; fail=1
    fi
    grep -q 'kill -KILL' "${w}" && { echo "FAIL: ${w} still kills its daemon"; fail=1; }
done

# The daemon logs each interrupt; on the second it takes three seconds to shut
# down, longer than any wrapper-side limit would allow, then exits with $2.
cat > "${TMP}/daemon.py" <<'EOF'
import os, signal, sys, time
log = open(sys.argv[1], "a", buffering=1)
open(sys.argv[1] + ".pid", "w").write(str(os.getpid()))
seen = []
def interrupt(*_):
    seen.append(time.monotonic())
    log.write("INT\n")
    if len(seen) == 2:
        log.write("gap %.1f\n" % (seen[1] - seen[0]))
        time.sleep(3)
        log.write("done\n")
        sys.exit(int(sys.argv[2]))
signal.signal(signal.SIGINT, interrupt)
while True:
    time.sleep(0.1)
EOF

run() {
    status=$1
    log="${TMP}/log.${status}"
    : > "${log}"
    { echo 'CHILD_PID=""'; cat "${TMP}/stop.sh"; cat <<EOF
trap shutdown INT TERM
python3 "${TMP}/daemon.py" "${log}" ${status} &
CHILD_PID=\$!
while true; do
    sleep 30 & wait \$!
    echo relaunch >> "${log}"
done
EOF
    } > "${TMP}/wrapper.sh"
    sh "${TMP}/wrapper.sh" &
    wpid=$!
    sleep 1
    start=$(date +%s)
    kill -TERM "${wpid}"
    # A wrapper that never lets its daemon finish fails here instead of hanging.
    ( sleep 20; kill -KILL "${wpid}" "$(cat "${log}.pid")" 2>/dev/null ) >/dev/null 2>&1 &
    guard=$!
    wait "${wpid}"
    got=$?
    kill "${guard}" 2>/dev/null
    took=$(( $(date +%s) - start ))

    [ "${got}" = "${status}" ] || { echo "FAIL: wrapper exited ${got}, want the daemon's ${status}"; fail=1; }
    [ "$(grep -c '^INT$' "${log}")" = 2 ] || { echo "FAIL: daemon saw $(grep -c '^INT$' "${log}") interrupts, want 2"; fail=1; }
    grep -Eq '^gap (1\.[5-9]|2\.[0-9])$' "${log}" || { echo "FAIL: interrupts not two seconds apart: $(grep gap "${log}")"; fail=1; }
    grep -q '^done$' "${log}" || { echo "FAIL: daemon was killed before it finished"; fail=1; }
    grep -q '^relaunch$' "${log}" && { echo "FAIL: wrapper went on after the stop signal"; fail=1; }
    [ "${took}" -lt 15 ] || { echo "FAIL: stop took ${took}s, the trap waited for the sleep"; fail=1; }
}
run 0
run 7

[ "${fail}" = 0 ] && echo "stop_child: ok"
exit "${fail}"
