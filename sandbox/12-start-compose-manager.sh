#!/bin/bash
set -e

# Sandbox-absorbs-runner pivot: start the compose-manager daemon if the
# sandbox is registered with a control plane. Skip in local dev mode where
# there's no API to fetch assignments from.
#
# NOTE: Use "return" not "exit" — this script is sourced by entrypoint.sh.

if [ -z "$HELIX_API_URL" ] || [ -z "$RUNNER_TOKEN" ]; then
    echo "ℹ️  No HELIX_API_URL set, skipping compose-manager (local mode)"
    return 0
fi

SANDBOX_INSTANCE_ID=${SANDBOX_INSTANCE_ID:-local}
echo "🧱 Starting compose-manager for sandbox: $SANDBOX_INSTANCE_ID"

# Ensure the per-service log dir exists. Hydra's tailServiceLog reads
# /var/log/helix-services/*.log and pushes each line into the LogBuffer
# the admin Runner Logs WS streams from, so we tee into the file here
# AND keep the existing sed-prefixed stdout for `docker logs` viewers
# on docker-compose-style deployments.
#
# `|| true` guards: this script is `source`d by entrypoint.sh under
# `set -e`, so a hostile filesystem (readonly /var, mount race) would
# otherwise abort container init. The tee inside the pipeline below
# will surface the same error if it can't write, but failures there
# don't kill the supervisor (see SIGPIPE trap rationale below).
mkdir -p /var/log/helix-services 2>/dev/null || true
# Truncate-on-boot so the file always starts empty for this container's
# lifetime. Hydra's tailer reads from the START of the file, so this
# both (a) avoids replaying a previous container's logs (the file lives
# on the container layer; not persisted across recreates anyway, but
# defensive) and (b) ensures dockerd/heartbeat startup output - which
# is written BEFORE hydra boots in 10-start-hydra.sh - is captured
# from t=0 once hydra's tailer attaches a moment later.
: > /var/log/helix-services/compose-manager.log 2>/dev/null || true

# The compose-manager polls /api/v1/runners/{id}/assignment and applies
# the assigned profile by running `docker compose` against the inner
# dockerd. Auto-restart on crash.
(
    # Ignore SIGPIPE inside the supervisor: if `tee` or `sed` downstream
    # dies (file unwritable, FD exhausted, mount drops mid-run), the
    # next write inside this loop would otherwise SIGPIPE and kill the
    # supervisor, leaving compose-manager permanently dead. Trapping
    # PIPE means individual writes silently fail but the loop keeps
    # restarting compose-manager forever.
    trap '' PIPE
    # `stdbuf -oL tee` below: tee block-buffers its write to the file
    # when its stdout is a pipe (not a TTY). For low-volume producers
    # like compose-manager (a few hundred bytes/min when idle), the
    # buffer holds output for minutes before flushing - making hydra's
    # admin Runner Logs WS appear silent even though [COMPOSE-MGR]
    # lines are visible on `docker logs`. `-oL` forces line buffering
    # on tee's stdout writes so each line hits the file (and so the
    # tailer) immediately. Same fix in 04/08/14 scripts.
    while true; do
        echo "[$(date -Iseconds)] Starting compose-manager..."
        HELIX_RUNNER_ID="$SANDBOX_INSTANCE_ID" \
        HELIX_RUNNER_TOKEN="$RUNNER_TOKEN" \
        /usr/local/bin/compose-manager \
            --api-url "$HELIX_API_URL" \
            --runner-id "$SANDBOX_INSTANCE_ID" \
            --runner-token "$RUNNER_TOKEN"
        EXIT_CODE=$?
        echo "[$(date -Iseconds)] ⚠️  compose-manager exited with $EXIT_CODE, restarting in 2s..."
        sleep 2
    done
) 2>&1 | stdbuf -oL tee -a /var/log/helix-services/compose-manager.log | sed -u 's/^/[COMPOSE-MGR] /' &

CM_PID=$!
echo "✅ compose-manager started with auto-restart (wrapper PID: $CM_PID)"
