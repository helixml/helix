# Fix: Firefox Auto-Open in Ubuntu Desktop

**Date:** 2025-12-08
**Status:** Implemented (2026-01-26)

## Problem

When launching the Ubuntu desktop environment, Firefox doesn't open automatically, even though the project's `.helix/startup.sh` script calls `xdg-open http://localhost:3000`.

Additionally, clicking URLs in Zed IDE or agent output fails silently because Zed uses `xdg-open` to open URLs, which requires a default browser to be configured.

**Expected behavior (works in Sway):**
- Terminal window opens (with startup script)
- Zed editor opens
- Firefox opens automatically with the dev server URL

**Actual behavior (Ubuntu):**
- Terminal window opens (startup script runs)
- Zed editor opens
- Firefox does NOT open

## Root Cause

Ubuntu GNOME requires explicit MIME handler configuration for `xdg-open` to know which browser to use for HTTP URLs.

In Sway (Wayland), `xdg-open` falls back to checking available browsers directly via the `$BROWSER` environment variable or by scanning for known browsers. Firefox being installed is sufficient.

In GNOME, `xdg-open` uses the XDG MIME handler system, which requires Firefox to be registered as the default handler for `x-scheme-handler/http` and `x-scheme-handler/https`.

## Solution

Add `xdg-mime` commands to `wolf/ubuntu-config/startup-app.sh` before GNOME session starts.

### File to Modify

`wolf/ubuntu-config/startup-app.sh`

### Location

Around line 376, after `unset WAYLAND_DISPLAY` and before the "Launch Xwayland and GNOME session" comment.

### Code to Add

```bash
# Set Firefox as default browser for xdg-open to work with HTTP/HTTPS URLs
# This is needed because GNOME requires explicit default handler configuration
# Without this, the project startup script's `xdg-open http://localhost:3000` fails silently
xdg-mime default firefox.desktop x-scheme-handler/http
xdg-mime default firefox.desktop x-scheme-handler/https
xdg-mime default firefox.desktop text/html
echo "Firefox set as default browser for HTTP/HTTPS URLs"
```

### Why This Approach

- `xdg-mime` directly updates `~/.config/mimeapps.list` which is the XDG standard
- This works for both GNOME apps and command-line `xdg-open`
- dconf browser settings only affect GNOME-specific apps, not `xdg-open`

## Testing

After making the change:

1. Rebuild the Ubuntu desktop image:
   ```bash
   ./stack build-ubuntu
   ```

2. Launch an Ubuntu desktop environment session

3. Verify that Firefox opens automatically with the dev server URL (http://localhost:3000)

## Background Context

### How Startup Works

Both Sway and Ubuntu desktops follow this flow:

1. Desktop session starts
2. `start-zed-helix.sh` runs (via autostart in Ubuntu, via `custom_launcher` in Sway)
3. Repositories are cloned
4. If `.helix/startup.sh` exists in the primary repo, it runs in a terminal window
5. The startup script typically runs `npm run dev` and calls `xdg-open http://localhost:3000`
6. Zed editor starts

### User's Startup Script

The user's project startup script (`.helix/startup.sh`):

```bash
#!/bin/bash
set -euo pipefail

# ... setup code ...

echo "Starting dev server in background..."
nohup npm run dev > /tmp/dev-server.log 2>&1 &

sleep 5

# Open browser - this is the line that fails in Ubuntu
if command -v xdg-open &> /dev/null; then
    xdg-open http://localhost:3000 > /dev/null 2>&1 &
fi
```

The `xdg-open` call succeeds in Sway but silently fails in Ubuntu GNOME because no default handler is configured.

## Related Files

- `desktop/ubuntu-config/startup-app.sh` - Ubuntu startup script (modified)
- `desktop/sway-config/startup-app.sh` - Sway startup script (modified)
- `desktop/ubuntu-config/start-zed-helix.sh` - Launches the terminal with startup script
- `desktop/ubuntu-config/dconf-settings.ini` - GNOME settings (no changes needed)

## Implementation (2026-01-26)

Added `xdg-mime` configuration to both desktop startup scripts:

**Ubuntu (`desktop/ubuntu-config/startup-app.sh`):**
```bash
xdg-mime default firefox.desktop x-scheme-handler/http
xdg-mime default firefox.desktop x-scheme-handler/https
xdg-mime default firefox.desktop text/html
```

**Sway (`desktop/sway-config/startup-app.sh`):**
```bash
export BROWSER=firefox
xdg-mime default firefox.desktop x-scheme-handler/http 2>/dev/null || true
xdg-mime default firefox.desktop x-scheme-handler/https 2>/dev/null || true
xdg-mime default firefox.desktop text/html 2>/dev/null || true
```

This enables:
1. URLs clicked in Zed IDE or agent output to open in Firefox
2. Startup scripts using `xdg-open` to launch Firefox with URLs
3. Consistent URL handling across both Ubuntu and Sway desktops
