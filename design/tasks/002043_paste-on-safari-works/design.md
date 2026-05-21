# Design: Fix Safari Copy from Remote Desktop to System Clipboard

## Where the bug lives

Single file: `frontend/src/components/external-agent/DesktopStreamViewer.tsx`.

The relevant block is the Cmd+C / Ctrl+C interception in the
`handleKeyDown` listener (currently around lines 3905–4049). The copy
helper `clipboardWriteText` (lines 73–81) is also relevant.

Current shape (paraphrased):

```ts
if (isCopyKeystroke && sessionIdRef.current) {
  // 1. forward Ctrl+C to remote desktop synchronously (good)
  input.onKeyDown(ctrlDown); input.onKeyDown(ctrlCDown);
  input.onKeyUp(ctrlCUp);   input.onKeyUp(ctrlUpForCopy);

  // 2. wait 300ms then fetch + write — THIS LOSES THE USER GESTURE ON SAFARI
  setTimeout(async () => {
    const response = await apiClient.v1ExternalAgentsClipboardDetail(sessionIdRef.current);
    const clipboardData = response.data;
    ...
    await clipboardWriteText(clipboardData.data); // Safari: silently fails
    ...
    showClipboardToast("Copied", "success");      // misleading on Safari
  }, 300);
  event.preventDefault();
  event.stopPropagation();
}
```

## Why Safari fails and Chrome doesn't

Both browsers implement the Async Clipboard API, but Safari/WebKit
enforces a stricter rule: a successful `navigator.clipboard.writeText()`
(or `.write()`) requires the call to be made **inside the same
user-gesture task** as the originating UI event. A `setTimeout` callback
runs on a fresh task with no gesture, so Safari treats the write as
untrusted and refuses it.

Chrome is more permissive — it allows clipboard writes for a short
window after a gesture and across awaited promises — so today's code
happens to work there.

## Fix: ClipboardItem with a Promise

WebKit added support for `ClipboardItem` constructed with a `Promise<Blob>`
as the value (the "deferred async clipboard" pattern, announced in the
[2020 WebKit blog post on the Async Clipboard
API](https://webkit.org/blog/10855/async-clipboard-api/)). When you call
`navigator.clipboard.write([new ClipboardItem({ "text/plain": promise })])`
**synchronously inside the user gesture**, Safari accepts the write and
waits for the promise to resolve before populating the system pasteboard.
Chrome 76+ supports the same constructor signature.

This lets us:

- Initiate the local clipboard write **inside the Cmd+C keydown handler**
  (preserves the user gesture).
- Resolve the actual text **asynchronously** — after the 300 ms wait and
  the API call — without losing the gesture.

### New copy flow

```ts
if (isCopyKeystroke && sessionIdRef.current) {
  event.preventDefault();
  event.stopPropagation();

  // 1. forward Ctrl+C to remote (unchanged)
  forwardCtrlCToRemote(input, event);

  // 2. Build a promise that fetches the remote clipboard.
  //    The promise resolves to a Blob containing the new text.
  const sessionId = sessionIdRef.current;
  const textBlobPromise: Promise<Blob> = (async () => {
    await delay(300);
    const response = await apiClient.v1ExternalAgentsClipboardDetail(sessionId);
    const data = response.data;
    const text = (data && data.type === "text" && data.data) || "";
    return new Blob([text], { type: "text/plain" });
  })();

  // 3. Synchronously start the clipboard write inside the user gesture.
  //    Safari accepts this; the promise resolves later and Safari uses it.
  if (
    !isInIframe &&
    typeof ClipboardItem !== "undefined" &&
    navigator.clipboard?.write
  ) {
    navigator.clipboard
      .write([new ClipboardItem({ "text/plain": textBlobPromise })])
      .then(() => {
        // also update lastRemoteClipboardHash from the blob we just wrote
        showClipboardToast("Copied", "success");
      })
      .catch((err) => {
        // Local write blocked. The remote copy still happened.
        console.warn("[Clipboard] local write blocked:", err);
        showClipboardToast("Copied on remote — local clipboard blocked", "error");
      });
  } else {
    // Fallback for: iframe (postMessage bridge), or browsers without
    // ClipboardItem (covers the same code paths already in use).
    textBlobPromise.then(async (blob) => {
      const text = await blob.text();
      try {
        if (text) await clipboardWriteText(text);
        showClipboardToast("Copied", "success");
      } catch (err) {
        showClipboardToast("Copied on remote — local sync failed", "error");
      }
    });
  }
}
```

## Key design decisions

### 1. ClipboardItem-with-Promise over `document.execCommand('copy')` fallback

`document.execCommand('copy')` is deprecated and requires a real DOM
selection. Using a hidden textarea works but is fiddly and would still
need the actual text synchronously — which we don't have (it's behind
a 300 ms round-trip to the remote). The Promise-valued ClipboardItem
solves both problems at once and is the modern WebKit-blessed approach.

### 2. Keep the existing iframe / postMessage bridge unchanged

When `isInIframe` is true the code already routes through the Wails
runtime via `postMessage`. The new ClipboardItem path only runs in
non-iframe browsers. We do not need to alter the bridge.

### 3. Stop lying in the toast

Today's catch block always shows "Copied" success even when the local
write failed. The new code distinguishes:

- success → green "Copied".
- remote OK but local write blocked → orange/red "Copied on remote —
  local clipboard blocked" (uses the existing `"error"` toast variant
  which already auto-dismisses after 4 s instead of 2 s — perfect for
  reading the longer message).

