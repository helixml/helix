# Requirements: Fix Chrome Copy-Paste Regression in Remote Desktop Clipboard Sync

## Background

PR [#2607](https://github.com/helixml/helix/pull/2607) ("Fix Safari copy
from remote desktop to system clipboard", spec task `002043`) reworked the
Cmd+C / Ctrl+C clipboard handler in
`frontend/src/components/external-agent/DesktopStreamViewer.tsx`. It fixed
copy on Safari but **regressed copy on Chrome** — the browser most users
are on. Copying from the remote desktop no longer reaches the local system
clipboard on Chrome, so paste-back (and paste into any local app) is broken.

### Root cause

To anchor the clipboard write to the user gesture (a WebKit requirement),
the new handler builds a single `ClipboardItem` declaring **both**
`text/plain` and `image/png` up front, because it doesn't yet know which
type the remote produced. The MIME that does *not* match is filled with a
**zero-byte Blob** (`new Blob([], { type: "image/png" })`).

Chrome sanitizes images written to the clipboard by decoding them. A
zero-byte Blob is not a valid PNG, so Chrome **rejects the whole
`navigator.clipboard.write()` promise** — neither the (valid) `text/plain`
nor the (invalid) `image/png` reaches the clipboard. The `.catch` then fires
the misleading red toast "Copied on remote — local clipboard blocked".
Safari tolerated the empty representation, which is why the regression
shipped unnoticed.

## User Stories

### US-1: Copy text from remote desktop on Chrome
**As a** Chrome user viewing a remote desktop session,
**I want** Ctrl/Cmd+C to copy the selected text to my local system clipboard,
**so that** I can paste it into local apps and back into the remote.

**Acceptance criteria:**
- Pressing Ctrl+C (Linux/Win) or Cmd+C (macOS) over remote-selected text
  places that text on the local system clipboard on Chrome.
- A green "Copied text" toast appears; the red "local clipboard blocked"
  toast does **not** appear for a successful text copy.
- Pasting (Ctrl/Cmd+V) into a local text field yields the copied text.

### US-2: Copy image from remote desktop still works
**As a** user who copies an image inside the remote desktop,
**I want** Ctrl/Cmd+C to place the image on my local clipboard,
**so that** image copy parity (added in 002043) is preserved.

**Acceptance criteria:**
- Copying an image in the remote and pressing Ctrl/Cmd+C puts a valid PNG on
  the local clipboard on both Chrome and Safari.
- A green "Copied image" toast appears.

### US-3: Paste back into the remote pastes the right type
**As a** user who copied text from the remote,
**I want** pasting back into the remote (Ctrl/Cmd+V) to paste the **text**,
**so that** the round-trip is not silently replaced by a placeholder image.

**Acceptance criteria:**
- After copying text, Ctrl/Cmd+V into the remote pastes the text, not an
  empty/placeholder image.
- After copying a real image, Ctrl/Cmd+V into the remote pastes the image.

### US-4: Safari fix is preserved
**As a** Safari user,
**I want** the 002043 copy fix to keep working,
**so that** we fix Chrome without re-breaking Safari.

**Acceptance criteria:**
- Copy from remote on Safari still writes to the system clipboard (gesture
  anchoring via synchronous `ClipboardItem` is retained — no `await` before
  `navigator.clipboard.write()`).

## Out of Scope

- Restoring the removed 2.7s background remote→local auto-sync poll (it was
  deliberately removed in 002043; right-click "Copy" inside the remote that
  does not emit Ctrl+C is not covered by this fix).
- The macOS Wails iframe / NSPasteboard bridge path (it writes a single MIME
  per copy and is not affected by the dual-MIME Chrome rejection). It is only
  touched by the read-ordering change in US-3 for consistency.
- A server-side "wait for clipboard change" endpoint (tracked as future work
  in 002043).
