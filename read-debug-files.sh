#!/bin/bash

CONTAINER=$(docker ps --format '{{.Names}}' | grep "zed-external" | head -1)

if [ -z "$CONTAINER" ]; then
    echo "No container running"
    exit 1
fi

echo "Container: $CONTAINER"
echo ""

for file in websocket_ping_result websocket_ping_success websocket_event_received websocket_sending websocket_send_success init_websocket_service_called zed_agent_panel_loaded; do
    echo "=== /tmp/$file.txt ==="
    docker exec "$CONTAINER" cat "/tmp/$file.txt" 2>/dev/null || echo "File not found"
    echo ""
done
