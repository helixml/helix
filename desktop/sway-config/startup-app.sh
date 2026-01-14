#!/bin/bash
# GOW base-app startup script for Helix Desktop (Sway)
# Used for: Desktop sessions, Spec Task sessions, and Exploratory sessions
set -e

echo "Starting Helix Desktop (Sway)..."

# NOTE: Telemetry firewall is configured in the sandbox container,
# not inside agent containers. This provides centralized monitoring across all agents.

# Create symlink to Zed binary if not exists
if [ -f /zed-build/zed ] && [ ! -f /usr/local/bin/zed ]; then
    sudo ln -sf /zed-build/zed /usr/local/bin/zed
    echo "Created symlink: /usr/local/bin/zed -> /zed-build/zed"
fi

# Workspace setup: Container executor mounts workspace at BOTH paths via bind mount:
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
    echo "FATAL: /home/retro/work bind mount not present - container executor must mount workspace at both paths"
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

# Configure xdg-desktop-portal-wlr for headless operation (no chooser prompt)
# This allows screen capture without user interaction
mkdir -p ~/.config/xdg-desktop-portal-wlr
cat > ~/.config/xdg-desktop-portal-wlr/config << 'PORTAL_EOF'
[screencast]
# Skip the output chooser dialog - use first available output
chooser_type=none
PORTAL_EOF
echo "✅ xdg-desktop-portal-wlr configured for headless screen capture"

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

# Note: RevDial client is now integrated into desktop-bridge
# It starts automatically when desktop-bridge launches with the required env vars

