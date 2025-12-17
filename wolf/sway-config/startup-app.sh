#!/bin/bash
# GOW base-app startup script for Helix Desktop (Sway)
# Used for: Desktop sessions, Spec Task sessions, and Exploratory sessions
set -e

echo "Starting Helix Desktop (Sway)..."

# NOTE: Telemetry firewall is configured in the sandbox container (Wolf host),
# not inside agent containers. This provides centralized monitoring across all agents.

# Create symlink to Zed binary if not exists
if [ -f /zed-build/zed ] && [ ! -f /usr/local/bin/zed ]; then
    sudo ln -sf /zed-build/zed /usr/local/bin/zed
    echo "Created symlink: /usr/local/bin/zed -> /zed-build/zed"
fi

# Workspace setup: Wolf executor mounts workspace at BOTH paths via bind mount:
# 1. $WORKSPACE_DIR (e.g., /data/workspaces/spec-tasks/{id}) - for Docker wrapper hacks
# 2. /home/retro/work - so agent tools see a real directory (not a symlink)
# This eliminates symlink confusion where tools resolve symlinks and get confused.
# See: design/2025-12-01-hydra-bind-mount-symlink.md
# CRITICAL: No fallbacks - these mounts MUST exist or fail fast
if [ -z "$WORKSPACE_DIR" ]; then
    echo "FATAL: WORKSPACE_DIR environment variable not set"
    exit 1
fi
if [ ! -d "$WORKSPACE_DIR" ]; then
    echo "FATAL: WORKSPACE_DIR does not exist: $WORKSPACE_DIR"
    exit 1
fi
if [ ! -d /home/retro/work ]; then
    echo "FATAL: /home/retro/work bind mount not present - Wolf executor must mount workspace at both paths"
    exit 1
fi
# Ensure correct ownership on both workspace paths (same underlying directory)
sudo chown retro:retro "$WORKSPACE_DIR"
sudo chown retro:retro /home/retro/work
echo "Workspace mounted at both $WORKSPACE_DIR and /home/retro/work"

# CRITICAL: Create Zed config symlinks BEFORE Sway starts
# Settings-sync-daemon (started by Sway) needs the symlink to exist
WORK_DIR=/home/retro/work
ZED_STATE_DIR=$WORK_DIR/.zed-state

# Ensure we're in the workspace (works with both symlink and direct mount)
cd /home/retro/work

# Create persistent state directory structure
mkdir -p $ZED_STATE_DIR/config
mkdir -p $ZED_STATE_DIR/local-share
mkdir -p $ZED_STATE_DIR/cache

# Create symlinks BEFORE Sway starts (so settings-sync-daemon can write immediately)
rm -rf ~/.config/zed
mkdir -p ~/.config
ln -sf $ZED_STATE_DIR/config ~/.config/zed

rm -rf ~/.local/share/zed
mkdir -p ~/.local/share
ln -sf $ZED_STATE_DIR/local-share ~/.local/share/zed

rm -rf ~/.cache/zed
mkdir -p ~/.cache
ln -sf $ZED_STATE_DIR/cache ~/.cache/zed

echo "✅ Zed state symlinks created (settings-sync-daemon can write immediately)"

# Configure fontconfig for grayscale antialiasing (affects all apps including Zed)
# RGB subpixel rendering looks bad when desktop is scaled via streaming
mkdir -p ~/.config/fontconfig
cat > ~/.config/fontconfig/fonts.conf << 'FONTCONFIG_EOF'
<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "fonts.dtd">
<fontconfig>
  <!-- Use grayscale antialiasing instead of subpixel (RGB) -->
  <!-- Subpixel rendering breaks when desktop is scaled via streaming -->
  <match target="font">
    <edit name="rgba" mode="assign"><const>none</const></edit>
    <edit name="antialias" mode="assign"><bool>true</bool></edit>
    <edit name="hinting" mode="assign"><bool>true</bool></edit>
    <edit name="hintstyle" mode="assign"><const>hintslight</const></edit>
  </match>
</fontconfig>
FONTCONFIG_EOF
echo "✅ Fontconfig set to grayscale antialiasing"

# Configure Qwen Code session persistence
# Qwen stores sessions at $QWEN_DATA_DIR/projects/<project_hash>/chats/
# By setting QWEN_DATA_DIR to workspace, sessions persist across container restarts
export QWEN_DATA_DIR=$WORK_DIR/.qwen-state
mkdir -p $QWEN_DATA_DIR
# Also create symlink for backwards compatibility and any tools that look at ~/.qwen
rm -rf ~/.qwen
ln -sf $QWEN_DATA_DIR ~/.qwen
echo "✅ Qwen data directory set to persistent storage: QWEN_DATA_DIR=$QWEN_DATA_DIR"

# Copy Sway user guide to workspace (if not already present)
if [ -f /cfg/sway/SWAY-USER-GUIDE.md ] && [ ! -f $WORK_DIR/SWAY-USER-GUIDE.md ]; then
    cp /cfg/sway/SWAY-USER-GUIDE.md $WORK_DIR/SWAY-USER-GUIDE.md
    echo "✅ Sway user guide copied to workspace (see SWAY-USER-GUIDE.md for keyboard shortcuts)"
