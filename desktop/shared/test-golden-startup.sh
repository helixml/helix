#!/bin/bash
#
# Test helix-golden-startup.sh with fast idle settings.
#
# Usage: ./test-golden-startup.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMP=$(mktemp -d)
# Background FIFO feeders would otherwise outlive a failed check, blocked on open.
trap 'jobs -p | xargs -r kill 2>/dev/null || true; rm -rf "$TMP"' EXIT
export HELIX_GOLDEN_IDLE_TIMEOUT=4 HELIX_GOLDEN_IDLE_POLL=2
# A fake agent CPU tier: tests simulate build activity by bumping its counters.
export HELIX_GOLDEN_AGENT_CGROUP="$TMP/cgroup"
PROCS="$HELIX_GOLDEN_AGENT_CGROUP/procs"
BUILDKIT="$HELIX_GOLDEN_AGENT_CGROUP/docker/buildkit"
mkdir -p "$PROCS" "$BUILDKIT"

check() {
    local name="$1" expected="$2" actual="$3"
    if [ "${actual}" != "${expected}" ]; then
        echo "FAIL: ${name}: expected '${expected}', got '${actual}'"
        exit 1
    fi
    echo "ok: ${name}"
}

reset_counters() {
    for cgroup in "$PROCS" "$BUILDKIT"; do
        rm -f "$cgroup/cpu.stat" "$cgroup/io.stat"
        echo "usage_usec 0" > "$cgroup/cpu.stat"
        echo "259:0 rbytes=0 wbytes=0" > "$cgroup/io.stat"
    done
}

run() {
    reset_counters
    launch "$1"
}

launch() {
    printf '%s\n' "$1" > "$TMP/startup.sh"
    local start=$SECONDS code=0
    timeout 60 "$SCRIPT_DIR/helix-golden-startup.sh" "$TMP/startup.sh" >"$TMP/out" 2>&1 || code=$?
    echo "$code $((SECONDS - start))"
}

# Runs a startup script that waits for the build activity fed by run_fed and
# reports whether the monitor let it finish.
run_fed() {
    local cpu_step="$1" write_step="$2" code
    reset_counters
    rm -f "$TMP/fed" "$BUILDKIT/cpu.stat" "$PROCS/io.stat"
    mkfifo "$BUILDKIT/cpu.stat" "$PROCS/io.stat"
    feed "$cpu_step" "$write_step" &
    local feeder=$!
    read -r code _ <<<"$(launch 'until [ -e '"$TMP"'/fed ]; do sleep 0.1; done; echo "startup finished"; exit 0')"
    kill "$feeder" 2>/dev/null || true
    wait "$feeder" 2>/dev/null || true
    reset_counters
    if grep -q "startup finished" "$TMP/out"; then echo "$code finished"; else echo "$code stopped"; fi
}

# Every monitor sample reads BuildKit's cpu.stat and then the agent's io.stat.
# Making both FIFOs hands each sample the next counter values, so activity
# grows per sample rather than per second and a stalled runner cannot make a
# busy build look idle. $TMP/fed appears once the monitor has taken more
# samples than it needs to stop an idle build.
feed() {
    local cpu_step="$1" write_step="$2" samples=0
    local enough=$((HELIX_GOLDEN_IDLE_TIMEOUT / HELIX_GOLDEN_IDLE_POLL + 2))
    while :; do
        echo "usage_usec $((samples * cpu_step))" > "$BUILDKIT/cpu.stat"
        echo "259:0 rbytes=0 wbytes=$((samples * write_step))" > "$PROCS/io.stat"
        samples=$((samples + 1))
        [ "$samples" -lt "$enough" ] || touch "$TMP/fed"
    done
}

