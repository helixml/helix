#!/bin/bash
# Test script to debug moonlight-web streaming to Wolf

echo "=== Moonlight Web Streaming Debug Test ==="
echo ""

# Get container IPs
WOLF_IP=$(docker inspect helix-wolf-1 | grep '"IPAddress"' | tail -1 | awk -F'"' '{print $4}')
ML_WEB_IP=$(docker inspect helix-moonlight-web-1 | grep '"IPAddress"' | tail -1 | awk -F'"' '{print $4}')

echo "Wolf container IP: $WOLF_IP"
echo "Moonlight-web container IP: $ML_WEB_IP"
echo ""

echo "Testing network connectivity..."
docker exec helix-moonlight-web-1 ping -c 2 $WOLF_IP 2>&1 | grep -E "transmitted|received"
echo ""

echo "Testing Wolf HTTP port 47989..."
docker exec helix-api-1 curl -s --max-time 2 http://$WOLF_IP:47989/serverinfo | head -2
echo ""

echo "Checking Wolf RTP ping ports (should be listening on 0.0.0.0)..."
docker exec helix-wolf-1 ss -ulnp | grep -E "48100|48200|47999" || echo "No UDP listeners found"
echo ""

echo "=== Next Steps ==="
echo "1. Create a new external agent session via Helix UI"
echo "2. Click 'Live Stream' button"
echo "3. Monitor these logs:"
echo "   docker compose -f docker-compose.dev.yaml logs -f wolf | grep -E 'RTP|UDP|client_ip'"
echo "   docker compose -f docker-compose.dev.yaml logs -f moonlight-web | grep -E 'Stream|IDR|ping'"
