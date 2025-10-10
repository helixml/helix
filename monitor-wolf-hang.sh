#!/bin/bash
# Real-time monitor for Wolf GStreamer hang bug
# Run this while manually testing lobby switching

BASELINE_ERRORS=$(docker compose -f docker-compose.dev.yaml logs wolf --since 1m 2>&1 | grep -c "gst_mini_object_unref" || echo "0")

echo "=== Wolf Hang Monitor ==="
echo "Baseline refcount errors (last 1min): $BASELINE_ERRORS"
echo ""
echo "Monitoring for hang condition..."
echo "Press Ctrl+C to stop"
echo ""
echo "Test procedure:"
echo "1. Open http://node01.lukemarsden.net:8081"
echo "2. Launch Wolf UI app"
echo "3. Inside Wolf-UI, join different lobbies multiple times"
echo "4. This script will detect when hang occurs"
echo ""
echo "=========================================="
echo ""

HANG_DETECTED=false
CHECK_COUNT=0

while true; do
    sleep 3
    CHECK_COUNT=$((CHECK_COUNT + 1))

    # Check refcount errors in last 5 seconds
    ERRORS=$(docker compose -f docker-compose.dev.yaml logs wolf --since 5s 2>&1 | grep -c "gst_mini_object_unref" || echo "0")

    # Check Wolf process state
    WOLF_STATE=$(docker inspect helix-wolf-1 2>/dev/null | jq -r '.[0].State.Status' 2>/dev/null || echo "missing")

    # Calculate errors per second
    ERRORS_PER_SEC=$((ERRORS / 5))

    if [ "$ERRORS" -gt 50 ]; then
        if [ "$HANG_DETECTED" = "false" ]; then
            echo ""
            echo "🚨 HANG DETECTED at check #$CHECK_COUNT!"
            echo "   GStreamer errors: $ERRORS in last 5s ($ERRORS_PER_SEC/sec)"
            echo "   Wolf state: $WOLF_STATE"
            echo ""
            echo "   Capturing diagnostic information..."

            # Save Wolf logs
            TIMESTAMP=$(date +%s)
            docker compose -f docker-compose.dev.yaml logs wolf --since 30s > "/tmp/wolf-hang-$TIMESTAMP.log" 2>&1
            echo "   Logs saved to: /tmp/wolf-hang-$TIMESTAMP.log"

            # Get active sessions
            docker exec helix-api-1 curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/sessions 2>&1 > "/tmp/wolf-sessions-$TIMESTAMP.json"
            echo "   Sessions saved to: /tmp/wolf-sessions-$TIMESTAMP.json"

            # Get lobbies
            docker exec helix-api-1 curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/lobbies 2>&1 > "/tmp/wolf-lobbies-$TIMESTAMP.json"
            echo "   Lobbies saved to: /tmp/wolf-lobbies-$TIMESTAMP.json"

            echo ""
            echo "   Analyzing last 50 log lines before hang..."
            docker compose -f docker-compose.dev.yaml logs wolf --tail 50 2>&1 | strings | grep -v "gst_mini_object_unref" | tail -20

            HANG_DETECTED=true

            echo ""
            echo "🔧 Auto-recovery in 5 seconds..."
            sleep 5

            echo "   Killing Wolf container..."
            docker rm -f helix-wolf-1 >/dev/null 2>&1

            echo "   Restarting Wolf..."
            docker compose -f docker-compose.dev.yaml up -d wolf >/dev/null 2>&1

            echo "   ✅ Wolf restarted. Continue testing."
            echo ""

            HANG_DETECTED=false
        fi
    else
        # Normal operation
        if [ $((CHECK_COUNT % 10)) -eq 0 ]; then
            echo "[Check #$CHECK_COUNT] OK - Errors: $ERRORS (${ERRORS_PER_SEC}/sec), Wolf: $WOLF_STATE"
        fi
    fi
done
