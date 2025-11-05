#!/bin/bash

echo "[start-zed-helix] Waiting 5 seconds for labwc compositor to initialize..."
sleep 5

echo "[start-zed-helix] Starting Zed auto-restart loop"

while true; do
    echo "[start-zed-helix] Launching Zed..."

    # Launch Zed with Helix integration
    /usr/local/bin/zed --foreground 2>&1 | while IFS= read -r line; do
        echo "[zed] $line"
    done

    EXIT_CODE=$?
    echo "[start-zed-helix] Zed exited with code $EXIT_CODE"

    # Wait 2 seconds before restarting
    echo "[start-zed-helix] Waiting 2 seconds before restart..."
    sleep 2
done