# Start desktop-bridge (screenshot, MCP, RevDial)
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
    # Run Sway as a headless compositor (no outer display needed)
    # This creates a virtual display that applications connect to.
    # xdg-desktop-portal-wlr captures via wlr-screencopy → PipeWire → our GStreamer pipeline
    export WLR_BACKENDS=headless
    # Suppress libinput errors about missing physical input devices
    export WLR_LIBINPUT_NO_DEVICES=1

    # Create waybar config directory (config files generated inline below)
    mkdir -p $HOME/.config/waybar

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

    # Define modifier key (Alt instead of Super - Super/Cmd is captured by macOS/browsers)
    # This MUST come before any bindsym commands
    echo "" >> $HOME/.config/sway/config
    echo "# Use Alt as modifier key (Super/Cmd doesn't work reliably in browser streaming)" >> $HOME/.config/sway/config
    echo "set \$mod Mod1" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    echo "[Sway] Set modifier key to Alt (Mod1)"

    # Calculate display scale from HELIX_ZOOM_LEVEL (default: 100%)
    # Unlike GNOME's X11 stack, Sway/Wayland properly handles fractional scaling
    ZOOM_LEVEL=${HELIX_ZOOM_LEVEL:-100}
    SWAY_SCALE=$(echo "scale=2; $ZOOM_LEVEL / 100" | bc)
    # Ensure minimum scale of 1.0
    if [ "$(echo "$SWAY_SCALE < 1" | bc)" -eq 1 ]; then
        SWAY_SCALE="1"
    fi
    echo "# Configure display scaling (HELIX_ZOOM_LEVEL=${ZOOM_LEVEL}%)" >> $HOME/.config/sway/config
    echo "# HEADLESS-1 is the output name when running with WLR_BACKENDS=headless" >> $HOME/.config/sway/config
    echo "output HEADLESS-1 scale $SWAY_SCALE" >> $HOME/.config/sway/config
    echo "[Sway] Display scale set to $SWAY_SCALE (from HELIX_ZOOM_LEVEL=${ZOOM_LEVEL}%)"
    echo "" >> $HOME/.config/sway/config

    # Key bindings (now that $mod is defined)
    echo "# Workaround for Moonlight keyboard modifier state desync bug" >> $HOME/.config/sway/config
    echo "# Press Super+Escape to reset all modifier keys if they get stuck" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Escape exec swaymsg 'input type:keyboard xkb_switch_layout 0'" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    echo "# Additional key bindings for our tools" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Shift+Return exec kitty" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Shift+f exec firefox" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config

    # =====================================================================
    # Workspace bindings - CRITICAL for multi-desktop workflow
    # =====================================================================
    echo "# Workspace switching (Alt+1/2/3/4)" >> $HOME/.config/sway/config
    echo "bindsym \$mod+1 workspace number 1" >> $HOME/.config/sway/config
    echo "bindsym \$mod+2 workspace number 2" >> $HOME/.config/sway/config
    echo "bindsym \$mod+3 workspace number 3" >> $HOME/.config/sway/config
    echo "bindsym \$mod+4 workspace number 4" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    echo "# Move windows to workspaces (Alt+Shift+1/2/3/4)" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Shift+1 move container to workspace number 1" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Shift+2 move container to workspace number 2" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Shift+3 move container to workspace number 3" >> $HOME/.config/sway/config
    echo "bindsym \$mod+Shift+4 move container to workspace number 4" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config

    # =====================================================================
    # Auto-assign applications to workspaces
    # =====================================================================
    # Zed on workspace 1 (app_id from our dev build is "dev.zed.Zed-Dev")
    echo "# Auto-assign applications to workspaces" >> $HOME/.config/sway/config
    echo "assign [app_id=\"dev.zed.Zed-Dev\"] workspace number 1" >> $HOME/.config/sway/config
    echo "assign [class=\"Zed\"] workspace number 1" >> $HOME/.config/sway/config
    # Terminals on workspace 2
    echo "assign [app_id=\"kitty\"] workspace number 2" >> $HOME/.config/sway/config
    echo "assign [app_id=\"ghostty\"] workspace number 2" >> $HOME/.config/sway/config
    echo "assign [app_id=\"foot\"] workspace number 2" >> $HOME/.config/sway/config
    # Debug terminals on workspace 2 (ACP log viewer launched with --class)
    echo "assign [app_id=\"acp-log-viewer\"] workspace number 2" >> $HOME/.config/sway/config
    # Firefox on workspace 3
    echo "assign [app_id=\"firefox\"] workspace number 3" >> $HOME/.config/sway/config
    echo "assign [class=\"firefox\"] workspace number 3" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config

    # =====================================================================
    # Window appearance - clean borderless look
    # =====================================================================
    echo "# Remove window borders and title bars for clean look" >> $HOME/.config/sway/config
    echo "default_border none" >> $HOME/.config/sway/config
    echo "default_floating_border none" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config

    # =====================================================================
    # Status bar - use waybar instead of default swaybar
    # =====================================================================
    echo "# Hide default swaybar (we use waybar instead)" >> $HOME/.config/sway/config
    echo "bar {" >> $HOME/.config/sway/config
    echo "    mode invisible" >> $HOME/.config/sway/config
    echo "}" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config

    # NOTE: PipeWire and XDG portals are started manually in sway-session.sh
    # to ensure proper D-Bus integration and startup ordering.
    # Only waybar and ydotoold are started via Sway exec.
    echo "# Start ydotoold for input injection (ydotool daemon)" >> $HOME/.config/sway/config
    echo "exec ydotoold > /tmp/ydotoold.log 2>&1" >> $HOME/.config/sway/config
    echo "# Start waybar status bar" >> $HOME/.config/sway/config
    echo "exec waybar > /tmp/waybar.log 2>&1" >> $HOME/.config/sway/config
    echo "" >> $HOME/.config/sway/config
    # Configure headless output resolution (using GAMESCOPE_WIDTH/HEIGHT like Ubuntu desktop)
    echo "# Headless output resolution: ${GAMESCOPE_WIDTH}x${GAMESCOPE_HEIGHT}@${GAMESCOPE_REFRESH}Hz" >> $HOME/.config/sway/config
    echo "output HEADLESS-1 resolution ${GAMESCOPE_WIDTH}x${GAMESCOPE_HEIGHT}@${GAMESCOPE_REFRESH}Hz position 0,0" >> $HOME/.config/sway/config
    echo "workspace number 1; exec $@" >> $HOME/.config/sway/config

    # Create sway-session script that runs inside dbus-run-session
    # This is the same pattern Ubuntu uses for GNOME
    cat > $XDG_RUNTIME_DIR/sway-session.sh << 'SWAY_SESSION_EOF'
