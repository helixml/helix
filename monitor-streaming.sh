#!/bin/bash
# Monitor Wolf and moonlight-web for streaming activity

echo "=== Monitoring for Moonlight Web Streaming Activity ==="
echo "Waiting for streaming to start..."
echo "Please click 'Live Stream' in the Helix UI now"
echo ""
echo "Monitoring Wolf for RTP ping reception (Ctrl+C to stop)..."
echo "=========================================================="
echo ""

docker compose -f docker-compose.dev.yaml logs -f wolf 2>&1 | grep --line-buffered -E "RTP|Received ping|Starting.*ping|client_ip.*172\.19\.0\.15|Video port|Audio port"
