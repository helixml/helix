#!/bin/bash
# Fully automated Wolf hang testing
# Creates sessions, switches lobbies, monitors for hang

set -e

API_KEY="hl-80B8fQwxPScxobApjVvA-ag8N7_m6X48ss0qPu3Dvig="
HELIX_API="http://localhost:8080/api/v1"
WOLF_SOCKET="/var/run/wolf/wolf.sock"

echo "=== Automated Wolf Hang Test ==="
echo ""

# Step 1: Create external agent sessions (these become lobbies)
echo "Creating 3 external agent sessions (lobbies)..."

SESSION1=$(curl -s -X POST "$HELIX_API/external-agents" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{}' | jq -r '.session_id')

sleep 3

SESSION2=$(curl -s -X POST "$HELIX_API/external-agents" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{}' | jq -r '.session_id')

sleep 3

SESSION3=$(curl -s -X POST "$HELIX_API/external-agents" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{}' | jq -r '.session_id')

sleep 5

echo "Created external agent sessions:"
echo "  Session 1: $SESSION1"
echo "  Session 2: $SESSION2"
echo "  Session 3: $SESSION3"
echo ""

# Step 2: Get lobby IDs from Wolf
echo "Fetching lobby information from Wolf..."
LOBBIES=$(docker exec helix-api-1 curl -s --unix-socket $WOLF_SOCKET http://localhost/api/v1/lobbies)
echo "Lobbies response: $LOBBIES"
echo ""

# Extract lobby IDs (they should match session IDs or be derived from them)
LOBBY_IDS=$(echo "$LOBBIES" | jq -r '.lobbies[] | .id' 2>/dev/null || echo "")

if [ -z "$LOBBY_IDS" ]; then
    echo "❌ No lobbies found! Cannot proceed."
    echo "Debug: Check if external agents created lobbies properly"
    exit 1
fi

echo "Found lobbies:"
echo "$LOBBY_IDS"
echo ""

# Step 3: Get Wolf apps to find Wolf-UI
echo "Fetching Wolf apps..."
APPS=$(docker exec helix-api-1 curl -s --unix-socket $WOLF_SOCKET http://localhost/api/v1/apps)
echo "Apps response: $APPS"

WOLF_UI_APP=$(echo "$APPS" | jq -r '.apps[] | select(.title == "Wolf UI" or .title == "Wolf-UI") | .id' | head -1)

if [ -z "$WOLF_UI_APP" ]; then
    echo "❌ Wolf-UI app not found! Cannot create test session."
    echo "Available apps:"
    echo "$APPS" | jq -r '.apps[] | .title'
    exit 1
fi

echo "Wolf-UI app ID: $WOLF_UI_APP"
echo ""

# Step 4: Create a Wolf streaming session by connecting via moonlight-web API
echo "Creating Wolf streaming session via moonlight-web..."

# Use moonlight-web to pair and launch Wolf-UI
# This will create a Wolf session we can use for switching

# First, check if we're already paired
PAIR_STATE=$(curl -s "http://localhost:8082/api/pair?uniqueid=automated-test")
echo "Pair state: $PAIR_STATE"

# Launch the Wolf-UI app to create a session
echo "Launching Wolf-UI app to create session..."
LAUNCH_RESULT=$(curl -s "http://localhost:8082/api/launch?uniqueid=automated-test&appid=$WOLF_UI_APP&mode=0")
echo "Launch result: $LAUNCH_RESULT"

sleep 5

# Step 5: Get active Wolf sessions to find our Wolf-UI session
echo "Finding Wolf-UI session..."
SESSIONS=$(docker exec helix-api-1 curl -s --unix-socket $WOLF_SOCKET http://localhost/api/v1/sessions)
echo "Active sessions: $SESSIONS"
echo ""

# Get the session ID of our Wolf-UI session
WOLF_UI_SESSION=$(echo "$SESSIONS" | jq -r '.sessions[] | select(.app_id == '$WOLF_UI_APP') | .session_id' | head -1)

if [ -z "$WOLF_UI_SESSION" ]; then
    echo "⚠️  No active Wolf-UI session found."
    echo "Attempting alternative: Use first available session"
    WOLF_UI_SESSION=$(echo "$SESSIONS" | jq -r '.sessions[0].session_id' 2>/dev/null || echo "")
fi

if [ -z "$WOLF_UI_SESSION" ]; then
    echo "❌ Cannot find or create Wolf session for testing."
    echo "Manual intervention needed:"
    echo "  1. Open http://localhost:8081"
    echo "  2. Launch Wolf-UI app"
    echo "  3. Get session ID and run:"
    echo "     WOLF_UI_SESSION=<id> ./automated-hang-test.sh --switch-only"
    exit 1
fi

echo "Using Wolf session: $WOLF_UI_SESSION"
echo ""

# Step 6: Automated lobby switching
echo "=== Starting Automated Lobby Switching ==="
echo "This will switch between lobbies and monitor for hangs..."
echo ""

LOBBY_ARRAY=($LOBBY_IDS)
LOBBY_COUNT=${#LOBBY_ARRAY[@]}

if [ "$LOBBY_COUNT" -lt 2 ]; then
    echo "❌ Need at least 2 lobbies for switching test (found $LOBBY_COUNT)"
    exit 1
fi

echo "Will switch between $LOBBY_COUNT lobbies"
echo ""

HANG_COUNT=0
SUCCESS_COUNT=0
MAX_ITERATIONS=30

for i in $(seq 1 $MAX_ITERATIONS); do
    LOBBY_INDEX=$((i % LOBBY_COUNT))
    LOBBY_ID="${LOBBY_ARRAY[$LOBBY_INDEX]}"

    echo "=== Iteration $i/$MAX_ITERATIONS ==="
    echo "Switching to lobby: $LOBBY_ID"

    # Join lobby via Wolf API
    JOIN_RESULT=$(docker exec helix-api-1 curl -s --unix-socket $WOLF_SOCKET \
        -X POST http://localhost/api/v1/lobbies/join \
        -H "Content-Type: application/json" \
        -d "{\"lobby_id\":\"$LOBBY_ID\",\"session_id\":\"$WOLF_UI_SESSION\"}" 2>&1)

    echo "  Join result: $JOIN_RESULT"

    # Wait a moment for switch to complete
    sleep 2

    # Check for refcount errors (hang indicator)
    ERRORS=$(docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml logs wolf --since 5s 2>&1 | grep -c "gst_mini_object_unref" || echo "0")

    # Check for HANG_DEBUG messages
    DEBUG_MSGS=$(docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml logs wolf --since 5s 2>&1 | grep "HANG_DEBUG" || echo "")

    if [ ! -z "$DEBUG_MSGS" ]; then
        echo "  [HANG_DEBUG messages captured]:"
        echo "$DEBUG_MSGS" | head -10
    fi

    if [ "$ERRORS" -gt 100 ]; then
        echo "  ❌ HANG DETECTED! (refcount errors: $ERRORS)"
        HANG_COUNT=$((HANG_COUNT + 1))

        # Save diagnostic logs
        TIMESTAMP=$(date +%s)
        docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml logs wolf --since 30s > "/tmp/wolf-hang-automated-$TIMESTAMP.log" 2>&1
        echo "  📋 Logs saved to: /tmp/wolf-hang-automated-$TIMESTAMP.log"

        # Show last HANG_DEBUG messages before hang
        echo "  Last events before hang:"
        cat "/tmp/wolf-hang-automated-$TIMESTAMP.log" | grep "HANG_DEBUG" | tail -20

        # Restart Wolf
        echo "  Restarting Wolf..."
        docker rm -f helix-wolf-1 >/dev/null 2>&1
        docker compose -f /home/luke/pm/helix/docker-compose.dev.yaml up -d wolf >/dev/null 2>&1
        sleep 10

        echo "  ⚠️  Hang #$HANG_COUNT detected at iteration $i"
    else
        echo "  ✅ Switch OK (errors: $ERRORS)"
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    fi

    echo ""
    sleep 1
done

echo "=== Test Complete ==="
echo "Total iterations: $MAX_ITERATIONS"
echo "Successful switches: $SUCCESS_COUNT"
echo "Hangs detected: $HANG_COUNT"
echo "Success rate: $(echo "scale=2; $SUCCESS_COUNT * 100 / $MAX_ITERATIONS" | bc)%"
echo ""

if [ "$HANG_COUNT" -gt 0 ]; then
    echo "📋 Diagnostic logs saved in /tmp/wolf-hang-automated-*.log"
    echo "Review with: ls -lt /tmp/wolf-hang-automated-*.log"
fi