run_rootless() {
    printf '%s\n' "$1" > "$TMP/startup.sh"
    local start=$SECONDS code=0
    HELIX_GOLDEN_AGENT_CGROUP="$TMP/missing-cgroup" \
        HELIX_ROOTLESS_CONTAINER_ENGINE=1 \
        XDG_RUNTIME_DIR="$TMP/runtime" \
        timeout 60 "$SCRIPT_DIR/helix-golden-startup.sh" "$TMP/startup.sh" >"$TMP/out" 2>&1 || code=$?
    echo "$code $((SECONDS - start))"
}

# Tests that aren't about the idle stop turn it off: the fake counters never
# move, so a runner slow to start bash -i would otherwise stop the script as
# idle before it reached its exit.
NO_IDLE_STOP=3600

read -r code _ <<<"$(HELIX_GOLDEN_IDLE_TIMEOUT=$NO_IDLE_STOP run 'echo built; exit 0')"
check "exiting script succeeds" 0 "$code"

read -r code _ <<<"$(HELIX_GOLDEN_IDLE_TIMEOUT=$NO_IDLE_STOP run 'echo broken; exit 3')"
check "failing script keeps its exit code" 3 "$code"

read -r code took <<<"$(run 'echo built; sleep 4242 & exec sleep 4242')"
check "foreground server is stopped once idle" 0 "$code"
[ "$took" -lt 30 ] || { echo "FAIL: idle stop took ${took}s"; exit 1; }
pgrep -f "^sleep 4242" >/dev/null && { echo "FAIL: server processes left running"; exit 1; }
echo "ok: server processes killed"

read -r code _ <<<"$(HELIX_GOLDEN_IDLE_TIMEOUT=$NO_IDLE_STOP run 'read -r line; echo "got: $line"; exit 0')"
check "prompt does not block" 0 "$code"

# BuildKit steps run outside the script's processes; they show up only in the
# tier's counters.
read -r code outcome <<<"$(run_fed 1000000 0)"
check "script waiting on a BuildKit build is not stopped" "0 finished" "$code $outcome"

read -r code outcome <<<"$(run_fed 0 100000000)"
check "script pulling images is not stopped" "0 finished" "$code $outcome"

mkdir -p "$TMP/bin" "$TMP/runtime/dockerd-rootless"
cat > "$TMP/bin/docker" <<'EOF'
#!/bin/sh
if [ -n "${HELIX_GOLDEN_TEST_BUILDX_ERROR:-}" ]; then
    exit 1
fi
if [ "$1 $2 $3" = "buildx history ls" ] && [ -f "${HELIX_GOLDEN_TEST_BUILDX_RUNNING:-}" ]; then
    echo default/default/test
fi
EOF
chmod +x "$TMP/bin/docker"
export PATH="$TMP/bin:$PATH"

read -r code _ <<<"$(HELIX_GOLDEN_IDLE_TIMEOUT=$NO_IDLE_STOP run_rootless 'echo broken; exit 3')"
check "rootless failing script keeps its exit code" 3 "$code"

read -r code _ <<<"$(HELIX_GOLDEN_IDLE_TIMEOUT=$NO_IDLE_STOP run_rootless 'read -r line; echo "got: $line"; exit 0')"
check "rootless prompt does not block" 0 "$code"

read -r code took <<<"$(run_rootless 'echo built; sleep 4343 & exec sleep 4343')"
check "rootless foreground server is stopped once idle" 0 "$code"
[ "$took" -lt 30 ] || { echo "FAIL: rootless idle stop took ${took}s"; exit 1; }
pgrep -f "^sleep 4343" >/dev/null && { echo "FAIL: rootless server processes left running"; exit 1; }
echo "ok: rootless server processes killed"

read -r code took <<<"$(run_rootless 'end=$((SECONDS + 10)); while [ "$SECONDS" -lt "$end" ]; do :; done; exit 0')"
check "rootless startup CPU activity is not stopped" 0 "$code"
[ "$took" -ge 10 ] || { echo "FAIL: rootless CPU work stopped after ${took}s"; exit 1; }
echo "ok: rootless startup CPU work ran to completion"

