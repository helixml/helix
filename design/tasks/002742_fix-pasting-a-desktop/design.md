# Design: Stop Desktop Clipboard Placeholder PNG Hijacking Chat Paste

## Overview

Three changes, all in `frontend/src`:

1. Extract the sentinel PNG into a shared module (single source of truth).
2. Filter the sentinel out of the clipboard→attachment path, so a text copy from the
   desktop pastes as text in both chat inputs.
3. Best-effort re-write the local clipboard as text-only on the copy side, so the
   sentinel stops leaking to third-party apps at all.

Change 2 is the fix; changes 1 and 3 are what make it correct and durable. Change 2 alone
is sufficient to close the reported bug, which is why it is the primary fix — it protects
against *any* clipboard producer, not just ours.

## Codebase notes (for future agents)

Things worth knowing before touching this area:

- **The dual-MIME `ClipboardItem` is load-bearing, do not "simplify" it.**
  `DesktopStreamViewer.tsx:~4295-4325` declares `text/plain` *and* `image/png` up front
  with unresolved promises because Safari's Async Clipboard API requires the
  `navigator.clipboard.write()` call itself to happen inside the user-gesture task, and
  at that moment we do not yet know whether the remote copy produced text or an image
  (we discover it by polling `/external-agents/{id}/clipboard` for up to 500ms).
- **The 70-byte placeholder is also load-bearing.** Commit `2161143e2` introduced it
  because Chrome decodes/sanitises every image representation and rejects the *whole*
  write if one fails — a zero-byte `image/png` blob silently killed all text copy on
  Chrome. Do not replace it with an empty Blob.
- **The precedent already exists.** `DesktopStreamViewer.tsx:165-175`
  (`clipboardReadAny`, the paste-*into*-desktop path) already reads `text/plain` before
  `image/png` with a comment naming this exact placeholder. This task applies the same
  reasoning to the chat inputs.
- **Genuine image copies from the desktop carry an empty `text/plain`**, per that same
  comment — which is why the existing desktop paste path's "text first" rule does not
  break image paste there.
- Frontend test runner is **vitest** (`yarn test` → `vitest run`). Existing paste tests in
  `RobustPromptInput.test.tsx` build synthetic `clipboardData` objects
  (`{ files: [...], items: [], getData: () => '' }`) and fire them via
  `fireEvent.paste(textarea, { clipboardData })` — reuse that shape.
- Vite HMR is live on the inner Helix (`helix-frontend-1`, port 8081, proxied by 8080), so
  frontend edits are testable in the browser immediately without a rebuild.

## Change 1 — shared sentinel module

New file: `frontend/src/components/common/clipboardPlaceholder.ts`

```ts
// Minimal valid 1x1 transparent PNG. <full existing rationale comment moved here>
export const PLACEHOLDER_PNG_BASE64 = "iVBORw0KGgo…RK5CYII="

// Derived, never hardcoded, so the constant and the size check cannot drift.
export const PLACEHOLDER_PNG_BYTE_LENGTH = <decoded length of the base64 above>

export function isPlaceholderPngCandidate(file: File): boolean
export async function isPlaceholderPng(file: File): Promise<boolean>   // exact bytes, test-only if unused
```

- `DesktopStreamViewer.tsx` deletes its local `const PLACEHOLDER_PNG_BASE64` and imports
  from here. Its `base64ToBytes` helper stays where it is (it has other callers in that
  file); the shared module derives the byte length itself.
- The base64 string must appear exactly once in the tree afterwards
  (`grep -rn iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ frontend/src` → 1 hit).
- The decoded length is 70 bytes (96 base64 chars, two `=` pad → `96/4*3 - 2`). Compute it
  at module load from the constant rather than writing `70`.

## Change 2 — drop the sentinel on the paste side (primary fix)

### `chatAttachments.ts`

`filesFromClipboard(data: DataTransfer): File[]` gains a filter step:

```ts
export function filesFromClipboard(data: DataTransfer): File[] {
  const directFiles = Array.from(data.files)
  const files = directFiles.length > 0 ? directFiles : <items path, unchanged>
  return files.filter((file) => !isPlaceholderPngCandidate(file))
}
```

Filtering inside `filesFromClipboard` (rather than at each call site) means every present
and future consumer is protected by construction. `filesFromClipboard` currently has one
caller (`RobustPromptInput`); `InferenceTextField` gets migrated onto it (below), giving
two.

`isPlaceholderPngCandidate` is the **cheap synchronous** match:
`file.type === 'image/png' && file.size === PLACEHOLDER_PNG_BYTE_LENGTH`.

**Decision — no async byte confirmation.** The paste handler must decide whether to call
`preventDefault()` synchronously; an async byte comparison cannot gate that decision
without either double-handling the event or attaching-then-removing (a visible flash).
The sync match is not a vague heuristic — it is keyed to the exact byte length of the
sentinel we ourselves produce, and a 70-byte PNG can only encode a 1x1 single-colour
pixel, so a false positive costs the user nothing observable. `isPlaceholderPng` (exact
bytes, async) is provided and asserted in unit tests so the constant and the length stay
verifiably in sync; if it ends up with no production caller it is not shipped as dead
code — the length constant is derived from the base64 directly and the exact-bytes
assertion lives in the test file instead. (Open Question 2 flags this for review.)

