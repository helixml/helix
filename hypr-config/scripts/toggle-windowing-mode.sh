#!/bin/bash
# Toggle between tiling and windowing modes in Hyprland

# Check if windowing mode is active (stored in a file)
WINDOWING_MODE_FILE="$HOME/.cache/hyprland-windowing-mode"

if [ -f "$WINDOWING_MODE_FILE" ]; then
    # Currently in windowing mode, switch to tiling
    rm "$WINDOWING_MODE_FILE"
    
    # Set layout to dwindle (tiling)
    hyprctl keyword general:layout dwindle
    
    # Restore gaps and borders for tiling
    hyprctl keyword general:gaps_in 5
    hyprctl keyword general:gaps_out 20
    hyprctl keyword general:border_size 2
    
    # Make all existing windows tiled
    hyprctl clients -j | jq -r '.[] | select(.floating == true) | .address' | while read addr; do
        hyprctl dispatch settiled "address:$addr"
    done
    
    notify-send "Layout Mode" "Switched to Tiling Mode" -i preferences-desktop -t 2000
else
    # Currently in tiling mode, switch to windowing
    touch "$WINDOWING_MODE_FILE"
    
    # Keep dwindle layout but make new windows float by default
    hyprctl keyword general:layout dwindle
    
    # Reduce gaps for windowing mode
    hyprctl keyword general:gaps_in 2
    hyprctl keyword general:gaps_out 10
    hyprctl keyword general:border_size 1
    
    # Make all existing windows floating
    hyprctl clients -j | jq -r '.[] | select(.floating == false) | .address' | while read addr; do
        hyprctl dispatch setfloating "address:$addr"
    done
    
    notify-send "Layout Mode" "Switched to Windowing Mode" -i preferences-desktop -t 2000
fi

# Show current mode
if [ -f "$WINDOWING_MODE_FILE" ]; then
    echo "Windowing Mode (floating windows)"
else
    echo "Tiling Mode (dwindle layout)"
fi