read -r code took <<<"$(run_rootless 'payload=$(printf "%65536s" x); end=$((SECONDS + 10)); while [ "$SECONDS" -lt "$end" ]; do : > '"$TMP"'/startup-io; printf "%s" "$payload" >> '"$TMP"'/startup-io; sleep 0.1; done; exit 0')"
check "rootless startup I/O activity is not stopped" 0 "$code"
[ "$took" -ge 10 ] || { echo "FAIL: rootless I/O work stopped after ${took}s"; exit 1; }
echo "ok: rootless startup I/O work ran to completion"

cp /bin/bash "$TMP/runc"
"$TMP/runc" -c 'payload=$(printf "%65536s" x); end=$((SECONDS + 10)); while [ "$SECONDS" -lt "$end" ]; do : > "$0"; printf "%s" "$payload" >> "$0"; sleep 0.1; done' "$TMP/engine-output" &
engine_pid=$!
echo "$engine_pid" > "$TMP/runtime/dockerd-rootless/child_pid"
read -r code took <<<"$(run_rootless 'exec sleep 4344')"
wait "$engine_pid"
rm -f "$TMP/runtime/dockerd-rootless/child_pid"
check "rootless engine activity is not stopped" 0 "$code"
[ "$took" -ge 10 ] || { echo "FAIL: rootless engine work stopped after ${took}s"; exit 1; }
echo "ok: rootless engine work ran to completion"

cp /bin/bash "$TMP/containerd-shim-runc-v2"
bash -c '"$1" -c '\''payload=$(printf "%65536s" x); while :; do : > "$0"; printf "%s" "$payload" >> "$0"; sleep 0.1; done'\'' "$3" & echo $! > "$2"; wait' _ \
    "$TMP/containerd-shim-runc-v2" "$TMP/shim-pid" "$TMP/runtime-output" &
engine_pid=$!
while [ ! -s "$TMP/shim-pid" ]; do sleep 0.1; done
shim_pid=$(cat "$TMP/shim-pid")
echo "$engine_pid" > "$TMP/runtime/dockerd-rootless/child_pid"
read -r code took <<<"$(run_rootless 'exec sleep 4345')"
kill "$shim_pid" 2>/dev/null || true
kill "$engine_pid" 2>/dev/null || true
wait "$shim_pid" 2>/dev/null || true
wait "$engine_pid" 2>/dev/null || true
rm -f "$TMP/runtime/dockerd-rootless/child_pid"
check "rootless runtime container does not keep build active" 0 "$code"
[ "$took" -lt 30 ] || { echo "FAIL: runtime container kept build active for ${took}s"; exit 1; }
echo "ok: rootless runtime container activity excluded"

export HELIX_GOLDEN_TEST_BUILDX_RUNNING="$TMP/buildx-running"
touch "$HELIX_GOLDEN_TEST_BUILDX_RUNNING"
(sleep 10; rm -f "$HELIX_GOLDEN_TEST_BUILDX_RUNNING") &
history_pid=$!
read -r code took <<<"$(run_rootless 'exec sleep 4346')"
wait "$history_pid"
check "rootless BuildKit activity is not stopped" 0 "$code"
[ "$took" -ge 10 ] || { echo "FAIL: rootless BuildKit work stopped after ${took}s"; exit 1; }
echo "ok: rootless BuildKit work ran to completion"

export HELIX_GOLDEN_TEST_BUILDX_ERROR=1
read -r code took <<<"$(run_rootless 'exec sleep 4347')"
unset HELIX_GOLDEN_TEST_BUILDX_ERROR
check "rootless BuildKit query failure fails the build" 1 "$code"
[ "$took" -lt 30 ] || { echo "FAIL: BuildKit query failure took ${took}s"; exit 1; }
pgrep -f "^sleep 4347" >/dev/null && { echo "FAIL: server left running after BuildKit query failure"; exit 1; }
echo "ok: rootless BuildKit query failure cleaned up"
