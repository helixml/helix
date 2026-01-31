# Keyboard Layout Support for Ubuntu Desktop Containers

**Date**: 2025-12-17
**Status**: Phase 1 Complete, Phase 2 Implemented

## Problem Statement

Users launching Ubuntu desktop containers from the Helix web UI were limited to US English keyboard layout only. International users (French, German, etc.) had no way to type special characters or use their native keyboard layout.

## Solution Overview

Two-phase approach:
1. **Phase 1 (COMPLETED)**: Add multiple keyboard layouts to Ubuntu image so users can manually switch in GNOME settings
2. **Phase 2 (PLANNED)**: Automatically detect browser's keyboard locale and set it as the default when the container launches

---

## Phase 1: Manual Keyboard Layout Selection (COMPLETED)

### What Was Implemented

Added keyboard input sources configuration to the Ubuntu desktop's dconf settings.

**File Modified**: `wolf/ubuntu-config/dconf-settings.ini`

**Changes Added** (at end of file):
```ini
# Keyboard Input Sources - Multiple layouts for international users
# Users can switch via GNOME Shell indicator (top bar) or Super+Space
# Layout list: US (default), UK, French, German, Spanish, Italian, Portuguese, Russian, Japanese, Chinese
[org/gnome/desktop/input-sources]
sources=[('xkb', 'us'), ('xkb', 'gb'), ('xkb', 'fr'), ('xkb', 'de'), ('xkb', 'es'), ('xkb', 'it'), ('xkb', 'pt'), ('xkb', 'ru'), ('xkb', 'jp'), ('xkb', 'cn')]
xkb-options=['caps:ctrl_nocaps']
per-window=false
show-all-sources=false
```

### Layouts Included

| Code | Language |
|------|----------|
| `us` | English (US) - default |
| `gb` | English (UK) |
| `fr` | French |
| `de` | German |
| `es` | Spanish |
| `it` | Italian |
| `pt` | Portuguese |
| `ru` | Russian |
| `jp` | Japanese |
| `cn` | Chinese |

### How Users Switch Layouts

1. **Top bar indicator**: Click the keyboard icon (shows "en", "fr", etc.)
2. **Super+Space**: Quick keyboard shortcut to cycle through layouts
3. **Settings**: Settings → Keyboard → Input Sources for full management

### Testing Phase 1

```bash
# Build updated image
./stack build-ubuntu

# Launch new session (existing sessions won't pick up change)
# Then in GNOME:
# - Click keyboard indicator in top bar
# - Verify all 10 layouts appear
# - Switch to French, type "éèàç" to verify
```

---

## Phase 2: Automatic Browser Locale Detection (IMPLEMENTED)

### Goal

Detect the user's browser keyboard/locale settings and automatically configure that as the default keyboard layout when the container starts. This eliminates the need for users to manually switch layouts.

### Architecture

```
Browser (Frontend)
    ↓ navigator.language / Keyboard API
    ↓
POST /api/v1/external-agents
    ↓ XKB_DEFAULT_LAYOUT env var in request
    ↓
wolf_executor.go
    ↓ passes env var to container
    ↓
startup-app.sh
    ↓ reads XKB_DEFAULT_LAYOUT
    ↓ generates keyboard-override.ini
    ↓
desktop.sh
    ↓ loads keyboard-override.ini via dconf
    ↓
GNOME Desktop (starts with user's preferred layout)
```

### Implementation Details

#### 1. Frontend: Browser Locale Detection Hook

**New file**: `frontend/src/hooks/useBrowserLocale.ts`

```typescript
export interface BrowserLocaleInfo {
  language: string;           // e.g., "en-US", "fr-FR"
  keyboardLayout: string;     // e.g., "us", "fr" (derived from language)
  timezone: string;           // e.g., "America/New_York"
}

export function useBrowserLocale(): BrowserLocaleInfo {
  const language = navigator.language || 'en-US';
  const keyboardLayout = deriveKeyboardLayout(language);
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;

  return { language, keyboardLayout, timezone };
}

function deriveKeyboardLayout(language: string): string {
  const langCode = language.split('-')[0].toLowerCase();  // "fr-FR" → "fr"
  const regionCode = language.split('-')[1]?.toLowerCase();  // "fr-FR" → "fr"

  // Special cases where region matters
  if (langCode === 'en') {
    if (regionCode === 'gb' || regionCode === 'uk') return 'gb';
    return 'us';
  }
  if (langCode === 'pt') {
    if (regionCode === 'br') return 'br';
    return 'pt';
  }
  if (langCode === 'zh') return 'cn';
  if (langCode === 'ja') return 'jp';

  // Default: use language code as layout
  return langCode;
}
```

#### 2. Frontend: Pass Locale in Session Creation

**File**: `frontend/src/components/external-agent/ExternalAgentManager.tsx`

In the `handleCreate` function, add keyboard layout to environment variables:

```typescript
import { useBrowserLocale } from '../../hooks/useBrowserLocale';

// Inside component:
const { keyboardLayout, timezone } = useBrowserLocale();

// In handleCreate:
const envWithLocale = [
  ...(input.env || []),
  `XKB_DEFAULT_LAYOUT=${keyboardLayout}`,
  `TZ=${timezone}`,
];

// Pass envWithLocale instead of input.env to the API
```

#### 3. Backend: Environment Variable Passthrough

**File**: `api/pkg/external-agent/wolf_executor.go`

No changes needed - the Wolf executor already passes all environment variables from the `ZedAgent.Env` field to the container via `createDesktopWolfApp()`.

The flow is:
1. Frontend sends `env: ["XKB_DEFAULT_LAYOUT=fr", ...]` in POST request
2. `external_agent_handlers.go` decodes into `ZedAgent.Env`
3. `wolf_executor.go` adds these to `extraEnv` array (line ~820)
4. Wolf creates container with these env vars

