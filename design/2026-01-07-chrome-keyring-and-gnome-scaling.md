# Design Doc: Chrome Keyring Fix and GNOME Display Scaling

**Date:** 2026-01-07
**Status:** Complete
**Author:** Claude

## Problem Statement

Two issues affecting Ubuntu desktop containers:

1. **Chrome Keyring Prompt**: When Chrome starts, it prompts to "Choose password for new keyring" because GNOME Keyring isn't pre-configured. This blocks automated workflows.

2. **Display Scaling Not Working**: Users configure 200% or 300% scaling in the agent configuration, but the scaling doesn't apply correctly to the GNOME headless desktop.

## Root Cause Analysis

### Chrome Keyring Issue

Chrome uses the system keyring (GNOME Keyring) to store passwords by default. In container environments without a pre-initialized keyring, Chrome prompts to create one. Additionally, Chrome shows first-run dialogs for:
- Default browser selection
- Sign-in prompts
- Metrics reporting opt-in
- Welcome pages

### Scaling Issue

Previous attempts at scaling used:
1. `gsettings set org.gnome.mutter experimental-features "['scale-monitor-framebuffer']"` (enables fractional scaling)
2. `ApplyMonitorsConfig` D-Bus call after gnome-shell starts

These didn't work because:
- The experimental feature enables fractional scaling but doesn't set the actual scale
- ApplyMonitorsConfig has complex parameter requirements and timing issues with virtual monitors
- The D-Bus API parameters varied between GNOME versions

## Solution

### Chrome Fix

Restore the comprehensive Chrome configuration from previous commits:

1. **Enterprise Policies** (`/etc/opt/chrome/policies/managed/helix.json`):
   - `DefaultBrowserSettingEnabled: false`
   - `MetricsReportingEnabled: false`
   - `SyncDisabled: true`
   - `BrowserSignin: 0`
   - Plus import and first-run suppressions

2. **Master Preferences** (`/opt/google/chrome/master_preferences`):
   - `skip_first_run_ui: true`
   - `suppress_first_run_default_browser_prompt: true`
   - All import options disabled

3. **Keyring Bypass**: Patch desktop file with `--password-store=basic` to use plaintext storage

4. **First Run Sentinel**: Create `/etc/skel/.config/google-chrome/First Run` file

### Scaling Fix

Use `MUTTER_DEBUG_DUMMY_MONITOR_SCALES` environment variable before starting gnome-shell:

```bash
# In startup script, before gnome-shell starts:
export MUTTER_DEBUG_DUMMY_MONITOR_SCALES=2  # For 200% scaling
gnome-shell --headless --virtual-monitor 1920x1080@60
```

This approach:
- Sets the virtual monitor's scale factor at creation time
- Works with both `--nested` and `--headless` modes
- Is the documented method from GNOME HiDPI wiki
- Doesn't require complex D-Bus calls after startup

Combined with client-app scaling:
- `GDK_SCALE` for GTK applications
- `QT_SCALE_FACTOR` for Qt applications

## Files Modified

**`Dockerfile.ubuntu-helix`**:
- Added Chrome enterprise policies JSON
- Added Chrome master preferences JSON
- Added `--password-store=basic` to desktop file
- Added First Run sentinel file
- Updated scaling to use `MUTTER_DEBUG_DUMMY_MONITOR_SCALES`
- Simplified D-Bus verification (logging only)

## Testing Plan

1. Build the Ubuntu image: `./stack build-ubuntu`
2. Start a session with 200% scaling configured
3. Verify Chrome starts without keyring prompts
4. Verify Chrome starts without first-run dialogs
5. Verify display scaling is correct (text should be larger)
6. Verify screenshots capture the correct scaled resolution

## References

- [GNOME HiDPI Wiki](https://wiki.gnome.org/HowDoI/HiDpi)
- [Arch Linux HiDPI Wiki](https://wiki.archlinux.org/title/HiDPI)
- [Chrome Enterprise Policies](https://chromeenterprise.google/policies/)
- [Mutter GitLab Issue #1376](https://gitlab.gnome.org/GNOME/mutter/-/issues/1376)

## Non-Goals

- Fractional scaling (1.5x, 1.25x) - may work but not tested
- Per-monitor scaling
- Runtime scale changes (scale is set at container start)
