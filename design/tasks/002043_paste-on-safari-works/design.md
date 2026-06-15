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
- Resolve the actual payload **asynchronously** — after the remote
  clipboard updates and an API round-trip — without losing the gesture.
- Cover **text and image** in the same gesture-anchored `write()` call
  by declaring both MIME types up front (we don't know which the remote
  will return until the fetch resolves).

### Two architectural sub-problems

We have to solve two things at the same time:

1. **"When does the remote clipboard actually have the new value?"** —
   client JS has no synchronous signal. The data path is
   `Browser ⌘C → WebSocket input → focused app handles Ctrl+C → GNOME
   D-Bus/wl-copy/xclip → port-9876 clipboard server → API GET → us`.
   The current code hard-codes a 300 ms wait. We replace this with
   **bounded adaptive polling** inside the deferred promise: snapshot
   the clipboard hash before sending Ctrl+C, then poll every ~30 ms
   for up to ~500 ms, return the moment the hash differs. On a fast
   desktop this resolves in ~30–60 ms; on a slow one we still wait up
   to 500 ms (vs today's unconditional 300 ms). A proper server-side
   "wait for clipboard change" endpoint is the right long-term fix —
   tracked as a follow-up (see *Future work*).
2. **"We don't know if it's text or image until the fetch returns,
   but ClipboardItem requires MIME types declared synchronously."** —
   we declare **both** `text/plain` and `image/png` in the
   `ClipboardItem`. Each MIME's `Promise<Blob>` resolves with the real
   Blob if the fetched type matches, or a zero-byte Blob of that MIME
   if it doesn't. Paste destinations naturally prefer the MIME they
   support: Notes/TextEdit/IDEs read `text/plain`, Preview/Photoshop
   read `image/png`. The unused-type empty Blob is the unavoidable
   cost of not knowing up front; in practice paste destinations
   either ignore a 0-byte representation or render nothing for it.

### New copy flow

```ts
const POLL_INTERVAL_MS = 30;
const POLL_DEADLINE_MS = 500;
const EMPTY_TEXT = new Blob([], { type: "text/plain" });
const EMPTY_IMAGE = new Blob([], { type: "image/png" });

function hashClip(d: TypesClipboardData | null | undefined): string {
  if (!d) return "";
  return `${d.type}:${(d.data || "").length}:${(d.data || "").slice(0, 64)}`;
}

if (isCopyKeystroke && sessionIdRef.current) {
  event.preventDefault();
  event.stopPropagation();
  const sessionId = sessionIdRef.current;

  // 1. Snapshot pre-copy clipboard hash in parallel with forwarding the
  //    keystroke. Used to detect "the value just changed". Fire-and-forget
  //    — if it fails we fall through to "first non-empty response wins".
  const beforeHashPromise: Promise<string> = apiClient
    .v1ExternalAgentsClipboardDetail(sessionId)
    .then((r) => hashClip(r.data))
    .catch(() => "");

  // 2. Forward Ctrl+C to remote (unchanged, synchronous).
  forwardCtrlCToRemote(input, event);

  // 3. Bounded poll for the new clipboard value.
  const fetchPromise: Promise<TypesClipboardData> = (async () => {
    const beforeHash = await beforeHashPromise;
    const deadline = Date.now() + POLL_DEADLINE_MS;
    let lastData: TypesClipboardData = { type: "text", data: "" };
    while (Date.now() < deadline) {
      try {
        const r = await apiClient.v1ExternalAgentsClipboardDetail(sessionId);
        lastData = r.data;
        if (hashClip(lastData) !== beforeHash) return lastData;
      } catch {
        // Ignore transient errors; loop until deadline.
      }
      await new Promise((res) => setTimeout(res, POLL_INTERVAL_MS));
    }
    return lastData;
  })();

  // 4. Per-MIME promises: real Blob if type matches, empty Blob if not.
  const textBlobPromise: Promise<Blob> = fetchPromise.then((d) => {
    if (d?.type === "text" && d.data) {
      return new Blob([d.data], { type: "text/plain" });
    }
    return EMPTY_TEXT;
  });
  const imageBlobPromise: Promise<Blob> = fetchPromise.then((d) => {
    if (d?.type === "image" && d.data) {
      const bytes = base64ToBytes(d.data);
      return new Blob([bytes], { type: "image/png" });
    }
    return EMPTY_IMAGE;
  });

  // 5. Synchronously start the gesture-anchored write.
  if (
    !isInIframe &&
    typeof ClipboardItem !== "undefined" &&
    navigator.clipboard?.write
  ) {
    const supportsImage =
      typeof ClipboardItem.supports === "function"
        ? ClipboardItem.supports("image/png")
        : true;

    const item = supportsImage
      ? new ClipboardItem({
          "text/plain": textBlobPromise,
          "image/png": imageBlobPromise,
        })
      : new ClipboardItem({ "text/plain": textBlobPromise });

    navigator.clipboard
      .write([item])
      .then(() =>
        fetchPromise.then((d) => {
          const kind = d?.type === "image" ? "image" : "text";
          showClipboardToast(`Copied ${kind}`, "success");
        }),
      )
      .catch((err) => {
        console.warn("[Clipboard] local write blocked:", err);
        showClipboardToast(
          "Copied on remote — local clipboard blocked",
          "error",
        );
      });
  } else {
    // Fallback: iframe (postMessage bridge) or no ClipboardItem support.
    fetchPromise
      .then(async (d) => {
        if (d?.type === "text" && d.data) {
          // Writes via extended postMessage (mime: "text/plain") in iframe,
          // or navigator.clipboard.writeText() in plain browsers.
          await clipboardWrite({ mime: "text/plain", text: d.data });
          showClipboardToast("Copied text", "success");
        } else if (d?.type === "image" && d.data) {
          // Same dispatch — postMessage carries mime "image/png" + base64
          // to the Wails parent, which calls SetClipboardImagePNG via cgo;
          // outside iframe, navigator.clipboard.write writes the Blob.
          await clipboardWrite({ mime: "image/png", base64: d.data });
          showClipboardToast("Copied image", "success");
        } else {
          showClipboardToast("Clipboard empty", "error");
        }
      })
      .catch((err) => {
        showClipboardToast(
          `Copied on remote — local sync failed: ${err.message}`,
          "error",
        );
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

### 4. Image clipboard — same gesture-anchored write

Images go through the **same** `clipboard.write()` call as text. We
construct a single `ClipboardItem` that declares both `text/plain` and
`image/png` synchronously, and each MIME's `Promise<Blob>` resolves
with either the real Blob (if the fetched type matches) or a zero-byte
Blob of that type (if it doesn't). The paste destination picks the
MIME it prefers.

This is the only way to support images without knowing the type up
front: we cannot make a second `clipboard.write()` call from a
non-gesture continuation, because Safari rejects it (same root cause
as the original bug).

**Trade-off**: when the user copies text on the remote and pastes into
an image-only app (e.g. Preview), Preview reads the 0-byte
`image/png` representation and produces nothing. Symmetrically, copy
an image and paste into a text-only field, the field reads an empty
string. The user can retry with the appropriate destination. This is
strictly better than today on Safari, where neither works.

**Feature-detection**: `ClipboardItem.supports("image/png")` is the
standard probe (available in Chrome 113+, Safari 16.4+). When it
returns false (very old browsers), drop the image representation and
fall through with text-only.

**iframe / postMessage bridge**: extended to carry images. See
decision 6.

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

### 6. Extend the iframe postMessage bridge to carry images

Today the bridge between the desktop stream iframe and the macOS
Wails app passes only text:

```ts
// iframe → parent
window.parent.postMessage({ type: "helix-clipboard-write", text }, "*");
window.parent.postMessage({ type: "helix-clipboard-read",  id    }, "*");
// parent → iframe
window.postMessage({ type: "helix-clipboard-response", id, text }, "*");
```

The Wails runtime exposes only `ClipboardSetText` /
`ClipboardGetText`, which is why we never wired up images. NSPasteboard
itself supports `NSPasteboardTypePNG` natively, so a small cgo binding
gets us symmetrical image support.

**Wails-side additions (`for-mac/`)**:

- New file `for-mac/clipboard_darwin.go` (cgo + AppKit, same pattern
  as `cursor_darwin.go`). Two exported methods on `*App`:

  ```go
  // SetClipboardImagePNG accepts base64-encoded PNG bytes, writes them
  // as NSPasteboardTypePNG to the general pasteboard.
  func (a *App) SetClipboardImagePNG(base64PNG string) error

  // GetClipboardImagePNG returns base64-encoded PNG bytes from the
  // general pasteboard, or "" if no image is present.
  func (a *App) GetClipboardImagePNG() (string, error)
  ```

  Implementation: `[NSPasteboard generalPasteboard]` → `clearContents`
  + `setData:forType:NSPasteboardTypePNG` for write;
  `dataForType:NSPasteboardTypePNG` for read.

- `wails build` / `wails dev` auto-regenerates the TS bindings in
  `for-mac/frontend/wailsjs/go/main/App.d.ts` — no manual binding
  registration; `*app` is already in `main.go`'s `Bind` list.

**Protocol (extended postMessage)**:

```ts
// iframe → parent: extended write
{ type: "helix-clipboard-write", mime: "text/plain", text: string }
{ type: "helix-clipboard-write", mime: "image/png", base64: string }
// iframe → parent: extended read (returns whichever type is on the
// pasteboard; parent picks based on what NSPasteboard.types contains)
{ type: "helix-clipboard-read",  id: string }
// parent → iframe: response carries the type discriminator
{ type: "helix-clipboard-response", id, mime: "text/plain", text }
{ type: "helix-clipboard-response", id, mime: "image/png",  base64 }
{ type: "helix-clipboard-response", id, mime: "empty" }
```

The old-shape `text`-only write/response remain accepted by the parent
(treated as `mime: "text/plain"`) for forward-compatibility during the
deploy window when an older Wails app sees a newer iframe or vice
versa.

**App.tsx changes**: route incoming messages by `mime`. For write
images call `SetClipboardImagePNG`. For read, query both
`ClipboardGetText` and `GetClipboardImagePNG` and pick image if
present (matches what macOS would return for a "best" type request).

**DesktopStreamViewer.tsx changes**: in the iframe code path, replace
the "image copy not supported in macOS app" error toast with a
postMessage of `mime: "image/png", base64: <data>`. In the paste path,
extend `clipboardReadText` to a `clipboardReadAny` that returns
`{ mime, data } | null` and feed an image result into the existing
`syncAndPaste({ type: "image", data: base64 }, ...)` upload.

**Size considerations**: postMessage in iframes is structured-cloned
in-process with no documented size limit (in practice limited by
memory; Chrome and Safari both handle tens of MB without trouble).
Base64-encoding a PNG inflates it ~33 %; a 4K screenshot is on the
order of 5–10 MB base64, which round-trips in <50 ms locally. The
existing HTTP path already moves the same data at the same size, so
we are not introducing a new bottleneck.

**Why now, not separate**: the reviewer explicitly asked for parity.
Once the cgo file and the protocol-with-mime are in place, the wiring
in App.tsx and DesktopStreamViewer.tsx is small. Splitting it out
would mean shipping the new ClipboardItem multi-MIME code first
with the iframe still showing an error toast, then having to revisit
the same files later — net more disruption.

## Risks and mitigations

| Risk | Mitigation |
|------|------------|
| `ClipboardItem` constructor missing on very old browsers | Feature-detect and fall back to the existing `clipboardWriteText` text-only path |
| Inner promise rejects (API 5xx, network drop) → spec says the entire `write()` fails | Catch all errors inside the inner async block, resolve to empty Blob of the declared type; the outer `.catch` on `clipboard.write()` shows an error toast |
| 500 ms poll deadline too short for a slow remote desktop | Poll returns the *last* response on timeout, so paste still works (just with whatever was on the remote clipboard previously). 500 ms is 67% longer than today's unconditional 300 ms; configurable via constant |
| User copies the same value twice — hash never changes, poll always hits the deadline | Acceptable: paste still gets the right value at deadline (it's the same value). The 500 ms latency is the cost. A server-side wait-for-change would solve this cleanly (see *Future work*) |
| Pre-copy snapshot call adds an extra round-trip before each Cmd+C | Fires in *parallel* with the synchronous Ctrl+C forward, so it doesn't delay the keystroke itself. On a 200 ms RTT link the snapshot resolves before the first poll inside the deferred async |
| Race with the 2.7-second auto-sync overwriting what we just wrote | Removed entirely — see design decision 5 |
| Removing auto-sync regresses users who copy on the remote via right-click (not Cmd+C) | Documented in `requirements.md` — user presses Cmd+C explicitly to recover; was a Chrome-only feature anyway |
| Safari user has denied clipboard permission | Caught in `.catch`, toast tells them why; matches acceptance criterion 2 |
| Paste into image-only app when remote is text (or vice versa) gets a 0-byte representation | Accepted UX cost of not knowing MIME type at gesture time; documented in design decision 4. User retries with appropriate destination |
| `base64ToBytes` choking on large images | Use the same decode loop already present in the file (it handles the existing image case); add streaming via `fetch("data:image/png;base64,...")` only if perf becomes a problem |
| Old Wails app shipped with no image-bridge handlers, new iframe sends `helix-clipboard-write` with `mime: "image/png"` | Old App.tsx checks `event.data.text` is a string — image messages fall through to no-op. Iframe never receives a `helix-clipboard-response` for image read, falls back to text-only paste path. Graceful degradation |
| New Wails app, old iframe sends old-shape `{ type, text }` writes | App.tsx keeps the old code path active (treats it as `mime: "text/plain"`). No behaviour change for unchanged iframes |
| cgo build adds Apple framework dependency in `for-mac/` | `cursor_darwin.go` already depends on AppKit (`#cgo LDFLAGS: -framework AppKit`). Same framework, no new link dependency |
| Image size > a few MB stresses base64 + postMessage path | Existing HTTP clipboard endpoint already moves the same data at the same size with no reports of issues. Add a soft cap at e.g. 32 MB with an error toast if we hit it in testing |

## Future work (out of scope for this PR)

- **Server-side wait-for-change endpoint**: add an optional
  `?wait_until_change=<hash>&timeout=500ms` query param to
  `GET /api/v1/external-agents/{sessionID}/clipboard`. Backend hooks
  into the existing GNOME D-Bus clipboard signal handler
  (`startClipboardSignalHandler` in
  `api/pkg/desktop/clipboard.go`) — or `wl-paste --watch` for
  wlroots, or X11 `XFixesSelectionNotify` — and returns the moment
  the clipboard changes (or on timeout). This eliminates the
  client-side polling loop and resolves in single-digit ms on a fast
  desktop. Touches Go + Rust/C clipboard managers; deliberately
  separate change.
- **Pre-emptive type hint**: tiny endpoint that returns just `{type:
  "text" | "image" | "empty"}` cheaply, so the frontend could pick
  the right MIME type before declaring the `ClipboardItem`. Probably
  not worth it — the dual-MIME-with-empty-blob approach is fine in
  practice.

## Files to touch

- `frontend/src/components/external-agent/DesktopStreamViewer.tsx`
  - Refactor the Cmd+C / Ctrl+C branch in `handleKeyDown` (around lines
    3905–4049) to use the ClipboardItem-with-Promise pattern with
    bounded adaptive polling and dual-MIME (`text/plain` + `image/png`).
  - Update the toast logic so failed local writes don't claim success.
  - Replace `clipboardWriteText` with `clipboardWrite({mime, text|base64})`
    so the iframe / browser dispatch can carry images as well as text.
    Update the existing call sites accordingly.
  - Extend `clipboardReadText` to `clipboardReadAny`
    returning `{ mime: "text/plain" | "image/png" | "empty", text?, base64? }`;
    feed image results into the paste-upload path.
  - Remove the `useEffect` that polls `v1ExternalAgentsClipboardDetail`
    every 2.7 s (around lines 2664–2740).
  - Remove the `lastRemoteClipboardHash` and `lastAutoSyncedText` refs
    (lines 403–404) and all references to them.
  - Remove the paste-flow short-circuit branch at ~line 4091 that
    skips the upload when `text === lastAutoSyncedText.current` — with
    auto-sync gone, the upload always happens.

- `for-mac/clipboard_darwin.go` (NEW)
  - cgo + AppKit, mirrors `cursor_darwin.go` pattern.
  - `(a *App) SetClipboardImagePNG(base64PNG string) error` — writes
    `NSPasteboardTypePNG`.
  - `(a *App) GetClipboardImagePNG() (string, error)` — reads
    `NSPasteboardTypePNG`, returns base64 or `""`.

- `for-mac/frontend/src/App.tsx`
  - Extend the `message` event handler to recognise `mime` discriminator
    in `helix-clipboard-write` and `helix-clipboard-read`.
  - For read: if `GetClipboardImagePNG` returns non-empty, respond with
    `mime: "image/png"`; else fall back to `ClipboardGetText` and respond
    with `mime: "text/plain"`; else `mime: "empty"`.
  - Keep accepting old-shape `{ type, text }` writes for back-compat.

- `for-mac/frontend/wailsjs/go/main/App.d.ts` and `.js`
  - Auto-regenerated by `wails dev` / `wails build`; do not hand-edit.
  - PR should include the regenerated files.

No Go API or generated-API-client changes.

## Test plan

Manual end-to-end (the only meaningful test for a browser-clipboard
quirk):

1. **Safari on macOS — text copy** (primary regression target).
   - Open a desktop session in Safari.
   - Select text in the remote desktop (e.g. open Terminal, type
     `echo hello world`, select "hello world"). Press Cmd+C.
   - Switch to Notes / TextEdit and Cmd+V. Expected: "hello world".
     (Today: previous clipboard contents.)
   - Toast: green "Copied text".
   - Revoke clipboard permission for the site in Safari settings,
     retry, verify the toast becomes the error variant.
2. **Safari on macOS — image copy** (new in this fix).
   - In the remote desktop, take a screenshot or open an image in
     Eye of GNOME / Files and use the app's Copy command (or
     select an image in a chat client and Ctrl+C). Press Cmd+C from
     the browser.
   - Switch to Preview → File → New from Clipboard, or paste into a
     chat app. Expected: the image pastes.
   - Toast: green "Copied image".
