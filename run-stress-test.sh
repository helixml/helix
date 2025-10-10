#!/bin/bash
# Run stress test with current Wolf session

set -e

WOLF_UI_SESSION=$(docker exec helix-api-1 curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/sessions | jq -r '.sessions[] | select(.app_id == "134906179") | .client_id')

echo "Wolf-UI Session (client_id): $WOLF_UI_SESSION"

if [ -z "$WOLF_UI_SESSION" ] || [ "$WOLF_UI_SESSION" == "null" ]; then
    echo "❌ No Wolf-UI session found!"
    exit 1
fi

LOBBIES=(
  "52dcbf1a-771e-49e1-82ee-049239a6e70b"
  "64b64b46-e48e-451e-86ac-2fd804c75329"
  "4e2181fb-9295-451c-90f3-53c5ec43d056"
)

echo "Lobbies to switch between:"
printf '  %s\n' "${LOBBIES[@]}"
echo ""
echo "Starting automated lobby switching test (50 iterations)..."
echo ""

HANG_COUNT=0
SUCCESS_COUNT=0

for i in {1..50}; do
    LOBBY_IDX=$((i % 3))
    LOBBY_ID="${LOBBIES[$LOBBY_IDX]}"

    echo "[$i/50] Switching to lobby: ${LOBBY_ID:0:8}..."

    # Join lobby
    JOIN_RESULT=$(docker exec helix-api-1 curl -s --unix-socket /var/run/wolf/wolf.sock \
        -X POST http://localhost/api/v1/lobbies/join \
        -H "Content-Type: application/json" \
        -d "{\"lobby_id\":\"$LOBBY_ID\",\"session_id\":\"$WOLF_UI_SESSION\"}" 2>&1)

    if echo "$JOIN_RESULT" | grep -q "success.*true"; then
        echo "  ✓ Join successful"
    else
        echo "  ! Result: $JOIN_RESULT"
    fi

    sleep 1

    # Check for HANG_DEBUG messages
    DEBUG_MSGS=$(docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml logs wolf --since 3s 2>&1 | grep "\[HANG_DEBUG\]" || true)

    if [ ! -z "$DEBUG_MSGS" ]; then
        echo "  [DEBUG]:"
        echo "$DEBUG_MSGS" | sed 's/^/    /' | head -5
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

        echo ""
        echo "  🔍 HANG ANALYSIS #$HANG_COUNT:"
        cat "$LOGFILE" | grep "\[HANG_DEBUG\]" | tail -20
        echo ""
        echo "  📋 Full logs: $LOGFILE"

        # Restart Wolf
        echo "  Restarting Wolf..."
        docker rm -f helix-wolf-1 >/dev/null 2>&1
        docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml up -d wolf >/dev/null 2>&1
        sleep 10

        # Need to get new session ID after restart
        WOLF_UI_SESSION=$(docker exec helix-api-1 curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/sessions | jq -r '.sessions[] | select(.app_id == "134906179") | .client_id')
        echo "  Wolf restarted (new client_id: ${WOLF_UI_SESSION:0:12}...)"
        echo ""
    else
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    fi

    sleep 0.5
done

echo ""
echo "=== RESULTS ==="
echo "Total: 50 iterations"
echo "Success: $SUCCESS_COUNT"
echo "Hangs: $HANG_COUNT"
echo "Rate: $(echo "scale=1; $SUCCESS_COUNT * 100 / 50" | bc)% success"

if [ "$HANG_COUNT" -gt 0 ]; then
    echo ""
    echo "Diagnostic logs saved:"
    ls -lt /tmp/wolf-hang-*.log | head -$HANG_COUNT
fi
