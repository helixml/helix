#!/bin/bash

# Test Wolf Unix socket connectivity
echo "Testing Wolf socket connectivity..."

# Check if socket exists
if [ -S /var/run/wolf/wolf.sock ]; then
    echo "✓ Wolf socket exists"
else
    echo "✗ Wolf socket not found"
    exit 1
fi

# Test with a simple HTTP request over Unix socket
echo "Testing basic HTTP request..."
echo -e "GET /api/v1/apps HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n" | nc -U /var/run/wolf/wolf.sock || echo "nc failed, trying socat..."

# Try with socat if nc failed
echo "Testing with socat..."
echo -e "GET /api/v1/apps HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n" | socat - UNIX-CONNECT:/var/run/wolf/wolf.sock || echo "socat also failed"

echo "Wolf socket test complete."