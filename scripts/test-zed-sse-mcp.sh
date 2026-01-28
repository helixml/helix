#!/bin/bash
# Test Zed SSE MCP transport by asking an agent to retrieve a secret
# Captures video during the test for debugging
set -euo pipefail

SECRET="HELIX-SSE-MCP-SECRET-7f3a9b2c"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELIX="${HELIX_CLI:-/tmp/helix}"
VIDEO_DIR="${VIDEO_DIR:-/tmp/sse-mcp-test}"

# Load credentials from various sources (optional - CLI has dev defaults)
if [[ -f "$DIR/../.env.usercreds" ]]; then
    set -a && source "$DIR/../.env.usercreds" && set +a
elif [[ -f "$HOME/.helix/credentials" ]]; then
    set -a && source "$HOME/.helix/credentials" && set +a
fi

# Use dev defaults if not set (matches CLI defaults in spectask.go)
export HELIX_URL="${HELIX_URL:-http://localhost:8080}"
export HELIX_API_KEY="${HELIX_API_KEY:-oh-hallo-insecure-token}"

echo "Using HELIX_URL=$HELIX_URL"
echo "Using HELIX_API_KEY=${HELIX_API_KEY:0:10}..."

# Cleanup function
cleanup() {
    echo "Cleaning up..."
    # Stop video capture if running
    if [[ -n "${VIDEO_PID:-}" ]] && kill -0 "$VIDEO_PID" 2>/dev/null; then
        kill "$VIDEO_PID" 2>/dev/null || true
        wait "$VIDEO_PID" 2>/dev/null || true
    fi
    # Stop session
    if [[ -n "${SESSION:-}" ]]; then
        "$HELIX" spectask stop "$SESSION" 2>/dev/null || true
    fi
    # Remove SSE server container
    docker rm -f sse-mcp-test 2>/dev/null || true
    # Note: We don't delete the project - it's useful for debugging failed tests
}
trap cleanup EXIT

# Create video output directory
mkdir -p "$VIDEO_DIR"

# Start SSE server
echo "=== Starting SSE MCP test server ==="
docker rm -f sse-mcp-test 2>/dev/null || true
docker run -d --name sse-mcp-test --network helix_default -p 3333:3333 \
    -v "$DIR/../../zed/script/test_sse_mcp_server.py:/app/server.py:ro" \
    python:3.11-slim python /app/server.py 3333

# Wait for SSE server to be ready
echo "Waiting for SSE server..."
for i in {1..10}; do
    if curl -sf --max-time 3 http://localhost:3333/secret 2>/dev/null | grep -q "$SECRET"; then
        echo "SSE server ready"
        break
    fi
    sleep 1
done

# Verify SSE server is working
if ! curl -sf --max-time 3 http://localhost:3333/secret 2>/dev/null | grep -q "$SECRET"; then
    echo "ERROR: SSE server failed to start"
    docker logs sse-mcp-test
    exit 1
fi

# Create a new project for this test
echo ""
echo "=== Creating test project ==="
PROJECT_NAME="SSE-MCP-Test-$(date +%Y%m%d-%H%M%S)"
PROJECT=$("$HELIX" project fork modern-todo-app --name "$PROJECT_NAME" 2>&1 | grep "Project ID:" | awk '{print $3}')
if [[ -z "$PROJECT" ]]; then
    echo "ERROR: Failed to create project"
    "$HELIX" project fork modern-todo-app --name "$PROJECT_NAME" 2>&1
    exit 1
fi
echo "Project: $PROJECT ($PROJECT_NAME)"

# Create/update agent
echo ""
echo "=== Creating test agent ==="
sleep 2  # Let API settle after project creation

# Retry agent creation up to 3 times (API can be slow on first call)
for attempt in 1 2 3; do
    echo "Attempt $attempt to create agent..."
    AGENT_OUTPUT=$("$HELIX" agent apply -f "$DIR/test-zed-sse-mcp-agent.yaml" 2>&1) || true
    # Extract app ID (starts with app_)
    AGENT=$(echo "$AGENT_OUTPUT" | grep -oE 'app_[a-z0-9]+' | head -1)
    if [[ -n "$AGENT" ]]; then
        break
    fi
    echo "Agent creation attempt $attempt failed:"
    echo "$AGENT_OUTPUT"
    if [[ $attempt -lt 3 ]]; then
        echo "Retrying in 3 seconds..."
        sleep 3
    fi
done

if [[ -z "$AGENT" ]]; then
    echo "ERROR: Failed to create agent after 3 attempts"
    exit 1
fi
echo "Agent: $AGENT"

# Start session
echo ""
echo "=== Starting session ==="
SESSION=$("$HELIX" spectask start -q --agent "$AGENT" --project "$PROJECT" -n "SSE MCP Test $(date +%H:%M:%S)")
echo "Session: $SESSION"

# Wait for session to initialize
echo "Waiting 20s for session to initialize..."
sleep 20

# Start video capture in background (60 seconds should be plenty)
echo ""
echo "=== Starting video capture ==="
VIDEO_FILE="$VIDEO_DIR/sse-mcp-test-$(date +%Y%m%d-%H%M%S).h264"
"$HELIX" spectask stream "$SESSION" --output "$VIDEO_FILE" --duration 60 -v &
VIDEO_PID=$!
echo "Video capture started (PID: $VIDEO_PID), saving to: $VIDEO_FILE"

# Give the stream a moment to connect
sleep 3

# Ask for the secret
echo ""
echo "=== Sending prompt to agent ==="
RESPONSE=$("$HELIX" spectask send "$SESSION" "Use the get_secret tool and tell me exactly what it returns. The tool is provided by the secret-server MCP." --wait --max-wait 180 2>&1) || true

# Stop video capture
echo ""
echo "=== Stopping video capture ==="
if kill -0 "$VIDEO_PID" 2>/dev/null; then
    kill "$VIDEO_PID" 2>/dev/null || true
    wait "$VIDEO_PID" 2>/dev/null || true
fi
echo "Video saved to: $VIDEO_FILE"

# Check result
echo ""
echo "=== Result ==="
if echo "$RESPONSE" | grep -q "$SECRET"; then
    echo "✓ PASSED: Agent retrieved '$SECRET' via SSE MCP"
    echo ""
    echo "Response excerpt:"
    echo "$RESPONSE" | grep -i secret | head -5
    exit 0
else
    echo "✗ FAILED: Secret not found in response"
    echo ""
    echo "Full response:"
    echo "$RESPONSE"
    echo ""
    echo "SSE server logs:"
    docker logs sse-mcp-test 2>&1 | tail -30
    echo ""
    echo "Video file for debugging: $VIDEO_FILE"
    echo "Convert to playable format: ffmpeg -i $VIDEO_FILE -c copy ${VIDEO_FILE%.h264}.mp4"
    exit 1
fi