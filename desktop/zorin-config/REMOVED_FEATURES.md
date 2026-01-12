# Removed Features from startup-app.sh - Stage 1.5 Minimal Baseline

This document tracks what was removed from startup-app.sh to create a minimal baseline, and how to add features back incrementally.

## Current State: Stage 1.5 Minimal Baseline

**What's INCLUDED**:
- ✅ Basic environment setup and debug logging
- ✅ Zed binary symlink creation
- ✅ Zed state directory symlinks (for persistence)
- ✅ Launch GNOME via GOW's xorg.sh

**What's REMOVED** (to be added back incrementally):
- ❌ GNOME autostart directory creation
- ❌ GNOME settings customization (apply-gnome-settings.sh)
- ❌ screenshot-server autostart entry
- ❌ settings-sync-daemon autostart entry
- ❌ Zed autostart entry (start-zed-helix.sh)

## Incremental Re-Addition Plan

### Step 1: Add screenshot-server Autostart (Test Screenshots)

Add this before the final `exec /opt/gow/xorg.sh`:

```bash
# Create GNOME autostart directory
mkdir -p ~/.config/autostart

# Create autostart entry for screenshot server
cat > ~/.config/autostart/screenshot-server.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Screenshot Server
Exec=/usr/local/bin/screenshot-server
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=3
NoDisplay=true
EOF

echo "✅ screenshot-server autostart entry created"
```

**Test**: Verify screenshots work in API, no 500 errors

### Step 2: Add settings-sync-daemon Autostart (Test Config Sync)

Add this after Step 1:

```bash
# Create autostart entry for settings-sync-daemon
# Pass environment variables via script wrapper
cat > /tmp/start-settings-sync-daemon.sh <<EOF
#!/bin/bash
exec env HELIX_SESSION_ID="$HELIX_SESSION_ID" HELIX_API_URL="$HELIX_API_URL" HELIX_API_TOKEN="$HELIX_API_TOKEN" /usr/local/bin/settings-sync-daemon > /tmp/settings-sync.log 2>&1
EOF
sudo mv /tmp/start-settings-sync-daemon.sh /usr/local/bin/start-settings-sync-daemon.sh
sudo chmod +x /usr/local/bin/start-settings-sync-daemon.sh

cat > ~/.config/autostart/settings-sync-daemon.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Settings Sync Daemon
Exec=/usr/local/bin/start-settings-sync-daemon.sh
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=3
NoDisplay=true
EOF

echo "✅ settings-sync-daemon autostart entry created"
```

**Test**: Verify settings.json is created with default_model

### Step 3: Add Zed Autostart (Test Auto-Launch)

Add this after Step 2:

```bash
# Create autostart entry for Zed (starts after settings are ready)
cat > ~/.config/autostart/zed-helix.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Zed Helix Editor
Exec=/usr/local/bin/start-zed-helix.sh
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=5
NoDisplay=false
Icon=zed
EOF

echo "✅ Zed autostart entry created"
```

**Test**: Verify Zed launches automatically, WebSocket connects

### Step 4: Add GNOME Settings Customization (Test Visual Polish)

Add this after Step 2 (before Zed):

```bash
# Create autostart entry for applying GNOME settings
cat > ~/.config/autostart/helix-gnome-settings.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Helix GNOME Settings
Exec=/usr/local/bin/apply-gnome-settings.sh
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=2
NoDisplay=true
EOF

# Create the settings application script
cat > /tmp/apply-gnome-settings.sh <<'EOF'
#!/bin/bash
# Apply GNOME settings after D-Bus is available

echo "Applying Helix GNOME settings..."

# Load dconf settings from config file
if [ -f /cfg/gnome/dconf-settings.ini ]; then
    dconf load / < /cfg/gnome/dconf-settings.ini
fi

# Set Helix wallpaper
gsettings set org.gnome.desktop.background picture-uri "file:///usr/share/backgrounds/helix-logo.png"
gsettings set org.gnome.desktop.background picture-uri-dark "file:///usr/share/backgrounds/helix-logo.png"

# Configure dark theme
gsettings set org.gnome.desktop.interface gtk-theme "Adwaita-dark"
gsettings set org.gnome.desktop.interface color-scheme "prefer-dark"

# Disable Activities overview (single-app focus mode)
gsettings set org.gnome.shell disable-user-extensions true

echo "✅ GNOME settings applied successfully"
EOF

sudo mv /tmp/apply-gnome-settings.sh /usr/local/bin/apply-gnome-settings.sh
sudo chmod +x /usr/local/bin/apply-gnome-settings.sh

echo "✅ GNOME settings autostart entry created"
```

**Test**: Verify dark theme, Helix logo, no screen lock

## Testing Strategy

After each step:
1. Rebuild image: `./stack build-zorin`
2. Restart Wolf: `docker compose -f docker-compose.dev.yaml down wolf && docker compose -f docker-compose.dev.yaml up -d wolf`
3. Create NEW session and test
4. Check logs: `docker logs <container-id>`
5. Verify feature works before proceeding to next step

## Notes

- Current minimal baseline should work like Stage 1 (just GNOME desktop)
- Each incremental addition tests ONE feature
- If any step breaks, we know exactly which feature caused the problem
- The binaries are already in the Docker image, we're just controlling when they launch
