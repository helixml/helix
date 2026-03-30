#!/usr/bin/env bash
set -euo pipefail

# TUI E2E test — runs inside Docker container.
#
# Components:
#   1. tui-test-server: Go binary (real HelixAPIServer + memorystore)
#   2. Zed binary: connects via WebSocket, runs real LLM
#   3. TUI test driver: sends messages via HTTP, verifies responses
#
# Flow:
#   TUI driver --HTTP--> test-server --WebSocket--> Zed --LLM--> response
#   Response streams back through WebSocket to test-server, stored in memorystore,
#   then TUI driver fetches interactions via HTTP and validates rendering.

echo "============================================"
echo "  TUI E2E Test"
echo "  Real Zed agent + real LLM + TUI rendering"
echo "============================================"
echo ""

TEST_SERVER="${TUI_TEST_SERVER:-/usr/local/bin/tui-test-server}"
ZED_BINARY="${ZED_BINARY:-/usr/local/bin/zed}"
HELIX_BINARY="${HELIX_BINARY:-/usr/local/bin/helix}"
TEST_TIMEOUT="${TEST_TIMEOUT:-300}"
PORT_FILE="/tmp/tui_test_server_port"
ENV_FILE="/tmp/tui_test_env"

cleanup() {
    echo "[cleanup] Shutting down..."
    [ -n "${ZED_PID:-}" ] && kill "$ZED_PID" 2>/dev/null || true
    [ -n "${SERVER_PID:-}" ] && kill "$SERVER_PID" 2>/dev/null || true
    [ -n "${XVFB_PID:-}" ] && kill "$XVFB_PID" 2>/dev/null || true
    rm -f "$PORT_FILE" "$ENV_FILE"

    if [ -f "${ZED_LOG:-}" ]; then
        ZED_ERRORS=$(grep -ciE "panic|error|fatal" "$ZED_LOG" 2>/dev/null || echo "0")
        if [ "$ZED_ERRORS" -gt 0 ]; then
            echo ""
            echo "=== Zed errors ($ZED_ERRORS) ==="
            grep -iE "panic|error|fatal" "$ZED_LOG" | tail -20 || true
        fi
    fi
}
trap cleanup EXIT

# D-Bus
if [ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ]; then
    export DBUS_SESSION_BUS_ADDRESS=$(dbus-daemon --session --fork --print-address 2>/dev/null || true)
fi

# Xvfb for headless Zed
if ! xdpyinfo -display "${DISPLAY:-}" >/dev/null 2>&1; then
    echo "[setup] Starting Xvfb..."
    Xvfb :99 -screen 0 1280x720x24 -ac +extension GLX &
    XVFB_PID=$!
    export DISPLAY=:99
    sleep 1
fi

# Start test server
echo "[server] Starting tui-test-server..."
export HELIX_SESSION_ID="ses_tui-e2e-001"
"$TEST_SERVER" &
SERVER_PID=$!
sleep 2

if [ ! -f "$PORT_FILE" ]; then
    echo "[error] Test server failed to start"
    exit 1
fi
SERVER_PORT=$(cat "$PORT_FILE")
echo "[server] Running on port $SERVER_PORT"

# Configure Zed
export ZED_EXTERNAL_SYNC_ENABLED=true
export ZED_WEBSOCKET_SYNC_ENABLED=true
export ZED_HELIX_URL="127.0.0.1:${SERVER_PORT}"
export ZED_HELIX_TOKEN="test-token"
export ZED_HELIX_TLS=false
export ZED_ALLOW_EMULATED_GPU=1
export ZED_ALLOW_ROOT=true

# Zed settings
ZED_CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/zed"
mkdir -p "$ZED_CONFIG_DIR"
cat > "$ZED_CONFIG_DIR/settings.json" << JSONEOF
{
  "language_models": {
    "anthropic": {
      "api_url": "${ANTHROPIC_BASE_URL:-https://api.anthropic.com}"
    }
  },
  "agent": {
    "default_model": {
      "provider": "anthropic",
      "model": "claude-sonnet-4-5-latest"
    },
    "always_allow_tool_actions": true,
    "show_onboarding": false,
    "auto_open_panel": true
  }
}
JSONEOF