#!/bin/bash
echo "[sway-session] Starting inside D-Bus session..."
echo "[sway-session] DBUS_SESSION_BUS_ADDRESS=$DBUS_SESSION_BUS_ADDRESS"

# Write D-Bus session env to file for init scripts to source
DBUS_ENV_FILE="${XDG_RUNTIME_DIR}/dbus-session.env"
echo "export DBUS_SESSION_BUS_ADDRESS='$DBUS_SESSION_BUS_ADDRESS'" > "$DBUS_ENV_FILE"
echo "[sway-session] D-Bus env written to $DBUS_ENV_FILE"

# Enable core dumps for crash debugging
# Core dumps are saved to /tmp/cores/ for later analysis
mkdir -p /tmp/cores
ulimit -c unlimited
echo "[sway-session] Core dumps enabled (ulimit -c unlimited)"

# Start PipeWire BEFORE Sway (needed for screen capture)
echo "[sway-session] Starting PipeWire..."
pipewire > /tmp/pipewire.log 2>&1 &
PIPEWIRE_PID=$!
sleep 0.3
pipewire-pulse > /tmp/pipewire-pulse.log 2>&1 &
sleep 0.3

# Sway crash recovery loop
# If Sway crashes (bus error, segfault, etc.), we capture the core dump and restart
# This prevents permanent video stream failure from transient GPU/driver issues
MAX_RESTARTS=3
RESTART_COUNT=0
SERVICES_STARTED=false

