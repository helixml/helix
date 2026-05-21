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

Both browsers implement the Async Clipboard API but interpret the
"user gesture" requirement differently.

### The spec (and Chrome's behaviour)

The [W3C Clipboard API spec](https://w3c.github.io/clipboard-apis/)
routes `writeText()` through a "check clipboard write permission"
algorithm that depends on
[transient activation](https://html.spec.whatwg.org/multipage/interaction.html#transient-activation)
— a time-limited HTML flag (default ~5 s in Chromium) set when a user
gesture fires. Critically, the algorithm reads activation **at the
time it runs**, not at the time the calling JS task was queued:

> "Let hasGesture be true if the relevant global object of this has
> transient activation, false otherwise."

So a `setTimeout(callback, 300)` followed by `await
apiClient.v1ExternalAgentsClipboardDetail(...)` followed by
`navigator.clipboard.writeText(...)` will succeed in Chrome: the
300 ms delay + ~tens-of-ms API round-trip is well inside the 5 s
transient-activation window. This is exactly what happens in the
current code, which is why Chrome users never reported the bug.

### WebKit's stricter policy

Safari/WebKit goes beyond the spec. The
[2020 WebKit Async Clipboard API blog post](https://webkit.org/blog/10855/async-clipboard-api/)
states:

> "The request to write to the clipboard must be triggered during a
> user gesture."

And specifies that calls outside `"click"` / `"touch"` handlers

> "will result in the immediate rejection of the promise returned by
> the API call."

The same post introduces the `ClipboardItem`-with-Promise pattern
explicitly *because* developers "couldn't realistically `await` the
data first and then call `write()` without losing the gesture." That
is our exact situation — we need 300 ms + an HTTP round-trip to
produce the data.

### Practical consequence

The existing `setTimeout(300) → fetch → writeText` flow is silently
rejected by Safari every time. The current catch-all `// Still show
success` toast hides this from the user.

## Fix: ClipboardItem with a Promise

WebKit added support for `ClipboardItem` constructed with a `Promise<Blob>`
as the value (the "deferred async clipboard" pattern, announced in the
[2020 WebKit blog post on the Async Clipboard
API](https://webkit.org/blog/10855/async-clipboard-api/), which describes
the constructor as taking *"a mapping of MIME type to `Promise` which
may resolve either to a string or a `Blob` of the same MIME type"*).
When you call
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

### 5. Remove the 2.7-second auto-sync polling loop

The `useEffect` near line 2664 polls
`v1ExternalAgentsClipboardDetail` every 2.7 s and writes the result
into the local clipboard via `clipboardWriteText`. The call originates
from `setInterval`, not from any user gesture, so:

- **Safari**: silently fails on every tick. Dead code.
- **Chrome**: succeeds, and that's where the perceived feature comes
  from — "I copied inside the remote desktop and it appeared in my
  local clipboard without pressing Cmd+C."
- **Both**: introduces a race with the explicit Cmd+C path on slow
  networks. If a poll request issued before the user pressed Cmd+C
  returns *after* the new gesture-anchored write completes, it will
  overwrite the just-copied value with stale data. The
  `lastRemoteClipboardHash` guard does not prevent this — the hash is
  only read inside the poll callback to decide whether to *write*; it
  cannot reason about ordering vs the explicit copy path.

Remove the entire `useEffect`. Also remove the two refs that exist
only because of it:

- `lastRemoteClipboardHash` — only consumer was the poll callback
  itself (line ~2692) plus a now-pointless write in the explicit copy
  path (line ~4034).
- `lastAutoSyncedText` — only consumer is the paste branch at
  line ~4091 which short-circuits the upload-then-paste when "local
  clipboard already matches what auto-sync last wrote." With the
  auto-sync gone, this branch is unreachable and can be deleted too;
  paste always uploads the current local clipboard then sends Ctrl+V.

The accepted UX regression (user copying inside the remote via
right-click instead of Cmd+C no longer auto-populates local clipboard)
is documented in `requirements.md`.

This is in scope for this fix, not a separate task: the auto-sync loop
is one of the things that made the original Cmd+C bug confusing to
diagnose (it sometimes papered over the gesture problem on Chrome by
re-writing the right value 2.7 s later), and the simplification it
buys makes the new code substantially easier to reason about.

## Risks and mitigations

| Risk | Mitigation |
|------|------------|
| `ClipboardItem` constructor missing on very old browsers | Feature-detect and fall back to the existing `clipboardWriteText` path |
| Promise rejects (API 404, network) → Safari pastes an empty string | Resolve the inner async with empty string on error, return empty Blob; toast shows error so user knows |
| The 300 ms wait is too short for the remote desktop to actually update the X11 selection | Unchanged from today's behaviour; if it's a problem it's a separate bug |
| Race with the 2.7-second auto-sync overwriting what we just wrote | Removed entirely — see design decision 5 |
| Removing auto-sync regresses users who copy on the remote via right-click (not Cmd+C) | Documented in `requirements.md` — user presses Cmd+C explicitly to recover; was a Chrome-only feature anyway |
| Safari user has denied clipboard permission | Caught in `.catch`, toast tells them why; matches acceptance criterion 2 |

## Files to touch

- `frontend/src/components/external-agent/DesktopStreamViewer.tsx`
  - Refactor the Cmd+C / Ctrl+C branch in `handleKeyDown` (around lines
    3905–4049) to use the ClipboardItem-with-Promise pattern.
  - Update the toast logic so failed local writes don't claim success.
  - Remove the `useEffect` that polls `v1ExternalAgentsClipboardDetail`
    every 2.7 s (around lines 2664–2740).
  - Remove the `lastRemoteClipboardHash` and `lastAutoSyncedText` refs
    (lines 403–404) and all references to them.
  - Remove the paste-flow short-circuit branch at ~line 4091 that
    skips the upload when `text === lastAutoSyncedText.current` — with
    auto-sync gone, the upload always happens.

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
- After removing the auto-sync `useEffect`, also delete the now-unused
  refs (`lastRemoteClipboardHash`, `lastAutoSyncedText`) and the paste-
  flow short-circuit at ~line 4091 (`text === lastAutoSyncedText.current`).
  Don't leave the refs in place "just in case" — they exist only to
  coordinate with the removed loop.
