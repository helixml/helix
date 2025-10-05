#!/bin/bash
./quick-check-zed.sh > /tmp/bg.log 2>&1 &
sleep 20
CONT=$(docker ps | grep zed-external | awk '{print $NF}' | head -1)
if [ -n "$CONT" ]; then
  echo "Container: $CONT"
  docker logs "$CONT" 2>&1 | grep -E "Launching|ZED|WEBSOCKET|AGENT|info.*zed" | head -100
else
  echo "No container found"
fi