while [ $RESTART_COUNT -lt $MAX_RESTARTS ]; do
    # Start sway in background
    echo "[sway-session] Starting Sway (attempt $((RESTART_COUNT + 1))/$MAX_RESTARTS)..."
    sway --unsupported-gpu &
    SWAY_PID=$!
    echo "[sway-session] Sway started with PID $SWAY_PID"

    # Wait for Wayland socket before starting portals (only on first start)
    echo "[sway-session] Waiting for Wayland socket..."
    SOCKET_READY=false
    for i in $(seq 1 30); do
        if [ -S "${XDG_RUNTIME_DIR}/wayland-1" ]; then
            echo "[sway-session] Wayland socket ready"
            SOCKET_READY=true
            break
        fi
        # Check if Sway already crashed during startup
        if ! kill -0 $SWAY_PID 2>/dev/null; then
            echo "[sway-session] Sway crashed during startup!"
            break
        fi
        sleep 0.5
    done

    # Start services only on first successful start
    if [ "$SOCKET_READY" = "true" ] && [ "$SERVICES_STARTED" = "false" ]; then
        # Start XDG portals in background - NO WAITING NEEDED
        # Our video capture uses ext-image-copy-capture (Sway 1.10+) or wlr-screencopy,
        # which are native Wayland protocols that bypass the portal entirely.
        # Portals are only needed for file dialogs, etc. - not critical path.
        echo "[sway-session] Starting XDG portals in background (not needed for video)..."
        WAYLAND_DISPLAY=wayland-1 /usr/libexec/xdg-desktop-portal-wlr > /tmp/portal-wlr.log 2>&1 &
        /usr/libexec/xdg-desktop-portal > /tmp/portal.log 2>&1 &
        echo "[sway-session] Portals started (non-blocking)"

        # Start services via shared init scripts (they wait for Wayland socket internally)
        # Logs are prefixed and go to docker logs (stdout)
        /usr/local/bin/start-desktop-bridge.sh &
        /usr/local/bin/start-settings-sync-daemon.sh &
        SERVICES_STARTED=true
    fi

    # Wait for sway to exit
    wait $SWAY_PID
    EXIT_CODE=$?

    # Check exit status
    # 0 = normal exit (user closed sway)
    # 128+N = killed by signal N (e.g., 128+7=135 for SIGBUS, 128+11=139 for SIGSEGV)
    if [ $EXIT_CODE -eq 0 ]; then
        echo "[sway-session] Sway exited normally (code 0)"
        break
    fi

    # Sway crashed - determine signal
    if [ $EXIT_CODE -gt 128 ]; then
        SIGNAL=$((EXIT_CODE - 128))
        case $SIGNAL in
            7)  SIGNAL_NAME="SIGBUS (bus error)" ;;
            11) SIGNAL_NAME="SIGSEGV (segmentation fault)" ;;
            6)  SIGNAL_NAME="SIGABRT (abort)" ;;
            *)  SIGNAL_NAME="signal $SIGNAL" ;;
        esac
        echo "[sway-session] ERROR: Sway crashed with $SIGNAL_NAME (exit code $EXIT_CODE)"
    else
        echo "[sway-session] ERROR: Sway exited with error code $EXIT_CODE"
    fi

    # Look for core dump (may be in current dir or /tmp/cores)
    CORE_FILE=""
    for pattern in "core" "core.$SWAY_PID" "/tmp/cores/core" "/tmp/cores/core.$SWAY_PID"; do
        if [ -f "$pattern" ]; then
            CORE_FILE="$pattern"
            break
        fi
    done

    if [ -n "$CORE_FILE" ]; then
        # Move core dump to timestamped file for preservation
        CRASH_TIME=$(date +%Y%m%d-%H%M%S)
        SAVED_CORE="/tmp/cores/sway-crash-${CRASH_TIME}.core"
        mv "$CORE_FILE" "$SAVED_CORE" 2>/dev/null || cp "$CORE_FILE" "$SAVED_CORE"
        echo "[sway-session] Core dump saved to: $SAVED_CORE"

        # Try to get basic backtrace if gdb is available
        if command -v gdb >/dev/null 2>&1 && [ -f "$SAVED_CORE" ]; then
            echo "[sway-session] Attempting to extract backtrace..."
            gdb -batch -ex "bt" -ex "quit" /usr/bin/sway "$SAVED_CORE" 2>/dev/null | head -50 > "/tmp/cores/sway-crash-${CRASH_TIME}.bt" || true
            if [ -s "/tmp/cores/sway-crash-${CRASH_TIME}.bt" ]; then
                echo "[sway-session] Backtrace saved to: /tmp/cores/sway-crash-${CRASH_TIME}.bt"
                cat "/tmp/cores/sway-crash-${CRASH_TIME}.bt"
            fi
        fi
    else
        echo "[sway-session] No core dump found (core_pattern may need configuration)"
    fi

    RESTART_COUNT=$((RESTART_COUNT + 1))

    if [ $RESTART_COUNT -lt $MAX_RESTARTS ]; then
        echo "[sway-session] Restarting Sway in 2 seconds..."
        sleep 2
        # Clean up stale Wayland socket if present
        rm -f "${XDG_RUNTIME_DIR}/wayland-1" 2>/dev/null || true
    else
        echo "[sway-session] FATAL: Sway crashed $MAX_RESTARTS times, giving up"
        echo "[sway-session] Check /tmp/cores/ for crash dumps"
    fi
done

echo "[sway-session] Session ended (restart_count=$RESTART_COUNT, exit_code=$EXIT_CODE)"
SWAY_SESSION_EOF
    chmod +x $XDG_RUNTIME_DIR/sway-session.sh

    # Start D-Bus session using dbus-run-session (same pattern as Ubuntu/GNOME)
    # This properly sets up DBUS_SESSION_BUS_ADDRESS for all child processes
    echo "[startup] Starting Sway inside D-Bus session (via dbus-run-session)..."
    dbus-run-session -- $XDG_RUNTIME_DIR/sway-session.sh
  else
    echo "[exec] Starting: $@"
    exec $@
  fi
}

custom_launcher /usr/local/bin/start-zed-helix.sh
