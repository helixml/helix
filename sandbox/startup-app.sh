#!/bin/bash
set -e

# This script is executed by GOW's /entrypoint.sh after all cont-init.d scripts
# At this point:
#   - dockerd is running (started by 40-start-dockerd.sh)
#   - RevDial clients started (50-start-revdial-clients.sh)
#   - Sandbox heartbeat started (55-start-sandbox-heartbeat.sh)
#   - Telemetry firewall configured (60-setup-telemetry-firewall.sh)
#   - Hydra multi-Docker daemon started (70-start-hydra.sh)
#
# Desktop containers (helix-sway, helix-ubuntu) are started on-demand by Hydra
# and stream video directly via WebSocket to the browser.

echo "=========================================="
echo "  Helix Sandbox Ready"
echo "=========================================="
echo ""
echo "Services running:"
echo "  - Docker daemon (nested containers)"
echo "  - Sandbox heartbeat (disk monitoring)"
if [ "$HYDRA_ENABLED" = "true" ]; then
    echo "  - Hydra daemon (multi-container isolation)"
fi
if [ -n "$HELIX_API_URL" ]; then
    echo "  - RevDial clients (API tunnel)"
fi
echo ""
echo "Desktop images available:"
for f in /opt/images/helix-*.version; do
    if [ -f "$f" ]; then
        NAME=$(basename "$f" .version)
        VERSION=$(cat "$f")
        echo "  - ${NAME}: ${VERSION}"
    fi
done
echo ""
echo "Waiting for desktop session requests via Hydra API..."
echo "=========================================="

# ==============================================================================
# Graceful shutdown of nested customer container stacks.
#
# This script is PID 1. On `docker stop helix-sandbox-app` (host reboot,
# SANDBOX_TAG bump via scripts/deploy-prod.sh) Docker sends SIGTERM to PID 1 and
# NOTHING ELSE. It used to be `exec tail -f /dev/null`, which dies instantly —
# so dockerd went down with it and every nested container, including hosted web
# services' own Postgres, was SIGKILLed mid-write. That is what corrupted
# we-find.ai's database on 2026-07-23:
#
#   PANIC:  could not locate a valid checkpoint record
#
# So: run `tail` in the BACKGROUND and `wait` on it, which lets a trap fire
# (a trap cannot run while the shell is blocked in `exec`). The trap asks hydra
# to stop the nested stacks cleanly before we let the container die.
#
# The outer compose service needs a stop_grace_period longer than
# HELIX_DRAIN_GRACE, or Docker SIGKILLs us mid-drain and we are back where we
# started. See docker-compose.yaml.
# ==============================================================================
HELIX_DRAIN_GRACE="${HELIX_DRAIN_GRACE:-60}"

drain_and_exit() {
    echo "🛑 SIGTERM received — draining nested container stacks (grace: ${HELIX_DRAIN_GRACE}s)..."
    if [ -S /var/run/hydra/hydra.sock ]; then
        # Synchronous: hydra returns only once the nested containers have
        # stopped. --max-time bounds it so a wedged container cannot hold the
        # host up past the compose stop_grace_period.
        curl -s --max-time "$((HELIX_DRAIN_GRACE + 30))" \
            --unix-socket /var/run/hydra/hydra.sock \
            -X POST "http://localhost/api/v1/drain?grace=${HELIX_DRAIN_GRACE}" \
            && echo "✅ Nested container stacks drained" \
            || echo "⚠️  Drain failed or timed out — nested data may not have flushed"
    else
        echo "⚠️  Hydra socket not present, skipping drain"
    fi
    exit 0
}
trap drain_and_exit TERM INT

# Keep the container running
# All services (Hydra, heartbeat, revdial) run as background processes
# started by cont-init.d scripts. This just keeps the container alive.
# NOTE: backgrounded + `wait` (not `exec`) so the TERM trap above can run.
tail -f /dev/null &
wait $!
