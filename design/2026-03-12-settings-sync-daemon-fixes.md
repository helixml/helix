# Settings-Sync-Daemon Fixes & Build-Ubuntu Transfer Investigation

**Date:** 2026-03-12
**Branch:** `feature/001518-stop-stop-stop-zed`
**Status:** Code complete, testing blocked by image transfer issue

## Problem Statement

Zed's default `format_on_save: "on"` reformats JS/TS/TSX files on save, which is destructive in Helix codebases. Go is the one language where format-on-save (`gofmt`) should remain enabled. The settings-sync-daemon controls `~/.config/zed/settings.json` inside desktop containers and needed to be updated.

What started as a simple "add `format_on_save: off`" change turned into a comprehensive fix of three interconnected bugs in the daemon.

## Bug 1: Dead fsnotify Watcher (Critical)

### Root Cause

`writeSettings()` does an atomic write via `os.Rename(tmpFile, SettingsPath)`. On Linux, inotify watches **inodes**, not paths. After the first atomic rename, the inode at `SettingsPath` is replaced — the fsnotify watcher becomes permanently dead. `onFileChanged()` never fires again after the daemon's first write.

This means `d.userOverrides` stays empty forever. User overrides for ANY setting (theme, context_servers, everything) have **never worked** since the daemon was written.

### Fix

Watch the settings **directory** instead of the file, and filter events by `filepath.Base(event.Name)`. This is the same pattern already used for the Claude credentials watcher in the same file.

```go
// Watch the settings DIRECTORY (not the file itself) so atomic renames
// in writeSettings() don't kill the watcher.
if err := watcher.Add(settingsDir); err != nil {
    return err
}
```

## Bug 2: Spurious Rewrite Every 30 Seconds

### Root Cause

`injectLanguageModelAPIKey()` and `injectAvailableModels()` mutate `d.helixSettings` in place **after** the `deepEqual` baseline is captured. On the next poll (every 30s), the fresh API response differs from the mutated version → `deepEqual` always fails → the daemon rewrites settings.json every 30 seconds.

This caused the user's most visible complaint: changing the theme in Zed would be reverted every 30 seconds.

### Fix

Store `d.helixSettingsBaseline` — a snapshot of the settings **before** injection mutations — and compare against that on subsequent polls.

```go
// Save baseline BEFORE injection for future deepEqual comparisons
d.helixSettingsBaseline = copyMap(d.helixSettings)

// Now mutate for the actual write
injectAgentToolPermissions(d.helixSettings)
d.injectLanguageModelAPIKey()
d.injectAvailableModels()
```

## Bug 3: Broken Settings Ownership Model

### Root Cause

The daemon had three ad-hoc categories with no clear boundaries:

| Category | Fields | Problem |
|----------|--------|---------|
| Protected | `telemetry`, `agent_servers` | `telemetry` was special-cased with read-from-disk logic |
| Hardcoded defaults | `text_rendering_mode`, `suggest_dev_container` | Set in `syncFromHelix()` but NOT in `checkHelixUpdates()` — dropped after first 30s poll |
| Everything else | `theme`, `language_models`, etc. | Fully Helix-owned; user changes supposed to be captured as overrides but never were (broken watcher) |

### Fix

Introduced three clearly separated categories with explicit declarations:

1. **`SECURITY_PROTECTED_FIELDS`** — never read from disk, always overwritten
2. **`USER_PREFERENCE_FIELDS`** — set as initial default, then always read from disk and never overwritten (currently just `theme`)
3. **`helixDefaults()`** — a shared function returning static Helix-owned defaults, used by both `syncFromHelix()` and `checkHelixUpdates()` to ensure consistency

Also added deep-merge for `languages` key (same pattern as `context_servers`) in both `mergeSettings()` and `extractUserOverrides()`.

## Changes Made

Single file: `api/cmd/settings-sync-daemon/main.go` (+123, -54)

| Change | Description |
|--------|-------------|
| `helixDefaults()` function | Static defaults shared by both sync paths: `format_on_save: "off"`, `languages.Go.format_on_save: "on"`, `text_rendering_mode`, `suggest_dev_container` |
| `USER_PREFERENCE_FIELDS` map | `{"theme": true}` — set as initial default, then preserved from disk |
| `helixSettingsBaseline` field | Pre-injection snapshot for `deepEqual` comparison |
| Directory-level fsnotify watcher | Watches `~/.config/zed/` instead of `settings.json` directly |
| Deep merge for `languages` | Same pattern as `context_servers` in `mergeSettings()` and `extractUserOverrides()` |
| `injectAgentToolPermissions()` | Extracted shared helper for both sync paths |
| `copyMap()` helper | For creating the baseline snapshot |

## Build & Deploy

```bash
go build ./cmd/settings-sync-daemon/   # ✅ compiles
./stack build-ubuntu                    # ✅ built helix-ubuntu:ac1bb0
```

---

## Investigation: Build-Ubuntu Image Transfer to Sandbox

### Symptom

After running `./stack build-ubuntu`, the version file (`sandbox-images/helix-ubuntu.version`) says `ac1bb0` but the sandbox's inner dockerd only has the old image (`c7b935`). New session creation fails with "no such image: helix-ubuntu:ac1bb0".

User reports this happens on all new Helix-in-Helix sessions.

### Architecture Recap

In Helix-in-Helix (docker-in-desktop mode):

```
Desktop Container (where agent runs)
├── Desktop's dockerd ← builds helix-ubuntu:latest here
│   ├── helix-registry-1 (localhost:5000)
│   ├── helix-sandbox-nvidia-1 (privileged, DinD)
│   │   └── Sandbox's inner dockerd ← image must end up here
│   │       └── helix-ubuntu:TAG ← used by Hydra to launch sessions
│   ├── helix-api-1
│   └── ... other compose services
```

