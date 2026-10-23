#!/bin/bash
#
# helix-golden-startup.sh - Run the project startup script for a golden build.
#
# Usage: helix-golden-startup.sh <startup-script>
#
# A golden build finishes when the startup script exits. Scripts that end by
# starting a dev server in the foreground (npm run dev, docker compose up,
# tail -f) never exit, and the build would sit at "Building" until the API's
# deadline. Once build work has been idle for HELIX_GOLDEN_IDLE_TIMEOUT seconds,
# the build is done: stop the script and report success. Privileged desktops use
# the agent CPU tier (see 16-cpu-tiers.sh). Rootless desktops use the startup
# session, the engine process tree, and BuildKit history while excluding runtime
# container shims, so a running stack's servers don't keep the build alive.
#
# Exits with the script's exit code, or 0 if it was stopped for being idle.

STARTUP_SCRIPT="$1"
IDLE_TIMEOUT="${HELIX_GOLDEN_IDLE_TIMEOUT:-1200}"
POLL="${HELIX_GOLDEN_IDLE_POLL:-60}"
AGENT_CGROUP="${HELIX_GOLDEN_AGENT_CGROUP:-/sys/fs/cgroup/desktop/agent}"
ACTIVITY_CGROUPS=("$AGENT_CGROUP/procs" "$AGENT_CGROUP/docker/buildkit")
ROOTLESS_STATE_DIR="${XDG_RUNTIME_DIR:-/run/user/1000}/dockerd-rootless"
# Below these per-poll rates build work is idle: 20% of a core (dockerd serving
# a running stack sits around 10%), 10 MiB/min written.
IDLE_CPU_USEC=$((POLL * 1000000 / 5))
IDLE_WRITE_BYTES=$((POLL * 10 * 1048576 / 60))

# Sums a counter over the activity cgroups; BuildKit's appears on first build.
cgroup_total() {
    local file="$1" key="$2" cgroup
    for cgroup in "${ACTIVITY_CGROUPS[@]}"; do
        cat "$cgroup/$file" 2>/dev/null
    done | awk -v key="$key" '
        $1 == key { total += $2 }
        { for (i = 2; i <= NF; i++) if (sub("^" key "=", "", $i)) total += $i }
        END { print total + 0 }'
}

rootless_engine_pid() {
    if [ -r "$ROOTLESS_STATE_DIR/child_pid" ]; then
        cat "$ROOTLESS_STATE_DIR/child_pid"
    fi
}

# Rootless desktops have a private, read-only cgroup namespace. Use the startup
# session plus the rootless engine tree, excluding persistent container shims.
process_activity_pids() {
    local engine_pid
    engine_pid=$(rootless_engine_pid)
    ps -eo pid=,ppid=,sid=,comm= | awk -v startup_sid="$PID" -v engine_root="$engine_pid" '
        { parent[$1] = $2; session[$1] = $3; command[$1] = $4 }
        function engine_process(pid, root, current, steps) {
            current = pid
            for (steps = 0; steps < 1000; steps++) {
                if (command[current] ~ /^containerd-shim/) return 0
                if (current == root) return 1
                if (!(current in parent) || parent[current] == current || parent[current] == 0) return 0
                current = parent[current]
            }
            return 0
        }
        END {
            for (pid in parent) {
                if (session[pid] == startup_sid ||
                    (engine_root != "" && engine_process(pid, engine_root))) {
                    print pid
                }
            }
        }'
}

process_snapshot() {
    local pid stat rest ticks written value starttime
    local clock_ticks
    clock_ticks=$(getconf CLK_TCK)
    while read -r pid; do
        ticks=0
        written=0
        starttime=""
        if [ -r "/proc/$pid/stat" ]; then
            stat=$(cat "/proc/$pid/stat" 2>/dev/null) || stat=""
            if [ -n "$stat" ]; then
                rest="${stat##*) }"
                read -r -a fields <<< "$rest"
                ticks=$((${fields[11]:-0} + ${fields[12]:-0} + ${fields[13]:-0} + ${fields[14]:-0}))
                starttime="${fields[19]:-}"
            fi
        fi
        [ -n "$starttime" ] || continue
        if [ -r "/proc/$pid/io" ]; then
            value=$(awk '$1 == "write_bytes:" { print $2 }' "/proc/$pid/io" 2>/dev/null) || value=0
            written=${value:-0}
        fi
        echo "$pid:$starttime $((ticks * 1000000 / clock_ticks)) $written"
    done < <(process_activity_pids)
}

declare -A LAST_PROCESS_CPU=() LAST_PROCESS_WRITE=()

