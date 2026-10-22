#!/bin/bash
#
# helix-golden-startup.sh - Run the project startup script for a golden build.
#
# Usage: helix-golden-startup.sh <startup-script>
#
# A golden build finishes when the startup script exits. Scripts that end by
# starting a dev server in the foreground (npm run dev, docker compose up,
# tail -f) never exit, and the build would sit at "Building" until the API's
# deadline. Once the agent CPU tier (the startup script, the inner container
# engine, its containers and BuildKit steps; see 16-cpu-tiers.sh) has been idle
# for HELIX_GOLDEN_IDLE_TIMEOUT seconds, the build work is done: stop the
# script and report success.
#
# Exits with the script's exit code, or 0 if it was stopped for being idle.

STARTUP_SCRIPT="$1"
IDLE_TIMEOUT="${HELIX_GOLDEN_IDLE_TIMEOUT:-1200}"
POLL="${HELIX_GOLDEN_IDLE_POLL:-60}"
ACTIVITY_CGROUP="${HELIX_GOLDEN_ACTIVITY_CGROUP:-/sys/fs/cgroup/desktop/agent}"
# Below these per-poll rates the tier is idle: 5% of a core, 1 MiB/min written.
IDLE_CPU_USEC=$((POLL * 1000000 / 20))
IDLE_WRITE_BYTES=$((POLL * 1048576 / 60))

cpu_usec() {
    awk '$1 == "usage_usec" { print $2 }' "$ACTIVITY_CGROUP/cpu.stat"
}

written_bytes() {
    awk '{ for (i = 2; i <= NF; i++) if (sub(/^wbytes=/, "", $i)) total += $i } END { print total + 0 }' \
        "$ACTIVITY_CGROUP/io.stat" 2>/dev/null || echo 0
}

# Its own session, so servers and helpers it backgrounds stop with it
# (setsid execs in place, so PID is the session ID; -i enables job control,
# which gives background jobs their own process groups). stdin is
# /dev/null so a prompt can't block the build; -i keeps ~/.bashrc as in
# normal sessions.
setsid bash -i "$STARTUP_SCRIPT" </dev/null &
PID=$!

if [ ! -r "$ACTIVITY_CGROUP/cpu.stat" ]; then
    echo "⚠️ Golden build: $ACTIVITY_CGROUP/cpu.stat unavailable, waiting for the startup script to exit"
    wait "$PID"
    exit $?
fi

idle_for=0
last_cpu=$(cpu_usec)
last_written=$(written_bytes)
while true; do
    for _ in $(seq 1 "$POLL"); do
        if ! kill -0 "$PID" 2>/dev/null; then
            wait "$PID"
            exit $?
        fi
        sleep 1
    done

    cpu=$(cpu_usec)
    written=$(written_bytes)
    if [ $((cpu - last_cpu)) -le "$IDLE_CPU_USEC" ] && [ $((written - last_written)) -le "$IDLE_WRITE_BYTES" ]; then
        idle_for=$((idle_for + POLL))
    else
        idle_for=0
    fi
    last_cpu=$cpu
    last_written=$written

    if [ "$idle_for" -ge "$IDLE_TIMEOUT" ]; then
        echo ""
        echo "⚠️ Golden build: startup script still running after ${IDLE_TIMEOUT}s without build activity."
        echo "   Treating the build as complete and stopping it. Skip long-running servers"
        echo "   when HELIX_GOLDEN_BUILD=true so the build finishes as soon as it is done."
        pkill -TERM -s "$PID"
        for _ in $(seq 1 10); do
            pgrep -s "$PID" >/dev/null || break
            sleep 1
        done
        pkill -KILL -s "$PID"
        wait "$PID" 2>/dev/null
        exit 0
    fi
done