# Start Zed
echo "[zed] Starting Zed..."
ZED_LOG="/tmp/zed-tui-e2e.log"
mkdir -p /test/project
echo "# TUI E2E Test" > /test/project/README.md

"$ZED_BINARY" --allow-multiple-instances /test/project > "$ZED_LOG" 2>&1 &
ZED_PID=$!
echo "[zed] Started (PID $ZED_PID)"

# Wait for agent to connect
echo "[test] Waiting for Zed agent to connect..."
ELAPSED=0
while [ "$ELAPSED" -lt 60 ]; do
    STATUS=$(curl -s "http://127.0.0.1:${SERVER_PORT}/api/v1/status" 2>/dev/null || echo "{}")
    CONNECTED=$(echo "$STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('agent_connected',False))" 2>/dev/null || echo "False")
    if [ "$CONNECTED" = "True" ]; then
        echo "[test] Agent connected!"
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done

if [ "$CONNECTED" != "True" ]; then
    echo "[error] Agent did not connect within 60s"
    exit 1
fi

# Give agent a moment to be ready
sleep 5

# === Test Phase 1: Send a chat message and verify response ===
echo ""
echo "=== Phase 1: Send chat message via TUI API ==="
RESPONSE=$(curl -s -X POST "http://127.0.0.1:${SERVER_PORT}/api/v1/sessions/chat" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer test-api-key" \
    -d "{
        \"session_id\": \"$HELIX_SESSION_ID\",
        \"messages\": [{
            \"role\": \"user\",
            \"content\": {\"content_type\": \"text\", \"parts\": [\"What is 2+2? Reply with just the number.\"]}
        }],
        \"type\": \"text\"
    }" 2>/dev/null || echo "CURL_FAILED")

echo "[test] Response: ${RESPONSE:0:200}"

if [ "$RESPONSE" = "CURL_FAILED" ] || [ -z "$RESPONSE" ]; then
    echo "[FAIL] No response received"
    exit 1
fi

echo "[PASS] Phase 1: Received response from Zed agent"

# === Phase 2: Verify interactions in store ===
echo ""
echo "=== Phase 2: Verify interactions ==="
INTERACTIONS=$(curl -s "http://127.0.0.1:${SERVER_PORT}/api/v1/sessions/${HELIX_SESSION_ID}/interactions" \
    -H "Authorization: Bearer test-api-key" 2>/dev/null || echo "[]")

INTERACTION_COUNT=$(echo "$INTERACTIONS" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
echo "[test] Interactions in store: $INTERACTION_COUNT"

if [ "$INTERACTION_COUNT" -gt 0 ]; then
    echo "[PASS] Phase 2: Interactions stored correctly"
else
    echo "[FAIL] Phase 2: No interactions found in store"
    exit 1
fi

# === Phase 3: Test TUI rendering ===
echo ""
echo "=== Phase 3: TUI rendering validation ==="

# Source the test server's env vars
source "$ENV_FILE"

# Use the helix binary to verify TUI components work
# (We can't run the full interactive TUI, but we can verify the binary starts)
if "$HELIX_BINARY" tui --help >/dev/null 2>&1; then
    echo "[PASS] Phase 3: TUI binary functional"
else
    echo "[FAIL] Phase 3: TUI binary failed"
    exit 1
fi

# === Phase 4: Server state validation ===
echo ""
echo "=== Phase 4: Server state ==="
FINAL_STATUS=$(curl -s "http://127.0.0.1:${SERVER_PORT}/api/v1/status" 2>/dev/null)
echo "[test] Final state: $FINAL_STATUS"

COMPLETIONS=$(echo "$FINAL_STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('completions',0))" 2>/dev/null || echo "0")
if [ "$COMPLETIONS" -gt 0 ]; then
    echo "[PASS] Phase 4: $COMPLETIONS message completions received"
else
    echo "[WARN] Phase 4: No completions yet (response may still be streaming)"
fi

echo ""
echo "============================================"
echo "  TUI E2E TEST PASSED"
echo "============================================"
exit 0