initialize_process_activity() {
    local key cpu written
    while read -r key cpu written; do
        LAST_PROCESS_CPU[$key]=$cpu
        LAST_PROCESS_WRITE[$key]=$written
    done < <(process_snapshot)
}

sample_process_activity() {
    local key cpu written cpu_delta=0 write_delta=0
    local -A next_cpu=() next_write=()
    while read -r key cpu written; do
        if [ -n "${LAST_PROCESS_CPU[$key]+x}" ]; then
            cpu_delta=$((cpu_delta + cpu - LAST_PROCESS_CPU[$key]))
            write_delta=$((write_delta + written - LAST_PROCESS_WRITE[$key]))
        else
            cpu_delta=$((cpu_delta + cpu))
            write_delta=$((write_delta + written))
        fi
        next_cpu[$key]=$cpu
        next_write[$key]=$written
    done < <(process_snapshot)
    LAST_PROCESS_CPU=()
    LAST_PROCESS_WRITE=()
    for key in "${!next_cpu[@]}"; do
        LAST_PROCESS_CPU[$key]=${next_cpu[$key]}
        LAST_PROCESS_WRITE[$key]=${next_write[$key]}
    done
    PROCESS_CPU_DELTA=$cpu_delta
    PROCESS_WRITE_DELTA=$write_delta
}

buildkit_running() {
    [ "$ACTIVITY_MODE" = process ] || return 1
    [ "${HELIX_ROOTLESS_CONTAINER_ENGINE:-0}" = 1 ] || return 1
    local running
    if ! running=$(BUILDX_BUILDER=default timeout 30 docker buildx history ls --filter status=running --format '{{.Ref}}' 2>/dev/null); then
        return 2
    fi
    [ -n "$running" ]
}

stop_startup() {
    pkill -TERM -s "$PID"
    for _ in $(seq 1 10); do
        pgrep -s "$PID" >/dev/null || break
        sleep 1
    done
    pkill -KILL -s "$PID"
    wait "$PID" 2>/dev/null
}

# Its own session, so servers and helpers it backgrounds stop with it
# (setsid execs in place, so PID is the session ID; -i enables job control,
# which gives background jobs their own process groups). stdin is
# /dev/null so a prompt can't block the build; -i keeps ~/.bashrc as in
# normal sessions.
setsid bash -i "$STARTUP_SCRIPT" </dev/null &
PID=$!

ACTIVITY_MODE=cgroup
if [ ! -r "$AGENT_CGROUP/procs/cpu.stat" ]; then
    ACTIVITY_MODE=process
    echo "Golden build: agent cgroup unavailable; monitoring startup and rootless engine processes"
fi

idle_for=0
if [ "$ACTIVITY_MODE" = cgroup ]; then
    last_cpu=$(cgroup_total cpu.stat usage_usec)
    last_written=$(cgroup_total io.stat wbytes)
else
    initialize_process_activity
fi
while true; do
    for _ in $(seq 1 "$POLL"); do
        if ! kill -0 "$PID" 2>/dev/null; then
            wait "$PID"
            exit $?
        fi
        sleep 1
    done

    if [ "$ACTIVITY_MODE" = cgroup ]; then
        cpu=$(cgroup_total cpu.stat usage_usec)
        written=$(cgroup_total io.stat wbytes)
        cpu_delta=$((cpu - last_cpu))
        write_delta=$((written - last_written))
        last_cpu=$cpu
        last_written=$written
    else
        sample_process_activity
        cpu_delta=$PROCESS_CPU_DELTA
        write_delta=$PROCESS_WRITE_DELTA
    fi
    buildkit_status=1
    buildkit_running
    buildkit_status=$?
    if [ "$buildkit_status" -eq 2 ]; then
        echo "Golden build: rootless BuildKit activity query failed or timed out" >&2
        stop_startup
        exit 1
    elif [ "$buildkit_status" -eq 0 ]; then
        idle_for=0
    elif [ "$cpu_delta" -le "$IDLE_CPU_USEC" ] && [ "$write_delta" -le "$IDLE_WRITE_BYTES" ]; then
        idle_for=$((idle_for + POLL))
    else
        idle_for=0
    fi

    if [ "$idle_for" -ge "$IDLE_TIMEOUT" ]; then
        echo ""
        echo "⚠️ Golden build: startup script still running after ${IDLE_TIMEOUT}s without build activity."
        echo "   Treating the build as complete and stopping it. Skip long-running servers"
        echo "   when HELIX_GOLDEN_BUILD=true so the build finishes as soon as it is done."
        stop_startup
        exit 0
    fi
done
