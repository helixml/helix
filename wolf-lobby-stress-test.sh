#!/bin/bash
# Automated Wolf lobby switching stress test
# Tests reliability of switching between lobbies to reproduce GStreamer hang bug

set -e

API_KEY="hl-80B8fQwxPScxobApjVvA-ag8N7_m6X48ss0qPu3Dvig="
HELIX_API="http://localhost:8080/api/v1"
WOLF_SOCKET="/var/run/wolf/wolf.sock"

echo "=== Wolf Lobby Switching Stress Test ==="
echo ""

# Create 2 external agent sessions (which create lobbies)
echo "Creating external agent sessions..."

SESSION1=$(curl -s -X POST "$HELIX_API/external-agents" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{}' | jq -r '.session_id')

sleep 2

SESSION2=$(curl -s -X POST "$HELIX_API/external-agents" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{}' | jq -r '.session_id')

sleep 5

echo "Session 1: $SESSION1"
echo "Session 2: $SESSION2"

# Get lobby IDs
echo ""
echo "Fetching lobby information..."

LOBBIES=$(docker exec helix-api-1 curl -s --unix-socket $WOLF_SOCKET http://localhost/api/v1/lobbies)
echo "Available lobbies: $LOBBIES"

# Get Wolf-UI session (we'll use this to switch between lobbies)
# For now, create one via moonlight-web by launching Wolf UI app

echo ""
echo "=== Test Instructions ==="
echo "1. Open http://node01.lukemarsden.net:8081"
echo "2. Click 'Wolf' host"
echo "3. Launch 'Wolf UI' app (this creates a Wolf-UI session)"
echo "4. Wait for Wolf-UI to load"
echo ""
echo "Then run this script again with the Wolf-UI session ID:"
echo "  export WOLF_UI_SESSION=<session_id>"
echo "  ./wolf-lobby-stress-test.sh <lobby1_id> <lobby2_id>"
echo ""
echo "Or I can automate the switching via Wolf API if you provide session ID..."
echo ""

# If session ID provided, run the switching test
if [ ! -z "$WOLF_UI_SESSION" ] && [ $# -eq 2 ]; then
    LOBBY1=$1
    LOBBY2=$2

    echo "Running automated lobby switching test..."
    echo "Wolf-UI Session: $WOLF_UI_SESSION"
    echo "Lobby 1: $LOBBY1"
    echo "Lobby 2: $LOBBY2"
    echo ""

    HANG_COUNT=0
    SUCCESS_COUNT=0

    for i in {1..50}; do
        echo "=== Test $i/50 ==="

        # Join Lobby 1
        echo "  Joining Lobby 1..."
        RESULT=$(docker exec helix-api-1 curl -s --unix-socket $WOLF_SOCKET \
            -X POST http://localhost/api/v1/lobbies/join \
            -H "Content-Type: application/json" \
            -d "{\"lobby_id\":\"$LOBBY1\",\"session_id\":\"$WOLF_UI_SESSION\"}" 2>&1)

        echo "  Result: $RESULT"
        sleep 2

        # Check for refcount errors
        ERRORS_BEFORE=$(docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml logs wolf --since 5s 2>&1 | grep -c "gst_mini_object_unref" || echo "0")

        # Join Lobby 2
        echo "  Switching to Lobby 2..."
        RESULT=$(docker exec helix-api-1 curl -s --unix-socket $WOLF_SOCKET \
            -X POST http://localhost/api/v1/lobbies/join \
            -H "Content-Type: application/json" \
            -d "{\"lobby_id\":\"$LOBBY2\",\"session_id\":\"$WOLF_UI_SESSION\"}" 2>&1)

        echo "  Result: $RESULT"
        sleep 2

        # Check for refcount errors after switch
        ERRORS_AFTER=$(docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml logs wolf --since 5s 2>&1 | grep -c "gst_mini_object_unref" || echo "0")

        if [ "$ERRORS_AFTER" -gt 100 ]; then
            echo "  ❌ HANG DETECTED! GStreamer refcount errors: $ERRORS_AFTER"
            HANG_COUNT=$((HANG_COUNT + 1))

            # Check if Wolf is zombie
            WOLF_STATE=$(docker inspect helix-wolf-1 2>/dev/null | jq -r '.[0].State.Status' || echo "unknown")
            echo "  Wolf state: $WOLF_STATE"

            # Restart Wolf
            echo "  Restarting Wolf..."
            docker rm -f helix-wolf-1 >/dev/null 2>&1
            docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml up -d wolf >/dev/null 2>&1
            sleep 10

            echo "  Hang #$HANG_COUNT detected at iteration $i"
        else
            echo "  ✅ Switch successful (errors: $ERRORS_BEFORE → $ERRORS_AFTER)"
            SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
        fi

        echo ""
    done

    echo "=== Test Results ==="
    echo "Successful switches: $SUCCESS_COUNT"
    echo "Hangs detected: $HANG_COUNT"
    echo "Success rate: $(echo "scale=2; $SUCCESS_COUNT * 100 / 50" | bc)%"
fi
