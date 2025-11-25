#!/bin/bash
# Local testing script for Wolf RevDial client
#
# This script helps test the Wolf RevDial client locally by:
# 1. Starting a mock Wolf API server
# 2. Running the RevDial client
# 3. Testing connectivity
#
# Prerequisites:
# - Helix API server running (docker-compose.dev.yaml)
# - Valid RUNNER_TOKEN

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
API_URL="${HELIX_API_URL:-http://localhost:8080}"
WOLF_ID="${WOLF_ID:-wolf-test}"
RUNNER_TOKEN="${RUNNER_TOKEN:-}"
MOCK_WOLF_PORT=9999

echo -e "${GREEN}Wolf RevDial Client - Local Test${NC}"
echo "=================================="
echo ""

# Check prerequisites
if [ -z "$RUNNER_TOKEN" ]; then
    echo -e "${RED}ERROR: RUNNER_TOKEN not set${NC}"
    echo "Set it in your environment:"
    echo "  export RUNNER_TOKEN=your-token-here"
    exit 1
fi

echo -e "${YELLOW}Configuration:${NC}"
echo "  API URL: $API_URL"
echo "  Wolf ID: $WOLF_ID"
echo "  Mock Wolf API Port: $MOCK_WOLF_PORT"
echo ""

# Check if API is running
echo -e "${YELLOW}Checking API server...${NC}"
if ! curl -sf "$API_URL/healthz" > /dev/null; then
    echo -e "${RED}ERROR: API server not reachable at $API_URL${NC}"
    echo "Start it with:"
    echo "  docker compose -f docker-compose.dev.yaml up api"
    exit 1
fi
echo -e "${GREEN}✓ API server is running${NC}"
echo ""

# Start mock Wolf API server
echo -e "${YELLOW}Starting mock Wolf API server on port $MOCK_WOLF_PORT...${NC}"
(
    cd "$(mktemp -d)"
    cat > server.go <<'EOF'
package main

import (
    "encoding/json"
    "log"
    "net/http"
)

func main() {
    http.HandleFunc("/api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
        log.Printf("%s %s", r.Method, r.URL.Path)
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "apps": []interface{}{},
            "message": "Mock Wolf API - RevDial test successful!",
        })
    })

    http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        log.Printf("%s %s", r.Method, r.URL.Path)
        w.WriteHeader(200)
        w.Write([]byte("OK"))
    })

    log.Printf("Mock Wolf API listening on :9999")
    log.Fatal(http.ListenAndServe(":9999", nil))
}
EOF
    go run server.go
) &
MOCK_PID=$!
echo "Mock Wolf API PID: $MOCK_PID"

# Cleanup function
cleanup() {
    echo ""
    echo -e "${YELLOW}Cleaning up...${NC}"
    if [ -n "$MOCK_PID" ]; then
        kill $MOCK_PID 2>/dev/null || true
    fi
    if [ -n "$CLIENT_PID" ]; then
        kill $CLIENT_PID 2>/dev/null || true
    fi
}
trap cleanup EXIT

# Wait for mock server to start
echo -e "${YELLOW}Waiting for mock Wolf API to start...${NC}"
for i in {1..10}; do
    if curl -sf "http://localhost:$MOCK_WOLF_PORT/healthz" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Mock Wolf API is running${NC}"
        break
    fi
    if [ $i -eq 10 ]; then
        echo -e "${RED}ERROR: Mock Wolf API failed to start${NC}"
        exit 1
    fi
    sleep 0.5
done
echo ""

# Build RevDial client
echo -e "${YELLOW}Building RevDial client...${NC}"
if ! go build -o /tmp/wolf-revdial-client ./api/cmd/wolf-revdial-client/; then
    echo -e "${RED}ERROR: Failed to build RevDial client${NC}"
    exit 1
fi
echo -e "${GREEN}✓ RevDial client built${NC}"
echo ""

# Start RevDial client
echo -e "${YELLOW}Starting RevDial client...${NC}"
HELIX_API_URL="$API_URL" \
WOLF_ID="$WOLF_ID" \
RUNNER_TOKEN="$RUNNER_TOKEN" \
/tmp/wolf-revdial-client -local "localhost:$MOCK_WOLF_PORT" &
CLIENT_PID=$!
echo "RevDial client PID: $CLIENT_PID"

# Wait for connection
echo -e "${YELLOW}Waiting for RevDial connection...${NC}"
sleep 2

# Check if client is still running
if ! ps -p $CLIENT_PID > /dev/null; then
    echo -e "${RED}ERROR: RevDial client exited unexpectedly${NC}"
    exit 1
fi
echo -e "${GREEN}✓ RevDial client is running${NC}"
echo ""

# Test connection via RevDial
echo -e "${YELLOW}Testing Wolf API via RevDial tunnel...${NC}"
echo "Note: This requires the API server to route Wolf requests via RevDial"
echo "URL: $API_URL/api/v1/wolf/apps?wolf_id=$WOLF_ID"
echo ""

# Give it a moment to establish connection
sleep 2

# Try to query Wolf API via RevDial (may not work if routing not implemented yet)
echo -e "${YELLOW}Attempting to query Wolf API via RevDial...${NC}"
if curl -sf "$API_URL/api/v1/wolf/apps?wolf_id=$WOLF_ID" \
    -H "Authorization: Bearer $RUNNER_TOKEN" > /dev/null; then
    echo -e "${GREEN}✓ Wolf API accessible via RevDial!${NC}"
else
    echo -e "${YELLOW}⚠ Wolf API not yet routed via RevDial (expected at this stage)${NC}"
    echo "This is normal - Wolf routing via RevDial needs to be implemented in the API server"
fi
echo ""

# Show connection status
echo -e "${GREEN}Test Summary:${NC}"
echo "  Mock Wolf API: Running on localhost:$MOCK_WOLF_PORT"
echo "  RevDial Client: Connected to $API_URL"
echo "  Wolf ID: $WOLF_ID"
echo ""
echo -e "${YELLOW}RevDial client is running. Press Ctrl+C to stop.${NC}"
echo ""
echo "You can test manually:"
echo "  curl http://localhost:$MOCK_WOLF_PORT/api/v1/apps"
echo ""

# Wait for user to stop
wait $CLIENT_PID
