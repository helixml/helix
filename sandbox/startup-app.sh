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
# and stream video directly via WebSocket to the browser (no Wolf/Moonlight).

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

# Keep the container running
# All services (Hydra, heartbeat, revdial) run as background processes
# started by cont-init.d scripts. This just keeps the container alive.
exec tail -f /dev/null
