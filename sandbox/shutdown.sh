#!/bin/bash
# Graceful sandbox shutdown, run by the entrypoint (PID 1) on SIGTERM.
#
# Without this, `docker stop` on the sandbox was a no-op for its grace period:
# PID 1 was `tail -f /dev/null`, and the kernel drops signals PID 1 has no
# handler for. Docker then SIGKILLed the whole tree at once — nested dockerd,
# every desktop with its own dockerd/BuildKit, and the XFS-on-zvol and loop
# mounts under them — and the kernel spent minutes in uninterruptible teardown
# flushing those filesystems. On Meta (~10 desktops) the container took ~10 min
# to exit and `docker compose up` gave up with "did not receive an exit event".
#
# Tear down top-down instead, while every process is still alive to flush its
# own state: stop Hydra so nothing new starts, quiesce each desktop's inner
# dockerd, stop the desktops, stop the sandbox dockerd, then sync and unmount
# the block-device mounts Hydra created. Every step is bounded so the whole
# thing fits inside the compose stop_grace_period.

set -u

STOP_FILE=/run/helix-sandbox-stopping
DESKTOP_DOCKERD_TIMEOUT=${HELIX_SHUTDOWN_DESKTOP_DOCKERD_TIMEOUT:-60}
DESKTOP_STOP_TIMEOUT=${HELIX_SHUTDOWN_DESKTOP_STOP_TIMEOUT:-10}
DOCKERD_TIMEOUT=${HELIX_SHUTDOWN_DOCKERD_TIMEOUT:-60}
UNMOUNT_TIMEOUT=${HELIX_SHUTDOWN_UNMOUNT_TIMEOUT:-30}

log() {
    echo "[$(date -Iseconds)] [SHUTDOWN] $*"
}

# PIDs of processes named $1 in the sandbox's own PID namespace. Desktops run
# their own dockerd, so a bare pkill would also hit nested daemons.
own_pids() {
    local pid self_ns
    self_ns=$(readlink /proc/1/ns/pid)
    for pid in $(pgrep -x "$1"); do
        [ "$(readlink "/proc/$pid/ns/pid" 2>/dev/null)" = "$self_ns" ] && echo "$pid"
    done
}

# wait_gone <timeout-seconds> <pid>... returns 1 if any pid is still alive.
wait_gone() {
    local deadline=$((SECONDS + $1)) pid alive
    shift
    while :; do
        alive=""
        for pid in "$@"; do
            kill -0 "$pid" 2>/dev/null && alive="$alive $pid"
        done
        [ -z "$alive" ] && return 0
        [ "$SECONDS" -ge "$deadline" ] && return 1
        sleep 1
    done
}

start=$SECONDS
touch "$STOP_FILE"
log "Graceful sandbox shutdown started"

# 1. Hydra: stop accepting lifecycle requests. Its restart loop honours $STOP_FILE.
hydra_pids=$(own_pids hydra)
if [ -n "$hydra_pids" ]; then
    log "Stopping Hydra (pid(s): $(echo "$hydra_pids" | tr '\n' ' '))"
    # shellcheck disable=SC2086
    kill -TERM $hydra_pids 2>/dev/null || true
    # shellcheck disable=SC2086
    wait_gone 30 $hydra_pids || log "Hydra still running after 30s; continuing"
fi

if docker info >/dev/null 2>&1; then
    mapfile -t containers < <(docker ps -q)
    if [ "${#containers[@]}" -gt 0 ]; then
        # 2. Quiesce each desktop's inner dockerd first, using the same
        # /tmp/.dockerd-stop protocol golden builds use, so nested containers,
        # BuildKit and the zvol-backed /var/lib/docker are flushed while the
        # desktop is still running. Desktop PID 1 does not handle SIGTERM
        # either, so `docker stop` alone would SIGKILL straight through it.
        log "Stopping inner dockerd in ${#containers[@]} container(s) (up to ${DESKTOP_DOCKERD_TIMEOUT}s)"
        for c in "${containers[@]}"; do
            # shellcheck disable=SC2016 # expanded by the container's shell
            timeout "$((DESKTOP_DOCKERD_TIMEOUT + 5))" docker exec -u root "$c" sh -c '
                [ -f /var/run/docker.pid ] || exit 0
                touch /tmp/.dockerd-stop
                pid=$(cat /var/run/docker.pid) || exit 0
                kill -TERM "$pid" 2>/dev/null || exit 0
                for _ in $(seq 1 "$1"); do
                    kill -0 "$pid" 2>/dev/null || exit 0
                    sleep 1
                done
                exit 1' sh "$DESKTOP_DOCKERD_TIMEOUT" >/dev/null 2>&1 &
        done
        wait

        # 3. Stop the desktops themselves (docker stop runs them in parallel).
        # Their state is already flushed above, and their PID 1 ignores
        # SIGTERM, so a short grace is enough.
        log "Stopping ${#containers[@]} container(s) (grace ${DESKTOP_STOP_TIMEOUT}s)"
        timeout "$((DESKTOP_STOP_TIMEOUT + 30))" docker stop -t "$DESKTOP_STOP_TIMEOUT" "${containers[@]}" >/dev/null 2>&1 ||
            log "Some containers did not stop cleanly"
    fi
fi

# 4. The sandbox dockerd. Its restart loop honours $STOP_FILE.
dockerd_pids=$(own_pids dockerd)
if [ -n "$dockerd_pids" ]; then
    log "Stopping sandbox dockerd (up to ${DOCKERD_TIMEOUT}s)"
    # shellcheck disable=SC2086
    kill -TERM $dockerd_pids 2>/dev/null || true
    # shellcheck disable=SC2086
    wait_gone "$DOCKERD_TIMEOUT" $dockerd_pids || log "dockerd still running after ${DOCKERD_TIMEOUT}s; continuing"
fi

# 5. Flush and unmount the block-device mounts Hydra created (session/golden
# zvol clones, instance-disk loop images), deepest first, so the kernel is not
# left to do it during namespace teardown.
log "Syncing filesystems"
timeout "$UNMOUNT_TIMEOUT" sync || log "sync did not finish within ${UNMOUNT_TIMEOUT}s"
awk '$0 ~ / - [^ ]+ \/dev\/(zd|loop)/ { print $5 }' /proc/self/mountinfo | sort -r | while read -r mnt; do
    mnt=$(printf '%b' "$mnt")
    timeout "$UNMOUNT_TIMEOUT" umount "$mnt" 2>/dev/null || log "Could not unmount $mnt"
done

log "Graceful sandbox shutdown finished in $((SECONDS - start))s"
