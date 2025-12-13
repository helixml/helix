# External Agent Resolution & Desktop Configuration

**Date:** 2025-12-13
**Status:** Design Review

## Summary

Add user-configurable resolution (1080p/4k) and desktop environment type (Ubuntu X11/Sway Wayland) for external agents, with automatic zoom scaling for 4k displays.

## Changes Overview

### 1. UI Rename: "Zed Agent" → "External Agent"
- **Files:** `frontend/src/components/app/AppSettings.tsx`, `frontend/src/types.ts`, and ~30 other frontend files
- **Scope:** All user-facing labels only (internal type names like `zed_external` remain unchanged for backwards compatibility)

### 2. New Configuration Fields

**Add to `ExternalAgentConfig` struct** (`api/pkg/types/types.go`):

```go
type ExternalAgentConfig struct {
    // ... existing fields ...

    // NEW: Resolution preset ("1080p" or "4k")
    Resolution     string `json:"resolution,omitempty"`      // "1080p" (default) or "4k"

    // NEW: Desktop environment type
    DesktopType    string `json:"desktop_type,omitempty"`   // "ubuntu" (default), "ubuntu2204", "sway"

    // NEW: Zoom level for GNOME (percentage, e.g., 100, 200)
    ZoomLevel      int    `json:"zoom_level,omitempty"`     // Auto: 100 for 1080p, 200 for 4k
}
```

**Resolution presets map to DisplayWidth/DisplayHeight:**
| Resolution | DisplayWidth | DisplayHeight |
|------------|--------------|---------------|
| 1080p      | 1920         | 1080          |
| 4k         | 3840         | 2160          |

### 3. Desktop Type Options

| Value       | Label (UI)                                    | Description                                |
|-------------|-----------------------------------------------|--------------------------------------------|
| `ubuntu`    | Ubuntu 22.04 (X11)                            | Default. GNOME desktop on Xwayland.        |
| `sway`      | Sway (Wayland) - Expert                       | Native Wayland tiling WM. i3-style.        |

### 4. 4k GPU Warning

Display warning in frontend when 4k is selected:
> "4K resolution requires a powerful GPU. Not all configurations support 4K streaming. If you experience issues, try 1080p."

### 5. Automatic Zoom for 4k

When `Resolution == "4k"`:
- Default `ZoomLevel` to 200 (user can override)
- Run gsettings BEFORE GNOME starts to set scaling

### 6. Implementation Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Frontend (AppSettings.tsx)                                               │
│ ├─ Resolution dropdown: [1080p] [4k]                                    │
│ ├─ Desktop Type dropdown: [Ubuntu] [Ubuntu 22.04] [Sway (Expert)]       │
│ └─ If 4k: show warning + zoom slider (default 200%)                     │
└─────────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ API (types.go)                                                          │
│ ├─ ExternalAgentConfig.Resolution                                       │
│ ├─ ExternalAgentConfig.DesktopType                                      │
│ └─ ExternalAgentConfig.ZoomLevel                                        │
└─────────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Wolf Executor (wolf_executor.go)                                        │
│ ├─ Map Resolution → GAMESCOPE_WIDTH/GAMESCOPE_HEIGHT env vars           │
│ ├─ Map DesktopType → image selection (helix-sway vs helix-ubuntu)       │
│ └─ Pass HELIX_ZOOM_LEVEL env var for zoom setting                       │
└─────────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────────┐
│ Startup Script (startup-app.sh)                                         │
│ └─ Before GNOME: gsettings set org.gnome.desktop.interface              │
│    text-scaling-factor $(echo "scale=2; $HELIX_ZOOM_LEVEL/100" | bc)    │
└─────────────────────────────────────────────────────────────────────────┘
```

### 7. Defaults

| Field        | Default  | Notes                              |
|--------------|----------|------------------------------------|
| Resolution   | `1080p`  | Safe for all GPUs                  |
| DesktopType  | `ubuntu` | Most user-friendly                 |
| ZoomLevel    | `100`    | Auto-set to 200 when 4k selected   |

### 8. Files to Modify

**Backend:**
- `api/pkg/types/types.go` - Add fields to ExternalAgentConfig
- `api/pkg/external-agent/wolf_executor.go` - Map config to env vars, select image

**Frontend:**
- `frontend/src/components/app/AppSettings.tsx` - Add dropdowns, warning, rename labels
- `frontend/src/types.ts` - Update type labels
- `frontend/src/contexts/apps.tsx` - Add defaults
- ~30 files: String replace "Zed" → "External" in user-facing labels

**Sandbox:**
- `wolf/ubuntu-config/startup-app.sh` - Read HELIX_ZOOM_LEVEL, run gsettings before GNOME

### 9. Migration

- Existing agents without these fields use defaults (1080p, ubuntu, 100% zoom)
- No database migration needed (JSON fields are optional)

## Out of Scope

- Arbitrary resolution input (only presets)
- Per-monitor zoom settings
- Ubuntu 22.04 image changes (use existing)
