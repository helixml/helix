#!/bin/bash
# Quick test to determine if Docker network blocks UDP between containers

echo "=== Quick UDP Connectivity Test ==="
echo ""

WOLF_IP="172.19.0.50"
ML_WEB_IP="172.19.0.15"

echo "Installing netcat in containers (if needed)..."
docker exec helix-moonlight-web-1 bash -c "command -v nc || (apt update -qq && apt install -y netcat-openbsd)" 2>&1 | grep -v "debconf" | tail -3

echo ""
echo "Test 1: Sending UDP packet from moonlight-web to Wolf port 48100"
echo "--------------------------------------------------------------"
docker exec helix-moonlight-web-1 bash -c 'echo "MANUAL_UDP_TEST_PING" | nc -u -w1 172.19.0.50 48100'
echo "Sent UDP packet"

sleep 2

echo ""
echo "Test 2: Checking if Wolf received it..."
echo "---------------------------------------"
docker compose -f docker-compose.dev.yaml logs wolf --since 10s 2>&1 | grep -E "RTP|Received|UDP|MANUAL" || echo "❌ Wolf did NOT receive the UDP packet"

echo ""
echo "Test 3: Reverse direction - Wolf to moonlight-web"
echo "--------------------------------------------------"
echo "(Would need netcat listener in moonlight-web - skipping for now)"

echo ""
echo "=== Results ==="
echo ""
echo "If Wolf logs show 'Received ping' or 'MANUAL_UDP_TEST_PING':"
echo "  → UDP routing works! Issue is in moonlight-web streamer code"
echo ""
echo "If Wolf logs show nothing:"
echo "  → Docker network blocks UDP between containers"
echo "  → Fix: Use 'network_mode: host' for moonlight-web"
echo ""
