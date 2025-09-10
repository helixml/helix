#!/bin/bash
set -e

# Background service to periodically refresh Wolf config when sessions change
# This allows new sessions to appear in Moonlight without restarting the server

REFRESH_INTERVAL="${WOLF_CONFIG_REFRESH_INTERVAL:-60}" # seconds
WOLF_PID_FILE="/tmp/wolf.pid"
LAST_CONFIG_HASH_FILE="/tmp/wolf_config_hash"

echo "Starting Wolf config refresher (checking every ${REFRESH_INTERVAL}s)..."

get_config_hash() {
    /usr/local/bin/generate-wolf-config.sh > /tmp/new_config.toml 2>/dev/null
    sha256sum /tmp/new_config.toml | cut -d' ' -f1
}

# Get initial config hash
current_hash=$(get_config_hash)
echo "$current_hash" > "$LAST_CONFIG_HASH_FILE"
echo "Initial config hash: $current_hash"

while true; do
    sleep "$REFRESH_INTERVAL"
    
    # Generate new config and check if it changed
    new_hash=$(get_config_hash)
    last_hash=$(cat "$LAST_CONFIG_HASH_FILE" 2>/dev/null || echo "")
    
    if [ "$new_hash" != "$last_hash" ]; then
        echo "Config changed! Old: $last_hash, New: $new_hash"
        
        # Replace the actual config
        mv /tmp/new_config.toml "$WOLF_CFG_FILE"
        echo "$new_hash" > "$LAST_CONFIG_HASH_FILE"
        
        echo "Updated Wolf config with new session list"
        echo "Note: Wolf server may need manual restart to pick up new apps"
        
        # Optional: Send SIGHUP to Wolf if it supports config reload
        # if [ -f "$WOLF_PID_FILE" ]; then
        #     wolf_pid=$(cat "$WOLF_PID_FILE")
        #     if kill -0 "$wolf_pid" 2>/dev/null; then
        #         echo "Sending SIGHUP to Wolf (PID: $wolf_pid)"
        #         kill -HUP "$wolf_pid" || true
        #     fi
        # fi
    else
        echo "Config unchanged (hash: $current_hash)"
    fi
done