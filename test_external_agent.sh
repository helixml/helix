#!/bin/bash

# Test script to create an external Zed agent session

API_URL="http://localhost:8080"
SESSION_ID="test-zed-$(date +%s)"

echo "Creating external Zed agent session: $SESSION_ID"

curl -v -X POST "$API_URL/api/v1/external-agents" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${HELIX_API_KEY:-test-key}" \
  -d "{
    \"session_id\": \"$SESSION_ID\",
    \"user_id\": \"test-user\",
    \"input\": \"Test task: Create a simple hello world program\",
    \"project_path\": \"/home/retro/work/test-project\"
  }" | jq '.'

echo ""
echo "Session created: $SESSION_ID"
echo ""
echo "To connect with Moonlight, pair first with:"
echo "  moonlight pair localhost"
echo ""
echo "Then list apps:"
echo "  moonlight list localhost"
echo ""
echo "Screenshot available at:"
echo "  curl -s \"$API_URL/api/v1/external-agents/$SESSION_ID/screenshot\" -o screenshot.png"
