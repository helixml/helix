#!/bin/bash
# Verification script for unique Moonlight client IDs

echo "=== Verifying Unique Moonlight Client IDs ==="
echo ""

echo "1. Wolf Sessions (should show different client_id for each agent):"
echo "-------------------------------------------------------------------"
docker compose -f docker-compose.dev.yaml exec api curl --unix-socket /var/run/wolf/wolf.sock \
  http://localhost/api/v1/sessions 2>/dev/null | jq '.sessions[] | {client_id, app_id, client_ip}'
echo ""

echo "2. Moonlight-web Sessions (should show unique client_unique_id):"
echo "-------------------------------------------------------------------"
docker compose -f docker-compose.dev.yaml exec api curl http://moonlight-web:8080/api/sessions 2>/dev/null | jq '.sessions[] | {session_id, client_unique_id, mode, has_websocket}'
echo ""

echo "3. Check for GStreamer errors (should be empty or minimal):"
echo "-------------------------------------------------------------------"
docker compose -f docker-compose.dev.yaml logs wolf --tail 100 2>&1 | grep -c "gst_mini_object_unref"
echo "Error count above ^"
echo ""

echo "4. Check for AES decryption errors (should be empty or minimal):"
echo "-------------------------------------------------------------------"
docker compose -f docker-compose.dev.yaml logs wolf --tail 100 2>&1 | grep -c "EVP_DecryptFinal_ex failed"
echo "Error count above ^"
echo ""

echo "✅ If you see unique client_id values and error counts are 0, the fix is working!"