fi

# Start RevDial client for reverse proxy (screenshot server, clipboard, git HTTP)
# CRITICAL: Starts BEFORE Sway so API can reach sandbox immediately
# Uses user's API token for authentication (session-scoped, user-owned)
if [ -n "$HELIX_API_BASE_URL" ] && [ -n "$HELIX_SESSION_ID" ] && [ -n "$USER_API_TOKEN" ]; then
    REVDIAL_SERVER="${HELIX_API_BASE_URL}/api/v1/revdial"
    RUNNER_ID="sandbox-${HELIX_SESSION_ID}"

    echo "Starting RevDial client for API ↔ sandbox communication..."
    /usr/local/bin/revdial-client \
        -server "$REVDIAL_SERVER" \
        -runner-id "$RUNNER_ID" \
        -token "$USER_API_TOKEN" \
        -local "localhost:9876" \
        >> /tmp/revdial-client.log 2>&1 &

    REVDIAL_PID=$!
    echo "✅ RevDial client started (PID: $REVDIAL_PID) - API can now reach this sandbox"
else
    echo "⚠️  RevDial client not started (missing HELIX_API_BASE_URL, HELIX_SESSION_ID, or USER_API_TOKEN)"
fi

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

    # Create custom waybar CSS for better workspace visibility
    cat > $HOME/.config/waybar/style.css << 'WAYBAR_CSS'
/* Helix custom waybar styling */
* {
    font-family: "Ubuntu", "Font Awesome 6 Free", sans-serif;
    font-size: 14px;
}

window#waybar {
    background-color: rgba(30, 30, 40, 0.95);
    color: #ffffff;
}

/* Workspace buttons - always visible, clickable */
#workspaces button {
    padding: 0 8px;
    margin: 2px 2px;
    background-color: #404050;
    color: #888888;
    border-radius: 4px;
    border: 1px solid #555555;
    min-width: 30px;
}

#workspaces button:hover {
    background-color: #505060;
    color: #ffffff;
}

#workspaces button.focused {
    background-color: #7c3aed;
    color: #ffffff;
    border: 1px solid #a855f7;
}

#workspaces button.urgent {
    background-color: #dc2626;
    color: #ffffff;
}

/* Has windows indicator */
#workspaces button.visible {
    background-color: #505060;
    color: #cccccc;
}

/* Separator between sections */
#custom-separator, #custom-separator2 {
    color: #555555;
    padding: 0 4px;
}

/* App launcher icons */
#custom-firefox, #custom-kitty {
    padding: 0 8px;
    font-size: 16px;
}

#custom-firefox:hover, #custom-kitty:hover {
    background-color: #404050;
    border-radius: 4px;
}

/* Keyboard layout flags */
#custom-keyboard-us, #custom-keyboard-gb, #custom-keyboard-fr {
    padding: 0 6px;
    font-size: 14px;
}

#custom-keyboard-us:hover, #custom-keyboard-gb:hover, #custom-keyboard-fr:hover {
    background-color: #404050;
    border-radius: 4px;
}

/* System info */
#cpu, #memory, #temperature, #pulseaudio, #network {
    padding: 0 8px;
    color: #aaaaaa;
}

#custom-clock {
    padding: 0 10px;
    color: #ffffff;
}
WAYBAR_CSS

    # Configure GTK applications: dark mode + grayscale antialiasing
    # (RGB subpixel rendering looks bad when desktop is scaled via streaming)
    mkdir -p $HOME/.config/gtk-3.0
    cat > $HOME/.config/gtk-3.0/settings.ini << 'GTK_EOF'
