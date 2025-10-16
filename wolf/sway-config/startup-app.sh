#!/bin/bash
# GOW base-app startup script for Helix Personal Dev Environment
set -e

echo "Starting Helix Personal Dev Environment with Sway..."

# Create symlink to Zed binary if not exists
if [ -f /zed-build/zed ] && [ ! -f /usr/local/bin/zed ]; then
    sudo ln -sf /zed-build/zed /usr/local/bin/zed
    echo "Created symlink: /usr/local/bin/zed -> /zed-build/zed"
fi

# Configure Zed settings
echo "Configuring Zed settings..."
mkdir -p $HOME/.config/zed
cat > $HOME/.config/zed/settings.json << 'EOF'
{
  "agent": {
    "default_model": {
      "provider": "anthropic",
      "model": "claude-sonnet-4-5-latest"
    },
    "model_parameters": []
  },
  "ui_font_size": 16,
  "buffer_font_size": 15,
  "theme": {
    "mode": "dark",
    "light": "One Light",
    "dark": "One Dark"
  }
}
EOF
echo "Zed settings configured"

# Start screenshot server in background (if binary exists)
# NOTE: Start AFTER Sway is running to get correct WAYLAND_DISPLAY
# We'll start it later in the script after Sway initializes

# Source GOW's launch-comp.sh for the launcher function
echo "Starting Sway and launching Zed via GOW launcher..."
source /opt/gow/launch-comp.sh

# Custom launcher that adds our HiDPI configuration
custom_launcher() {
  export GAMESCOPE_WIDTH=${GAMESCOPE_WIDTH:-1920}
  export GAMESCOPE_HEIGHT=${GAMESCOPE_HEIGHT:-1080}
  export GAMESCOPE_REFRESH=${GAMESCOPE_REFRESH:-60}

  if [ -n "$RUN_SWAY" ]; then
    echo "[Sway] - Starting with custom Helix configuration: \`$@\`"

    export SWAYSOCK=${XDG_RUNTIME_DIR}/sway.socket
    export SWAY_STOP_ON_APP_EXIT=${SWAY_STOP_ON_APP_EXIT:-"yes"}
    export XDG_CURRENT_DESKTOP=sway
    export XDG_SESSION_DESKTOP=sway
    export XDG_SESSION_TYPE=wayland

    # Copy waybar default config and customize it
    mkdir -p $HOME/.config/waybar
    cp -u /cfg/waybar/* $HOME/.config/waybar/

    # Configure dark mode for GTK applications
    mkdir -p $HOME/.config/gtk-3.0
    echo "[Settings]" > $HOME/.config/gtk-3.0/settings.ini
    echo "gtk-application-prefer-dark-theme=1" >> $HOME/.config/gtk-3.0/settings.ini

    # Create custom waybar config with launcher icons
    cat > $HOME/.config/waybar/config.jsonc << 'EOF'
// -*- mode: jsonc -*-
{
  "layer": "top",
  "position": "top",
  "height": 30,
  "spacing": 4,
  "modules-left": [
    "sway/workspaces",
    "sway/mode",
    "sway/scratchpad",
    "custom/firefox",
    "custom/ghostty",
    "custom/onlyoffice"
  ],
  "modules-center": [
    "sway/window"
  ],
  "modules-right": [
    "pulseaudio",
    "network",
    "cpu",
    "memory",
    "temperature",
    "sway/language",
    "custom/clock"
  ],
  "custom/firefox": {
    "format": "🦊",
    "tooltip": true,
    "tooltip-format": "Firefox",
    "on-click": "firefox"
  },
  "custom/ghostty": {
    "format": "👻",
    "tooltip": true,
    "tooltip-format": "Ghostty Terminal",
    "on-click": "ghostty"
  },
  "custom/onlyoffice": {
    "format": "📄",
    "tooltip": true,
    "tooltip-format": "OnlyOffice",
    "on-click": "onlyoffice-desktopeditors"
  },
  "sway/mode": {
    "format": "<span style=\"italic\">{}</span>"
  },
  "sway/scratchpad": {
    "format": "{icon} {count}",
    "show-empty": false,
    "format-icons": ["", ""],
    "tooltip": true,
    "tooltip-format": "{app}: {title}"
  },
  "custom/clock": {
    "format": "  {}",
    "tooltip": false,
    "interval": 60,
    "exec": "date +'%d %a %H:%M'"
  },
  "cpu": {
    "format": "{usage}% ",
    "tooltip": false
  },
  "memory": {
    "format": "{}% "
  },
  "pulseaudio": {
    "format": "{volume}% {icon} {format_source}",
    "format-bluetooth": "{volume}% {icon} {format_source}",
    "format-bluetooth-muted": " {icon} {format_source}",
    "format-muted": " {format_source}",
    "format-source": "{volume}% ",
    "format-source-muted": "",
    "format-icons": {
      "headphone": "",
      "hands-free": "",
      "headset": "",
      "phone": "",
      "portable": "",
      "car": "",
      "default": ["", "", ""]
    }
  }
}
EOF

    # Copy base Sway config
    mkdir -p $HOME/.config/sway/
    cp /cfg/sway/config $HOME/.config/sway/config

    # Add our custom Helix configuration
    echo "" >> $HOME/.config/sway/config
    echo "# Helix Personal Dev Environment custom configuration" >> $HOME/.config/sway/config
    echo "# Set Helix wallpaper" >> $HOME/.config/sway/config
    echo "output * bg /usr/share/backgrounds/helix-logo.png fill" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    echo "# Configure HiDPI scaling for display WL-1" >> $HOME/.config/sway/config
    echo "output WL-1 scale 3" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    echo "# Map Caps Lock to Ctrl (replace caps lock entirely)" >> $HOME/.config/sway/config
    echo "input type:keyboard xkb_options caps:ctrl_nocaps" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    echo "# Additional key bindings for our tools" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Shift+Return exec ghostty" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Shift+f exec firefox" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Shift+o exec onlyoffice-desktopeditors" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    echo "# Start screenshot server after Sway is ready (wayland-1 available)" >> $HOME/.config/sway/config
    echo "exec WAYLAND_DISPLAY=wayland-1 /usr/local/bin/screenshot-server > /tmp/screenshot-server.log 2>&1" >> $HOME/.config/sway/config

    # Add resolution and app launch (like the original launcher)
    echo "output * resolution ${GAMESCOPE_WIDTH}x${GAMESCOPE_HEIGHT} position 0,0" >> $HOME/.config/sway/config
    echo -n "workspace main; exec $@" >> $HOME/.config/sway/config

    # Add killall sway if needed
    if [ "$SWAY_STOP_ON_APP_EXIT" == "yes" ]; then
      echo -n " && killall sway" >> $HOME/.config/sway/config
    fi

    # Start sway
    dbus-run-session -- sway --unsupported-gpu
  else
    echo "[exec] Starting: $@"
    exec $@
  fi
}

custom_launcher /usr/local/bin/start-zed-helix.sh
