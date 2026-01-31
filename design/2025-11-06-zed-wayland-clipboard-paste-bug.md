# Zed Wayland Clipboard Paste Bug

**Date:** 2025-11-06
**Status:** Known Issue - Upstream Zed Bug
**Affects:** Zed editor on Wayland (Ctrl+V paste from browser → Zed)

## Problem Statement

**Ctrl+C works** (Zed → Browser clipboard):
- Copying text in Zed editor copies to browser user's clipboard ✅
- Copying text in Zed AI panel copies to browser user's clipboard ✅

**Ctrl+V does NOT work** (Browser → Zed clipboard):
- Pasting text copied on user's machine into Zed does NOT work ❌
- This affects both editor panes and AI assistant panel ❌
- **BUT: Firefox paste DOES work** (same container, same WAYLAND_DISPLAY) ✅

## Current Configuration

**Keymap** (`~/.config/zed/keymap.json`):
```json
[
  {
    "bindings": {
      "ctrl-c": "editor::Copy",
      "ctrl-v": "editor::Paste",
      "ctrl-x": "editor::Cut"
    }
  }
]
```

**Wayland Environment:**
- Compositor: Sway (via Games on Whales base-app)
- Clipboard tools: `wl-clipboard` (wl-copy, wl-paste)
- WAYLAND_DISPLAY: `wayland-1` (confirmed for both Zed and screenshot-server)
- Clipboard sync: Screenshot-server uses `wl-copy` to write browser clipboard to Wayland

## Why Firefox Works But Zed Doesn't

**Firefox:**
- Uses native Wayland clipboard protocols
- Receives Wayland data offer events correctly
- Can read from wl-copy clipboard immediately ✅

**Zed:**
- Also uses native Wayland clipboard protocols
- `editor::Copy` writes to Wayland clipboard (works!)
- `editor::Paste` requires receiving a Wayland `current_offer` event
- **Not receiving data offer events from wl-copy** ❌

## Root Cause Analysis

### Hypothesis 1: wl-copy Process Lifetime
When browser pastes to container, screenshot-server calls:
```go
cmd := exec.Command("wl-copy")
cmd.Stdin = strings.NewReader(clipboardData.Data)
cmd.Run() // wl-copy forks to background and cmd.Run() returns immediately
```

wl-copy forks to background to persist clipboard, but Zed might not receive the data offer event in time.

### Hypothesis 2: Zed Wayland Data Offer Bug
From `zed/crates/gpui/src/platform/linux/wayland/clipboard.rs`:

```rust
pub fn read(&mut self) -> Option<ClipboardItem> {
    let offer = self.current_offer.as_ref()?; // Returns None if no offer
    // ...
    let item = offer.read_text(&self.connection)
        .or_else(|| offer.read_image(&self.connection))?;
    // ...
}
```

Zed's `current_offer` is None because it's not receiving Wayland data offer events when wl-copy sets the clipboard.

**Evidence:**
- Firefox receives data offers correctly (same Wayland socket)
- Zed has multiple open GitHub issues about Wayland clipboard paste
- Known upstream bug: #26672, #20984, #12970, #12054

### Hypothesis 3: Clipboard Persistence
On Wayland, clipboard data disappears when the source application closes. wl-copy forks to persist data, but maybe it's not staying alive or Zed can't read from forked wl-copy processes.

**Ruled out:** Firefox can paste, so clipboard persistence is working correctly.

## Attempted Fixes

### ❌ Tried: Context-Specific Keybindings
```json
{
  "context": "Editor",
  "bindings": { "ctrl-v": "editor::Paste" }
},
{
  "context": "AssistantPanel",
  "bindings": { "ctrl-v": "editor::Paste" }
}
```

**Result:** Didn't help - `editor::Paste` doesn't read from system clipboard

### ❌ Tried: wl-clip-persist
Install `wl-clip-persist` package to manage clipboard persistence.

**Result:** Package not available in Ubuntu repos (Arch/NixOS/Alpine only)
**Alternative:** Would need to build from source (Rust project)