#### 4. Container: Dynamic Keyboard Configuration

**File**: `wolf/ubuntu-config/startup-app.sh`

Add after line 244 (after the IBus disable section):

```bash
# ============================================================================
# Dynamic Keyboard Layout Configuration
# ============================================================================
# If XKB_DEFAULT_LAYOUT is set (from browser locale detection), configure it
# as the primary layout while keeping other layouts available
if [ -n "$XKB_DEFAULT_LAYOUT" ]; then
    echo "Setting default keyboard layout from browser: $XKB_DEFAULT_LAYOUT"

    # Build dconf-compatible sources array with detected layout first
    # Format: [('xkb', 'layout1'), ('xkb', 'layout2'), ...]
    LAYOUT_SOURCES="[('xkb', '$XKB_DEFAULT_LAYOUT')"

    # Add other common layouts (skip if already the default)
    for layout in us gb fr de es; do
        if [ "$layout" != "$XKB_DEFAULT_LAYOUT" ]; then
            LAYOUT_SOURCES="$LAYOUT_SOURCES, ('xkb', '$layout')"
        fi
    done
    LAYOUT_SOURCES="$LAYOUT_SOURCES]"

    # Create override dconf file that will be loaded after static settings
    cat > /tmp/keyboard-override.ini << KEYBOARD_EOF
[org/gnome/desktop/input-sources]
sources=$LAYOUT_SOURCES
KEYBOARD_EOF

    echo "Keyboard layout sources: $LAYOUT_SOURCES"
else
    echo "No XKB_DEFAULT_LAYOUT set, using default layouts from dconf-settings.ini"
fi
```

#### 5. Container: Load Override in desktop.sh

**File**: `wolf/ubuntu-config/gow/desktop.sh`

Add after the main dconf load (around line 56):

```bash
# Apply keyboard override if set dynamically by browser locale
if [ -f /tmp/keyboard-override.ini ]; then
    echo "Loading keyboard layout override from /tmp/keyboard-override.ini"
    dconf load / < /tmp/keyboard-override.ini || echo "Warning: keyboard dconf load failed"
fi
```

### Files to Modify for Phase 2

| File | Change |
|------|--------|
| `frontend/src/hooks/useBrowserLocale.ts` | **NEW** - Browser locale detection hook |
| `frontend/src/components/external-agent/ExternalAgentManager.tsx` | Pass `XKB_DEFAULT_LAYOUT` env var |
| `wolf/ubuntu-config/startup-app.sh` | Generate `keyboard-override.ini` from env |
| `wolf/ubuntu-config/gow/desktop.sh` | Load `keyboard-override.ini` after main dconf |

### Testing Phase 2

1. **Change browser language** (Chrome: Settings → Languages → Add French → Move to top)
2. **Rebuild image**: `./stack build-ubuntu`
3. **Launch new session**
4. **Verify startup log**:
   ```bash
   docker exec <container> cat /tmp/ubuntu-startup-debug.log
   # Should show: "Setting default keyboard layout from browser: fr"
   ```
5. **Verify GNOME**: French keyboard should be default (indicator shows "fr")
6. **Type test**: Special characters éèàç should work immediately

---

## Keyboard Layout Reference

### Common XKB Layout Codes

| Code | Language | Code | Language |
|------|----------|------|----------|
| `us` | English (US) | `ru` | Russian |
| `gb` | English (UK) | `jp` | Japanese |
| `fr` | French | `cn` | Chinese |
| `de` | German | `kr` | Korean |
| `es` | Spanish | `ar` | Arabic |
| `it` | Italian | `he` | Hebrew |
| `pt` | Portuguese (PT) | `pl` | Polish |
| `br` | Portuguese (BR) | `nl` | Dutch |
| `se` | Swedish | `no` | Norwegian |
| `dk` | Danish | `fi` | Finnish |

### Full Layout List

Available at: `/usr/share/X11/xkb/rules/evdev.lst` (inside container)

### XKB Options

| Option | Effect |
|--------|--------|
| `caps:ctrl_nocaps` | Caps Lock → Ctrl (no Caps Lock) |
| `caps:escape` | Caps Lock → Escape |
| `caps:super` | Caps Lock → Super (Win key) |
| `compose:ralt` | Right Alt as Compose key |

---

## Related Files

### Session Launch Flow

1. **Frontend session creation**: `frontend/src/components/external-agent/ExternalAgentManager.tsx`
2. **Backend handler**: `api/pkg/server/external_agent_handlers.go`
3. **Wolf executor**: `api/pkg/external-agent/wolf_executor.go`
4. **Container entrypoint**: `wolf/ubuntu-config/entrypoint.sh`
5. **Startup script**: `wolf/ubuntu-config/startup-app.sh`
6. **Desktop launcher**: `wolf/ubuntu-config/gow/desktop.sh`
7. **GNOME dconf settings**: `wolf/ubuntu-config/dconf-settings.ini`

### Sway Keyboard Config (Reference)

The Sway desktop already has keyboard layouts configured in `wolf/sway-config/config`:

```ini
input type:keyboard {
    xkb_layout "us,gb,fr"
    xkb_options "caps:ctrl_nocaps"
    repeat_delay 0
    repeat_rate 0
}
```

---

## Future Enhancements

1. **Keyboard API**: Use experimental `navigator.keyboard.getLayoutMap()` for actual keyboard layout detection (limited browser support currently)
2. **User preferences**: Store keyboard preference in user profile for persistence
3. **Session UI**: Add keyboard layout dropdown in session creation dialog
4. **IME support**: Enable IBus/Fcitx for complex input methods (CJK languages)
