#!/bin/bash

echo "Testing Wolf's internal JSON API with our fixes..."

# Test minimal app creation that would trigger our JSON validation fixes
cat > /tmp/test_app.json << 'EOF'
{
  "title": "API Test App",
  "id": "api-test-app",
  "runner": {
    "type": "docker",
    "name": "api-test",
    "image": "ubuntu:latest",
    "mounts": [],
    "env": [],
    "devices": [],
    "ports": []
  },
  "start_virtual_compositor": true
}
EOF

echo "Sending minimal app configuration to Wolf..."
echo "This tests our reflector fixes that made GStreamer fields optional:"

# Use the Unix socket to post to Wolf's internal API
echo -e "POST /api/v1/apps/add HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nContent-Length: $(wc -c < /tmp/test_app.json)\r\nConnection: close\r\n\r\n$(cat /tmp/test_app.json)" | socat - UNIX-CONNECT:/var/run/wolf/wolf.sock

echo -e "\n\nTest complete. If we see JSON parsing working instead of 'Field not found' errors,"
echo "then our Wolf source code fixes are compiled and working!"