## Known Upstream Fixes

**Zed commit f8097c7c98** (May 2025): "Improve compatibility with Wayland clipboard"
- Fixed issue where some applications won't receive clipboard contents **from** Zed
- This is for **outgoing** (Zed → other apps), not **incoming** (other apps → Zed)
- Closed issues: #26672, #20984

**Zed commit 7523a7a437** (Aug 2024): "Do not reset clipboard data offer on keyboard leave"
- Fixed cross-window copy/paste in some Wayland configurations
- Closed #14415

**Status:** Outgoing clipboard works, but incoming paste to Zed remains broken.

## Current Workarounds

### None Available
- Can't paste from browser → Zed using Ctrl+V
- Can copy from Zed → browser using Ctrl+C (works fine)
- Firefox paste works (confirms clipboard data is valid)

### Temporary Manual Workaround
Users can type text instead of pasting (not ideal for large content)

## Potential Solutions

### Option 1: Wait for Upstream Zed Fix
Monitor Zed GitHub issues for Wayland clipboard paste fixes. This is an active issue with multiple reports.

### Option 2: Build wl-clip-persist from Source
Add to Dockerfile:
```dockerfile
# Build wl-clip-persist from source (Rust)
FROM golang:1.24 AS rust-builder
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
RUN . $HOME/.cargo/env && cargo install wl-clip-persist
# Copy binary to final image
COPY --from=rust-builder /root/.cargo/bin/wl-clip-persist /usr/local/bin/
```

Then start it in Sway startup:
```bash
exec WAYLAND_DISPLAY=wayland-1 wl-clip-persist --clipboard both
```

**Effort:** Medium (adds Rust toolchain to build, increases build time)
**Risk:** Low (wl-clip-persist is stable and used in many distros)

### Option 3: Custom Clipboard Bridge
Create a daemon that polls wl-paste and writes to a file Zed can read.

**Effort:** High (need to design and implement)
**Risk:** High (complex, might have race conditions)

### Option 4: Switch to X11/Xwayland
Force Zed to use Xwayland instead of native Wayland.

**Effort:** Low (set WAYLAND_DISPLAY="")
**Risk:** High (Xwayland has input issues with NVIDIA GPUs in our setup)

## Recommendation

**Defer for now** - This is an upstream Zed bug, not a Helix issue.

**When to revisit:**
1. If wl-clip-persist becomes available in Ubuntu repos
2. If upstream Zed fixes Wayland paste
3. If users report paste as a critical blocker

## Technical Details

**Wayland Clipboard Architecture:**
```
Browser Paste Request
    ↓
Helix API (POST /api/v1/external-agents/{id}/clipboard)
    ↓
Screenshot-Server in Container
    ↓
wl-copy (writes to Wayland compositor)
    ↓ (data offer event)
Firefox ✅ receives offer → reads clipboard
Zed ❌ does not receive offer → paste fails
```

**Clipboard Selections:**
- CLIPBOARD: Ctrl+C/V operations
- PRIMARY: Text selection / middle-click paste

Screenshot-server writes to both selections for maximum compatibility.

**Files Involved:**
- `api/cmd/screenshot-server/main.go` - handleSetClipboard() uses wl-copy
- `wolf/sway-config/start-zed-helix.sh` - Zed keymap configuration
- Zed source: `crates/gpui/src/platform/linux/wayland/clipboard.rs` - clipboard.read() requires current_offer

## Related Issues

- Zed #26672: Copy and paste still doesn't work (closed, but only fixed outgoing)
- Zed #20984: copy paste not working on the latest linux update
- Zed #12970: Cannot paste on Wayland
- Zed #12054: Unable to Paste on Wayland

## Next Steps

1. Monitor Zed repository for Wayland clipboard paste fixes
2. Test with newer Zed versions when available
3. Consider wl-clip-persist if it becomes easier to install
4. Document workaround for users (copy/paste via Firefox or terminal as intermediary)
