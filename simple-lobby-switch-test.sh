#!/bin/bash
# Simple lobby switching test using Wolf API directly
# Assumes you have an active Wolf-UI session from browser

set -e

WOLF_SOCKET="/var/run/wolf/wolf.sock"

echo "=== Simple Lobby Switch Test ==="
echo ""

# Check current Wolf state
echo "Current Wolf apps:"
docker exec helix-api-1 curl -s --unix-socket $WOLF_SOCKET http://localhost/api/v1/apps | jq -r '.apps[] | "\(.id) - \(.title)"'
echo ""

echo "Current Wolf sessions:"
SESSIONS=$(docker exec helix-api-1 curl -s --unix-socket $WOLF_SOCKET http://localhost/api/v1/sessions)
echo "$SESSIONS" | jq -r '.sessions[] | "\(.session_id) - app:\(.app_id) state:\(.state)"'
echo ""

# Count sessions
SESSION_COUNT=$(echo "$SESSIONS" | jq -r '.sessions | length')

if [ "$SESSION_COUNT" -eq 0 ]; then
    echo "❌ No active sessions found!"
    echo ""
    echo "To run this test:"
    echo "1. Open http://node01.lukemarsden.net:8081 in browser"
    echo "2. Click 'Wolf' host"
    echo "3. Launch 'Wolf UI' app"
    echo "4. Wait for Wolf-UI to load"
    echo "5. Inside Wolf-UI, create 2-3 external agent lobbies (Zed sessions)"
    echo "6. Come back and run this script again"
    echo ""
    echo "Or use the embedded client at:"
    echo "  http://localhost:8080/session/<your-external-agent-session-id>"
    exit 1
fi

echo "Current Wolf lobbies:"
LOBBIES=$(docker exec helix-api-1 curl -s --unix-socket $WOLF_SOCKET http://localhost/api/v1/lobbies)
echo "$LOBBIES" | jq '.'
echo ""

LOBBY_COUNT=$(echo "$LOBBIES" | jq -r '.lobbies | length')

if [ "$LOBBY_COUNT" -lt 2 ]; then
    echo "❌ Need at least 2 lobbies for switching (found $LOBBY_COUNT)"
    echo ""
    echo "In Wolf-UI, create more external agent lobbies, then run this script again."
    exit 1
fi

# Get lobbies as array
LOBBY_IDS=$(echo "$LOBBIES" | jq -r '.lobbies[] | .id')
LOBBY_ARRAY=($LOBBY_IDS)

# Get Wolf-UI session (the one that will switch between lobbies)
WOLF_UI_SESSION=$(echo "$SESSIONS" | jq -r '.sessions[] | select(.app_id == "134906179") | .session_id' | head -1)

if [ -z "$WOLF_UI_SESSION" ]; then
    echo "⚠️  No Wolf-UI session found, using first available session"
    WOLF_UI_SESSION=$(echo "$SESSIONS" | jq -r '.sessions[0].session_id')
fi

echo "Using session: $WOLF_UI_SESSION"
echo "Will switch between ${#LOBBY_ARRAY[@]} lobbies"
echo ""
echo "=== Starting Automated Lobby Switching ==="
echo ""

HANG_COUNT=0
SUCCESS_COUNT=0
MAX_ITER=50

for i in $(seq 1 $MAX_ITER); do
    LOBBY_IDX=$((i % ${#LOBBY_ARRAY[@]}))
    LOBBY_ID="${LOBBY_ARRAY[$LOBBY_IDX]}"

    echo "[$i/$MAX_ITER] Switching to lobby: $LOBBY_ID"

    # Join lobby
    JOIN_RESULT=$(docker exec helix-api-1 curl -s --unix-socket $WOLF_SOCKET \
        -X POST http://localhost/api/v1/lobbies/join \
        -H "Content-Type: application/json" \
        -d "{\"lobby_id\":\"$LOBBY_ID\",\"session_id\":\"$WOLF_UI_SESSION\"}" 2>&1)

    # Check for success
    if echo "$JOIN_RESULT" | grep -q "success.*true"; then
        echo "  ✓ Join successful"
    else
        echo "  ! Join result: $JOIN_RESULT"
    fi

    sleep 1

    # Check for HANG_DEBUG messages
    DEBUG_MSGS=$(docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml logs wolf --since 3s 2>&1 | grep "\[HANG_DEBUG\]" || echo "")

    if [ ! -z "$DEBUG_MSGS" ]; then
        echo "  [DEBUG]:"
        echo "$DEBUG_MSGS" | sed 's/^/    /'
    fi

    # Check for refcount errors
    ERRORS=$(docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml logs wolf --since 3s 2>&1 | grep -c "gst_mini_object_unref" || echo "0")

    if [ "$ERRORS" -gt 100 ]; then
        echo "  ❌ HANG DETECTED! (refcount errors: $ERRORS)"
        HANG_COUNT=$((HANG_COUNT + 1))

        # Save diagnostic logs
        TIMESTAMP=$(date +%s)
        LOGFILE="/tmp/wolf-hang-$TIMESTAMP.log"
        docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml logs wolf --since 30s > "$LOGFILE" 2>&1

        echo "  📋 Full logs: $LOGFILE"
        echo "  Last debug events:"
        cat "$LOGFILE" | grep "\[HANG_DEBUG\]" | tail -15 | sed 's/^/    /'

        echo ""
        echo "  🔍 ANALYSIS:"
        cat "$LOGFILE" | grep "\[HANG_DEBUG\]" | tail -20

        # Restart Wolf
        echo ""
        echo "  Restarting Wolf..."
        docker rm -f helix-wolf-1 >/dev/null 2>&1
        docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml up -d wolf >/dev/null 2>&1
        sleep 10
        echo "  Wolf restarted"
        echo ""
    else
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    fi

    sleep 0.5
done

echo ""
echo "=== Results ==="
echo "Total: $MAX_ITER iterations"
echo "Success: $SUCCESS_COUNT"
echo "Hangs: $HANG_COUNT"
echo "Rate: $(echo "scale=1; $SUCCESS_COUNT * 100 / $MAX_ITER" | bc)% success"

if [ "$HANG_COUNT" -gt 0 ]; then
    echo ""
    echo "Diagnostic logs: ls -lt /tmp/wolf-hang-*.log | head -$HANG_COUNT"
fi