[Settings]
gtk-application-prefer-dark-theme=1
gtk-xft-antialias=1
gtk-xft-hinting=1
gtk-xft-hintstyle=hintslight
gtk-xft-rgba=none
GTK_EOF

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
    "custom/separator",
    "custom/firefox",
    "custom/kitty",
    "custom/separator2",
    "custom/keyboard-us",
    "custom/keyboard-gb",
    "custom/keyboard-fr"
  ],
  "sway/workspaces": {
    "disable-scroll": true,
    "all-outputs": true,
    "format": " {name} ",
    "persistent-workspaces": {
      "*": [1, 2, 3, 4]
    }
  },
  "custom/separator": {
    "format": "|",
    "tooltip": false
  },
  "custom/separator2": {
    "format": "|",
    "tooltip": false
  },
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
  "custom/kitty": {
    "format": "🐱",
    "tooltip": true,
    "tooltip-format": "Kitty Terminal",
    "on-click": "kitty"
  },
  "custom/keyboard-us": {
    "format": "🇺🇸",
    "tooltip": true,
    "tooltip-format": "Switch to US keyboard layout",
    "on-click": "swaymsg input type:keyboard xkb_switch_layout 0"
  },
  "custom/keyboard-gb": {
    "format": "🇬🇧",
    "tooltip": true,
    "tooltip-format": "Switch to UK keyboard layout",
    "on-click": "swaymsg input type:keyboard xkb_switch_layout 1"
  },
  "custom/keyboard-fr": {
    "format": "🇫🇷",
    "tooltip": true,
    "tooltip-format": "Switch to French keyboard layout",
    "on-click": "swaymsg input type:keyboard xkb_switch_layout 2"
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
  "sway/language": {
    "format": "{short}",
    "tooltip": true,
    "tooltip-format": "Click to switch keyboard layout (current: {long})",
    "on-click": "swaymsg input type:keyboard xkb_switch_layout next"
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

    # CRITICAL: Replace modifier key BEFORE bindings are processed
    # GOW base config has "set $mod Mod4" at the top - we must change it in-place
    # Super/Cmd key is captured by macOS/browsers, Alt passes through reliably
    sed -i 's/set \$mod Mod4/set $mod Mod1/' $HOME/.config/sway/config
    echo "[Sway] Changed modifier key from Super (Mod4) to Alt (Mod1)"

    # Copy our custom Helix configuration (included by GOW base config on line 2)
    cp /cfg/sway/custom-cfg $HOME/.config/sway/custom-cfg

    # Add our custom Helix configuration
    echo "" >> $HOME/.config/sway/config
    echo "# Helix Desktop custom configuration" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    echo "# Disable Xwayland - force native Wayland (fixes Zed input issues)" >> $HOME/.config/sway/config
    echo "xwayland disable" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    echo "# Set Helix wallpaper" >> $HOME/.config/sway/config
    echo "output * bg /usr/share/backgrounds/helix-logo.png fill" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config

    # Calculate display scale from HELIX_ZOOM_LEVEL (default: 100%)
    # Unlike GNOME's X11 stack, Sway/Wayland properly handles fractional scaling
    ZOOM_LEVEL=${HELIX_ZOOM_LEVEL:-100}
    SWAY_SCALE=$(echo "scale=2; $ZOOM_LEVEL / 100" | bc)
    # Ensure minimum scale of 1.0
    if [ "$(echo "$SWAY_SCALE < 1" | bc)" -eq 1 ]; then
        SWAY_SCALE="1"
    fi
    echo "# Configure display scaling (HELIX_ZOOM_LEVEL=${ZOOM_LEVEL}%)" >> $HOME/.config/sway/config
    echo "output WL-1 scale $SWAY_SCALE" >> $HOME/.config/sway/config
    echo "[Sway] Display scale set to $SWAY_SCALE (from HELIX_ZOOM_LEVEL=${ZOOM_LEVEL}%)"
    echo "" >> $HOME/.config/sway/config
    echo "# Keyboard configuration: multiple layouts, Caps Lock as Ctrl" >> $HOME/.config/sway/config
    echo "# Use the flag buttons in waybar to switch layouts (Alt+Shift toggle disabled - causes issues with Moonlight)" >> $HOME/.config/sway/config
    echo "input type:keyboard {" >> $HOME/.config/sway/config
    echo "    xkb_layout \"us,gb,fr\"" >> $HOME/.config/sway/config
    echo "    xkb_options \"caps:ctrl_nocaps\"" >> $HOME/.config/sway/config
    echo "}" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    echo "# Workaround for Moonlight keyboard modifier state desync bug" >> $HOME/.config/sway/config
    echo "# Press Super+Escape to reset all modifier keys if they get stuck" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Escape exec swaymsg 'input type:keyboard xkb_switch_layout 0'" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    echo "# Additional key bindings for our tools" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Shift+Return exec kitty" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Shift+f exec firefox" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    echo "# Start screenshot server and settings-sync daemon after Sway is ready (wayland-1 available)" >> $HOME/.config/sway/config
    echo "exec WAYLAND_DISPLAY=wayland-1 /usr/local/bin/screenshot-server > /tmp/screenshot-server.log 2>&1" >> $HOME/.config/sway/config
    # Pass required environment variables to settings-sync-daemon
    echo "exec env HELIX_SESSION_ID=\$HELIX_SESSION_ID HELIX_API_URL=\$HELIX_API_URL HELIX_API_TOKEN=\$HELIX_API_TOKEN /usr/local/bin/settings-sync-daemon > /tmp/settings-sync.log 2>&1" >> $HOME/.config/sway/config

    # Add resolution and app launch (like the original launcher)
    echo "output * resolution ${GAMESCOPE_WIDTH}x${GAMESCOPE_HEIGHT} position 0,0" >> $HOME/.config/sway/config
    echo "workspace main; exec $@" >> $HOME/.config/sway/config

    # DISABLED: Do not kill Sway on app exit - Zed has auto-restart loop
    # Desktop/Spec Task/Exploratory sessions need persistent Sway compositor for reconnection
    # if [ "$SWAY_STOP_ON_APP_EXIT" == "yes" ]; then
    #   echo -n " && killall sway" >> $HOME/.config/sway/config
    # fi

    # Start sway
    dbus-run-session -- sway --unsupported-gpu
  else
    echo "[exec] Starting: $@"
    exec $@
  fi
}

custom_launcher /usr/local/bin/start-zed-helix.sh