### 4. Image clipboard

For now keep the existing 300 ms `setTimeout` path for images —
`ClipboardItem` with a Promise<Blob> works for `image/png` too in
theory, but the image case is rarer, the data is much larger, and the
existing image path is gated behind `clipboardData.type === "image"`
which we don't know until *after* the API call. Mixing text + image
into the same ClipboardItem before knowing the type would force us to
fetch the clipboard twice. Out of scope for this fix — we focus on
text, which is the reported and dominant case.

### 5. Auto-sync loop is unaffected

The 2-second poll at line ~2664 already uses `clipboardWriteText`
inside an async function that did not originate from a user gesture.
Safari blocks this *today* too (silently), so the auto-sync is
de-facto a Chrome-only feature. We don't try to fix that here — Cmd+C
is what users actually care about, and the new gesture-anchored write
covers it.

## Risks and mitigations

| Risk | Mitigation |
|------|------------|
| `ClipboardItem` constructor missing on very old browsers | Feature-detect and fall back to the existing `clipboardWriteText` path |
| Promise rejects (API 404, network) → Safari pastes an empty string | Resolve the inner async with empty string on error, return empty Blob; toast shows error so user knows |
| The 300 ms wait is too short for the remote desktop to actually update the X11 selection | Unchanged from today's behaviour; if it's a problem it's a separate bug |
| Race with the 2-second auto-sync overwriting what we just wrote | Update `lastRemoteClipboardHash` / `lastAutoSyncedText` from the new path the same way the existing code does |
| Safari user has denied clipboard permission | Caught in `.catch`, toast tells them why; matches acceptance criterion 2 |

## Files to touch

- `frontend/src/components/external-agent/DesktopStreamViewer.tsx`
  - Refactor the Cmd+C / Ctrl+C branch in `handleKeyDown` (around lines
    3905–4049) to use the ClipboardItem-with-Promise pattern.
  - Update the toast logic so failed local writes don't claim success.

No backend or generated-API-client changes.

## Test plan

Manual end-to-end (the only meaningful test for a browser-clipboard
quirk):

1. **Safari on macOS** — primary regression target.
   - Open a desktop session in Safari at `https://app.helix.example/...`.
   - Select text in the remote desktop (e.g. open Terminal, type
     `echo hello world`, select "hello world"). Press Cmd+C.
   - Switch to a native macOS app (Notes / TextEdit) and Cmd+V.
     Expected: "hello world" pastes. (Today: previous clipboard contents.)
   - Verify the toast says "Copied" (green) on success.
   - Revoke clipboard permission for the site in Safari settings, retry,
     verify the toast becomes the error variant.
2. **Chrome on macOS** — verify no regression. Same flow as above.
3. **macOS Wails app (iframe)** — verify the postMessage bridge is
   unchanged.
4. **Paste flows on Safari** — Cmd+V via Safari's paste button, native
   `paste` DOM event, and the keyboard fallback. None should regress.
5. **Auto-sync** — copy something on the remote via the agent (no user
   keypress), wait 2 s in Chrome, verify local clipboard updates.

## Notes for the implementer

- Don't add a Safari-specific branch on `navigator.userAgent`. The
  ClipboardItem-with-Promise path is the right code path for all modern
  browsers; only fall back when `ClipboardItem` or
  `navigator.clipboard.write` are missing.
- Don't widen the change to a refactor of the whole clipboard subsystem
  — the file is 4000+ lines and the bug is in one branch.
- Preserve all the existing `console.log("[Clipboard] ...")` /
  `[Paste DEBUG]` diagnostic logging — it's been useful for the
  WKWebView paste bugs and we'll want it again next time.
- Update `lastRemoteClipboardHash.current` in the success path so the
  2-second auto-sync poller doesn't immediately re-write the same value.
