#!/bin/bash
# Stress test for Wolf-UI lobby joining to reproduce random video hang

WOLF_API="http://172.19.0.50/api/v1"
WOLF_UI_SESSION="9950542253436598280"  # Current Wolf-UI session ID
ZED_LOBBY="64a8e4b7-711c-42db-b2f1-d5eebc7fd635"  # Target Zed lobby to join
PIN="[4,8,7,9]"  # Lobby PIN as JSON array

echo "=== Wolf-UI Lobby Join Stress Test ==="
echo "Wolf-UI Session: $WOLF_UI_SESSION"
echo "Target Zed Lobby: $ZED_LOBBY"
echo ""

for i in {1..20}; do
    echo "Test $i: Joining lobby..."

    # Join lobby via Wolf API
    RESULT=$(docker exec helix-api-1 curl -s --unix-socket /var/run/wolf/wolf.sock \
        -X POST "$WOLF_API/lobbies/join" \
        -H "Content-Type: application/json" \
        -d "{\"lobby_id\":\"$ZED_LOBBY\",\"session_id\":\"$WOLF_UI_SESSION\",\"pin\":$PIN}" 2>&1)

    echo "  Result: $RESULT"

    # Wait a moment
    sleep 2

    # Leave lobby (switch back to Wolf-UI's own lobby)
    LEAVE=$(docker exec helix-api-1 curl -s --unix-socket /var/run/wolf/wolf.sock \
        -X POST "$WOLF_API/lobbies/leave" \
        -H "Content-Type: application/json" \
        -d "{\"session_id\":\"$WOLF_UI_SESSION\"}" 2>&1)

    echo "  Left: $LEAVE"

    sleep 1

    # Check for GStreamer errors in Wolf logs
    ERRORS=$(docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml logs wolf --since 5s 2>&1 | grep -c "gst_mini_object_unref")
    echo "  GStreamer refcount errors in last 5s: $ERRORS"

    if [ "$ERRORS" -gt 100 ]; then
        echo "  ⚠️  High error count detected!"
    fi

    echo ""
done

echo "=== Test Complete ==="
echo "Check Wolf logs for patterns:"
echo "  docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml logs wolf --since 5m | grep -E 'ERROR|hang|stuck|deadlock'"