3. **Safari on macOS — text → image-only destination** (accepted
   regression). Copy text via Cmd+C, paste into Preview's "New from
   Clipboard". Expected: nothing or "no image on clipboard" message
   (0-byte representation). Retry by pasting into Notes — that
   works.
4. **Chrome on macOS** — verify no regression. Repeat tests 1 and 2.
5. **macOS Wails app (iframe) — text** — copy and paste still work via
   the postMessage bridge unchanged.
6. **macOS Wails app (iframe) — image copy** — copy an image on the
   remote desktop (e.g. screenshot, then GNOME Files → Copy), press
   Cmd+C in the Wails window. Switch to a native Mac app (Preview,
   Messages) and paste. Expected: the image pastes.
7. **macOS Wails app (iframe) — image paste** — copy an image on the
   Mac (e.g. screenshot to clipboard via `Cmd+Shift+Ctrl+4`), focus
   the desktop stream inside the Wails window, press Cmd+V. Expected:
   the image lands on the remote clipboard and pastes into a remote
   image-capable app (e.g. GIMP, Files → paste).
8. **Paste flows on Safari** — Cmd+V via Safari's paste button,
   native `paste` DOM event, keyboard fallback. None should
   regress.
9. **Latency** — copy on a fast local desktop, observe console
   timing logs. The poll loop should resolve in 30–90 ms on a
   healthy setup; 500 ms deadline only kicks in on a slow or
   unresponsive backend.
