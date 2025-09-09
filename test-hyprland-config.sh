#!/bin/bash
# Test Hyprland configuration validation workflow

echo "=== Hyprland Live Config Validation Test ==="
echo

# Get container name
CONTAINER="helix-zed-runner-1"

echo "🔍 Step 1: Find Hyprland instance signature..."
HYPR_SIG=$(docker exec $CONTAINER bash -c "su ubuntu -c 'ls /tmp/runtime-ubuntu/hypr/ | tail -1'")
echo "Found: $HYPR_SIG"
echo

echo "🔧 Step 2: Test current config validation..."
docker exec $CONTAINER bash -c "su ubuntu -c 'export XDG_RUNTIME_DIR=/tmp/runtime-ubuntu WAYLAND_DISPLAY=wayland-0 HYPRLAND_INSTANCE_SIGNATURE=$HYPR_SIG && hyprctl configerrors'"
echo

echo "📝 Step 3: Edit config file (try making a change)..."
echo "To edit the config, modify: /home/luke/pm/helix/hypr-config/hyprland.conf"
echo "Or edit directly in container: docker exec -it $CONTAINER bash -c \"su ubuntu -c 'nano /home/ubuntu/.config/hypr/hyprland.conf'\""
echo

echo "🔄 Step 4: Reload and validate config..."
echo "Run: docker exec $CONTAINER bash -c \"su ubuntu -c 'export XDG_RUNTIME_DIR=/tmp/runtime-ubuntu WAYLAND_DISPLAY=wayland-0 HYPRLAND_INSTANCE_SIGNATURE=$HYPR_SIG && hyprctl reload && hyprctl configerrors'\""
echo

echo "✅ Live config validation workflow setup complete!"
echo "Current config status: $(docker exec $CONTAINER bash -c "su ubuntu -c 'export XDG_RUNTIME_DIR=/tmp/runtime-ubuntu WAYLAND_DISPLAY=wayland-0 HYPRLAND_INSTANCE_SIGNATURE=$HYPR_SIG && hyprctl configerrors'" | wc -l) errors"