### `RobustPromptInput.tsx` `handlePaste`

Structurally unchanged — it already calls `filesFromClipboard` first, and that now returns
`[]` for a sentinel-only clipboard, so control falls through to the existing text path and
the browser's default text insertion. Add a comment stating the rule:

> Files win when the clipboard carries a real file. The desktop-stream copy path writes a
> sentinel 1x1 PNG alongside every text copy (see clipboardPlaceholder.ts); that sentinel
> is stripped in filesFromClipboard, so a text copy reaches the text branch below. We do
> NOT prefer text over a *real* image when both are present: Finder/Explorer file copies
> carry the filename in text/plain and Chrome's "Copy image" carries text/html, so a
> blanket text-wins rule would turn legitimate file pastes into filename strings.

**Decision — no blanket text-over-image preference.** The brief suggested erring toward
text when both are present, mirroring the desktop paste path. That rule is safe *there*
because the desktop paste path only ever sees clipboards our own copy handler wrote. The
chat inputs see every clipboard on the machine, where text+file is the normal shape of a
Finder copy. Dropping the sentinel already fully resolves the reported bug; a broader rule
would trade it for a new one. Flagged as Open Question 1.

### `InferenceTextField.tsx` `handlePaste`

Currently hand-rolls its own `items` loop filtering `kind === 'file'` && `type` starts
with `image/`. Replace that loop with `filesFromClipboard(event.clipboardData)` +
`.filter((f) => f.type.startsWith('image/'))`. This inherits the sentinel drop and removes
a duplicated clipboard-walk. Behaviour otherwise identical: still only swallows the paste
when at least one image remains, still previews the last image, still lets text fall
through.

Note this component reads `event.clipboardData.items` only and ignores `.files`; routing
it through `filesFromClipboard` also picks up the `.files` list, which is a strict
improvement (it makes drag-copied images from Finder work) and matches the other input.

## Change 3 — copy side: re-write as text-only, best-effort

In `DesktopStreamViewer.tsx`, in the `!isInIframe && ClipboardItem` branch, the existing
success handler already does `fetchPromise.then((d) => …toast…)`. Extend that same chain:

```ts
navigator.clipboard.write([clipboardItem])
  .then(() => fetchPromise)
  .then(async (d) => {
    const kind = d?.type === "image" ? "image" : "text";
    // The gesture-anchored write above had to declare image/png up front, so a
    // text copy leaves the sentinel placeholder on the clipboard. Now that we
    // know it was text, re-write text-only so no other app ever sees it.
    // Strictly best-effort: Chrome's transient activation (~5s) outlasts the
    // 500ms poll deadline so this usually lands; Safari will reject it, which
    // is fine — the paste-side sentinel drop covers that case.
    if (kind === "text" && d?.data) {
      await navigator.clipboard.writeText(d.data).catch(() => {});
    }
    showClipboardToast(`Copied ${kind}`, "success");
  })
  .catch(…unchanged…)
```

Key constraints encoded here:
- Chained **after** `write()` resolves, never in parallel — otherwise the slower
  gesture-anchored write could land last and restore the sentinel.
- Only for `kind === "text"`; an image copy must never be clobbered.
- `.catch(() => {})` on the `writeText` alone, so a rejection cannot flip the toast to the
  error branch or mark a successful copy as failed.
- The iframe / no-`ClipboardItem` fallback branch is untouched — it already resolves the
  fetch first and writes a single MIME, so it never produces the sentinel.

## Files touched

| File | Change |
|---|---|
| `frontend/src/components/common/clipboardPlaceholder.ts` | **new** — sentinel constant, derived byte length, matchers |
| `frontend/src/components/common/chatAttachments.ts` | `filesFromClipboard` drops the sentinel |
| `frontend/src/components/common/RobustPromptInput.tsx` | comment documenting the files-vs-text rule |
| `frontend/src/components/create/InferenceTextField.tsx` | use `filesFromClipboard`, drop the hand-rolled items loop |
| `frontend/src/components/external-agent/DesktopStreamViewer.tsx` | import shared constant (delete local copy); best-effort text-only re-write |
| `frontend/src/components/common/RobustPromptInput.test.tsx` | sentinel / real-image / large-text paste cases |
| `frontend/src/components/common/chatAttachments.test.ts` | `filesFromClipboard` sentinel-drop unit tests |
| `frontend/src/components/common/clipboardPlaceholder.test.ts` | **new** — asserts the constant decodes to exactly 70 bytes |

## Risks

- **False-positive sentinel drop.** A genuine 70-byte `image/png` on the clipboard is
  silently discarded. Such a PNG can only be a 1x1 single-colour pixel; accepted.
