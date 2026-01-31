#!/bin/bash
# Test Zed SSE MCP transport using the official MCP server-everything
# Verifies that legacy SSE transport (MCP 2024-11-05) works correctly
set -euo pipefail

# We'll ask the agent to echo this phrase and verify it comes back
TEST_PHRASE="helix-sse-mcp-test-$(date +%s)"
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

# Build and start official MCP server-everything with SSE transport
echo "=== Starting MCP server-everything (SSE mode) ==="
docker rm -f sse-mcp-test 2>/dev/null || true

# Build the image if needed
if ! docker images sse-mcp-test --format "{{.Repository}}" | grep -q sse-mcp-test; then
    echo "Building sse-mcp-test image..."
    docker build -t sse-mcp-test "$DIR/sse-mcp-server/"
fi

docker run -d --name sse-mcp-test --network helix_default -p 3001:3001 sse-mcp-test

# Wait for SSE server to start
echo "Waiting for SSE server..."
sleep 3
echo "SSE server started"

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

# Video capture disabled to save GPU memory
# Uncomment to enable video capture for debugging:
# echo ""
# echo "=== Starting video capture ==="
# VIDEO_FILE="$VIDEO_DIR/sse-mcp-test-$(date +%Y%m%d-%H%M%S).h264"
# "$HELIX" spectask stream "$SESSION" --output "$VIDEO_FILE" --duration 60 -v &
# VIDEO_PID=$!
# echo "Video capture started (PID: $VIDEO_PID), saving to: $VIDEO_FILE"
# sleep 3

# Ask the agent to use the echo tool
echo ""
echo "=== Sending prompt to agent ==="
RESPONSE=$("$HELIX" spectask send "$SESSION" "Use the echo tool from the everything-server MCP to echo this exact phrase: $TEST_PHRASE - then tell me what it returned." --wait --max-wait 180 2>&1) || true

# Stop video capture (disabled)
# echo ""
# echo "=== Stopping video capture ==="
# if [[ -n "${VIDEO_PID:-}" ]] && kill -0 "$VIDEO_PID" 2>/dev/null; then
#     kill "$VIDEO_PID" 2>/dev/null || true
#     wait "$VIDEO_PID" 2>/dev/null || true
# fi
# echo "Video saved to: $VIDEO_FILE"

# Check result
echo ""
echo "=== Result ==="
if echo "$RESPONSE" | grep -q "$TEST_PHRASE"; then
    echo "✓ PASSED: Agent echoed '$TEST_PHRASE' via SSE MCP"
    echo ""
    echo "Response excerpt:"
    echo "$RESPONSE" | grep -i "$TEST_PHRASE" | head -5
    exit 0
else
    echo "✗ FAILED: Echo phrase not found in response"
    echo ""
    echo "Full response:"
    echo "$RESPONSE"
    echo ""
    echo "SSE server logs:"
    docker logs sse-mcp-test 2>&1 | tail -30
    exit 1
fi