10. **Old Wails ↔ new iframe / new Wails ↔ old iframe** — confirm
    backward-compatibility: an older Wails app that doesn't
    understand `mime: "image/png"` simply ignores the message (text
    still works). A new Wails app that receives an old-shape
    `{ type, text }` write treats it as `mime: "text/plain"`.

## Implementation notes (added during implementation)

- **TIFF fallback in clipboard_darwin.go**: macOS screenshots-to-clipboard
  (`Cmd+Shift+Ctrl+4`) land as `NSPasteboardTypeTIFF`, not
  `NSPasteboardTypePNG`. To make image paste from the macOS Wails app
  Just Work, `GetClipboardImagePNG` falls back to reading TIFF, then
  transcodes to PNG via `NSBitmapImageRep`. Without this, paste-image
  worked from apps that put PNG on the clipboard (chat apps) but not
  from native macOS screenshots — the most common image source. The
  cost is one TIFF→PNG conversion per paste, which is fast.
- **wailsjs bindings hand-edited**: `App.d.ts` and `App.js` under
  `for-mac/frontend/wailsjs/go/main/` are normally regenerated by
  `wails dev`, but the spec-task environment doesn't run Wails. We
  hand-added entries for `SetClipboardImagePNG` and `GetClipboardImagePNG`
  matching the existing pattern. They will be re-regenerated and remain
  identical the next time anyone runs `wails dev` locally.
- **Native paste DOM event also got images**: the `handlePaste` handler
  (for macOS Edit Menu → Paste) previously only handled text via
  `event.clipboardData.getData("text/plain")`. We added a synchronous
  check for `event.clipboardData.files` with `image/png` or `image/jpeg`
  MIME, so images from native paste events also round-trip to the
  remote. JPEG support is incidental — the upload path on the server
  side accepts arbitrary blob bytes labelled as `image`, so a JPEG
  blob will still paste; if it becomes a problem we can transcode
  client-side.
- **Existing `syncAndPaste` already handles `type: "image"`**: no
  changes needed to the upload-to-remote path. The work was purely on
  the *acquisition* side (getting the image bytes out of either
  `navigator.clipboard.read()` or the new `clipboardReadAny()` bridge).
- **Untested in Safari / macOS Wails app on this machine**: there's no
  Safari, no macOS Wails build env, and no inner Helix running here.
  The TypeScript code compiles cleanly (`yarn build` passes, 21644
  modules transformed). The cgo file uses standard NSPasteboard APIs
  that match the WebKit blog's documented patterns. Manual test
  matrix in *Test plan* must be exercised by a reviewer with a Mac.

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