- **`InferenceTextField` behaviour drift.** Routing it through `filesFromClipboard` makes
  it also honour `clipboardData.files`. Intentional and an improvement, but it is a
  behaviour change beyond the bug — call it out in the PR description.
- **Copy-side re-write timing.** If Chrome's transient activation has expired by the time
  the poll resolves, `writeText` rejects and we fall back to the sentinel-on-clipboard
  state — which the paste-side fix handles. No user-visible failure either way.

## Implementation notes (written during implementation)

- **`filesFromClipboard` had exactly one caller before this change.** Filtering inside it
  (not at call sites) was the right seam: `InferenceTextField` was then migrated onto it,
  so both chat inputs inherit the sentinel drop from one place.
- **`InferenceTextField` previously ignored `clipboardData.files`** entirely — it only
  walked `.items`. Routing it through `filesFromClipboard` also picks up `.files`. That is
  a small behaviour improvement beyond the bug fix; called out in the PR description.
- **The three new `RobustPromptInput` tests were confirmed to be genuine regression
  tests**: with the one-line filter in `filesFromClipboard` reverted, all three fail; with
  it restored, all three pass. A test that passes both ways would have been worthless here.
- **Asserting "the text was inserted" in jsdom is not possible** — jsdom does not implement
  the browser's native paste insertion. The tests instead assert `event.defaultPrevented
  === false` (via `createEvent.paste` + `fireEvent`), which is the precise contract: the
  handler must not swallow the event, leaving native insertion to the browser. The live
  browser check below confirms the user-visible half.
- **`PLACEHOLDER_PNG_BYTE_LENGTH` is computed with `atob()` at module load**, not
  hardcoded as `70`. `clipboardPlaceholder.test.ts` asserts it equals 70 and that the bytes
  carry a valid PNG signature and trailing `IEND`, so the constant and the size-based match
  cannot silently drift.

### Environment gotchas hit in this sandbox (worth knowing next time)

- `frontend/node_modules` was absent; `yarn install` takes ~3 min.
- **`yarn tsc` emits compiled output into `frontend/lib/`** (gitignored). `yarn test`
  (`vitest run`) then collects those compiled `.js` duplicates and reports ~111 bogus
  failures ("Vitest cannot be imported in a CommonJS module using require()"). Remove/move
  `frontend/lib` before running the suite, or run `vitest` on `src/` paths directly.
- **`frontend/dist` is a root-owned bind mount** that the `retro` user cannot write to, so
  plain `yarn build` cannot write its output here. Build with
  `npx vite build --outDir /tmp/dist-check` to validate bundling without touching the mount
  (and never `rm -rf frontend/dist` — CLAUDE.md).
- This host was extremely contended during the run (load average 130–316 on 4 CPUs). Vite
  builds were OOM-killed (`Killed`), the frontend container's esbuild worker died and
  needed `docker compose -f docker-compose.dev.yaml restart frontend`, and `docker exec`
  calls intermittently timed out at 240s.
- The inner Helix had **no default agent provider/model configured**, so project creation
  failed with "Failed to create agent" (API: `default new project agent provider and model
  are not configured in Admin > System Settings`). There is no `/dashboard` or
  `/admin/settings` route in this build, so it was set directly:
  `UPDATE system_settings SET default_new_project_agent_provider='anthropic',
  default_new_project_agent_model='claude-opus-5' WHERE id='system';`
  (`claude-opus-5` is what the configured `ANTHROPIC_BASE_URL` proxy serves.)
- **`POST /external-agents/{id}/clipboard` cannot be used to fake a remote text copy.** On
  GNOME the bridge writes the selection over D-Bus RemoteDesktop `SetSelection`; the
  desktop then owns the selection, and the matching `GET` (which is what the copy handler
  polls) reads back empty. The POST returns 200 and still leaves the poll seeing `{"type":
  "text","data":""}`. A real copy performed by an app inside the desktop is the only way to
  drive that path.

## Verification plan

1. `cd frontend && yarn tsc && yarn build`.
2. `cd frontend && yarn test`.
3. `grep -rn 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ' frontend/src` → exactly 1 hit.
4. Inner Helix (`http://localhost:8080`) end-to-end with `chrome-devtools` MCP: register
   `test@helix.ml` / `helixtest` → onboarding → create spec task → detail page with
   desktop stream → Cmd+C text inside the desktop → paste into chat → assert text lands
   and the attachment tray is empty. Then repeat with an image copy inside the desktop and
   assert it still attaches as an image.
5. If any link of step 4 is genuinely unavailable in the sandbox, synthesise the clipboard
   at the live chat input via devtools (`DataTransfer` with a `text/plain` item plus the
   exact 70-byte PNG `File`, dispatched as a real `paste` event) — and say explicitly in
   the report that this substitution was made, and still verify the copy-side change
   against the live `DesktopStreamViewer` (console log / `navigator.clipboard.read()`
   showing text-only after a text copy).
6. Report exactly what was run and what was not.