The `transfer-desktop-to-sandbox` function:
1. Tags `helix-ubuntu:latest` as `localhost:5000/helix-ubuntu:TAG`
2. Pushes to the local registry (on desktop's dockerd)
3. Sandbox's inner dockerd pulls from `registry:5000/helix-ubuntu:TAG` (Docker DNS resolves `registry` to the inner registry container)
4. Tags as `helix-ubuntu:TAG` and `helix-ubuntu:latest` inside sandbox

### What We Verified

| Check | Result |
|-------|--------|
| Transfer mechanism works | ✅ Running `./stack transfer-ubuntu-to-sandbox` manually succeeds every time |
| Initial `build-sandbox` transfer | ✅ Timing log proves `c7b935` was transferred in 266s at session startup |
| Persistent volume survives restarts | ✅ `helix_sandbox-docker-storage` named volume retains images across `stop`/`start` |
| `./stack start` doesn't recreate sandbox | ✅ Confirmed same container ID before and after — `stop` + `up -d` just restarts |
| Registry DNS resolution from sandbox | ✅ `registry` resolves to 10.214.1.3 (inner registry, correct) |
| Port 5000 mapping | ✅ `localhost:5000` maps to inner `helix-registry-1`, not outer registry |
| Sandbox network isolation | ✅ Only on `helix_default` (inner), no access to outer compose network |

### Timeline of This Session

| Time (UTC) | Event |
|------------|-------|
| 14:21 | Startup script ran `build-sandbox` — built `helix-ubuntu:c7b935`, transferred to sandbox (266s), started stack |
| 14:21-21:26 | Previous agent session worked on settings-sync-daemon code changes |
| 21:26 | Previous agent ran `./stack build-ubuntu` — built `helix-ubuntu:ac1bb0` (new image with daemon fixes) |
| 21:26+ | Version file updated to `ac1bb0`, but sandbox still only has `c7b935` |

The previous session's `build-ubuntu` transfer failed silently. We don't have logs from that specific run.

### Hypotheses

#### 1. Transient failure during previous session's transfer (likely for this instance)
The `transfer-desktop-to-sandbox` function has error handling that continues on failure (`⚠️ Failed to transfer`). A transient Docker or network error during the push or pull would leave the version file updated but the sandbox without the image. The manual transfer works consistently now, suggesting a non-persistent issue.

#### 2. Race condition: version file updated before transfer completes
`build-desktop` writes the version file **before** calling `transfer-desktop-to-sandbox`. The sandbox heartbeat daemon reads the version file and reports the new version to the API. If a session is created during the transfer window (push + pull can take minutes for uncached layers), the API requests an image the sandbox doesn't have yet.

#### 3. Registry shadowing in Helix-in-Helix (unconfirmed)
The outer Helix also runs a registry service. If during startup the inner registry hasn't started yet but the sandbox can somehow resolve `registry` to the outer one (via leaked DNS, iptables, or network misconfiguration), the push and pull would target different registries. **We ruled out** the obvious paths (sandbox is only on inner network, Docker DNS is scoped, no extra_hosts), but there could be subtler interactions with privileged mode or iptables rules from the outer level.

#### 4. `docker compose up -d` profile handling
If `COMPOSE_PROFILES` isn't set correctly when `./stack start` runs, the sandbox might not be started/restarted properly. We confirmed `setup_sandbox_profile()` auto-detects NVIDIA and sets the profile, but there could be edge cases in the startup script's environment.

### What's NOT the Problem

- **Volume loss on restart**: Named volume `helix_sandbox-docker-storage` persists across container recreation. Confirmed.
- **Init script cleanup**: The `04-start-dockerd.sh` cleanup section only removes images that don't match the current version tag — it wouldn't remove a freshly transferred image.
- **`./stack start` wiping images**: `stop` + `up -d` doesn't recreate the container or touch the volume.

### Recommended Fix

Regardless of root cause, the version file should not be updated until the transfer succeeds. This eliminates the race condition (hypothesis 2) and makes transient failures visible (hypothesis 1):

```bash
# In build-desktop, AFTER successful transfer:
echo "${IMAGE_TAG}" > "sandbox-images/${IMAGE_NAME}.version"

# Or: write version file early but add a verification step:
# After transfer, verify sandbox has the image before declaring success
```

Additionally, `load_desktop_image` in `04-start-dockerd.sh` could attempt a pull from the local registry (`registry:5000`) as a fallback when the image is missing and no `.ref` file exists. This would make the sandbox self-healing on restart.

### Next Steps

1. **Test the settings-sync-daemon changes** — requires the image transfer to work. Either:
   - Run `./stack transfer-ubuntu-to-sandbox` manually (confirmed working), then start a session
   - Or fix the transfer ordering and rebuild
2. **Fix version file write ordering** in `build-desktop` to eliminate the race
3. **Add registry fallback** in `load_desktop_image` for resilience
4. **Add transfer verification** — after transfer, confirm sandbox has the expected image before updating version file

### Manual Workaround

Until the fix is in place, after `./stack build-ubuntu`:

```bash
./stack transfer-ubuntu-to-sandbox   # re-run transfer manually
# Verify:
docker exec helix-sandbox-nvidia-1 docker images helix-ubuntu --format "{{.Tag}}: {{.ID}}"
```

## Verification Plan (Settings-Sync-Daemon)

Once image transfer is working:

1. Start a new session → confirm theme is "Ayu Dark" (default)
2. Change theme in Zed → wait >30s → confirm it is NOT reverted
3. Open a JS/TS file, edit, save → confirm no auto-formatting
4. Open a Go file, edit, save → confirm `gofmt` runs
5. Check daemon logs → confirm no "Detected Helix config change" spam every 30s