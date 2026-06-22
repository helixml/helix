#!/bin/bash
set -e

# Skip if no control plane configured (local dev mode)
# NOTE: Use "return" not "exit" - this script is sourced by entrypoint.sh!
if [ -z "$HELIX_API_URL" ] || [ -z "$RUNNER_TOKEN" ]; then
    echo "ℹ️  No HELIX_API_URL set, skipping sandbox heartbeat (local mode)"
    return 0
fi

SANDBOX_INSTANCE_ID=${SANDBOX_INSTANCE_ID:-local}
echo "💓 Starting sandbox heartbeat daemon for instance: $SANDBOX_INSTANCE_ID"

# Log discovered desktop versions
echo "📦 Discovering desktop versions..."
for f in /opt/images/helix-*.version; do
    if [ -f "$f" ]; then
        NAME=$(basename "$f" | sed 's/helix-//' | sed 's/.version//')
        VERSION=$(cat "$f")
        echo "   ${NAME}: ${VERSION}"
    fi
done

# Start the Go heartbeat daemon with auto-restart supervisor loop
# The daemon:
# - Dynamically discovers all desktop versions from /opt/images/helix-*.version
# - Monitors disk space on /var and /
# - Reports container disk usage
# - Sends heartbeat every 30 seconds
# The daemon can be safely killed and will automatically restart within 2 seconds
# tee to /var/log/helix-services/heartbeat.log so hydra's tailer surfaces
# heartbeat output in the admin Runner Logs WS stream (see
# 12-start-compose-manager.sh for the same pattern + rationale, plus
# the `|| true` mkdir guard, truncate-on-boot, and SIGPIPE trap).
mkdir -p /var/log/helix-services 2>/dev/null || true
: > /var/log/helix-services/heartbeat.log 2>/dev/null || true

(
    trap '' PIPE
    while true; do
        echo "[$(date -Iseconds)] Starting heartbeat daemon..."
        /usr/local/bin/sandbox-heartbeat
        EXIT_CODE=$?
        echo "[$(date -Iseconds)] ⚠️  Heartbeat daemon exited with code $EXIT_CODE, restarting in 2s..."
        sleep 2
    done
) 2>&1 | stdbuf -oL tee -a /var/log/helix-services/heartbeat.log | sed -u 's/^/[HEARTBEAT] /' &

HEARTBEAT_PID=$!
echo "✅ Sandbox heartbeat daemon started with auto-restart (wrapper PID: $HEARTBEAT_PID)"
