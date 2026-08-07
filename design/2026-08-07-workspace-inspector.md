# T3 Code-inspired workspace inspector

**Status:** Implemented and live-verified on `feat/t3-workspace-inspector`

**Date:** 2026-08-07

**Scope:** Spec-task file browser, source-file viewer, multi-file diff viewer, and per-turn changed-files receipts in chat

## Summary

Replace the current selected-file diff screen with a read-only workspace inspector built around three connected surfaces:

- a virtualized, searchable repository tree;
- a syntax-highlighted source-file viewer;
- a virtualized, multi-file diff review surface.

The visual target is T3 Code's current right panel: dense neutral chrome, semantic diff tints, sticky file headers, syntax-colored code, and one compact surface/tab model. The implementation should use `@pierre/diffs` and `@pierre/trees` after a bounded compatibility spike. These are the libraries T3 Code uses; reproducing their syntax, virtualization, split/unified layout, and tree behavior with another hand-written renderer would be a substantial parallel implementation.

This is not only a frontend restyle. Helix's current diff endpoint can omit staged changes from its totals and can hide either branch or working-tree changes when both affect the same file. It also returns a lossy per-file projection that cannot support high-quality context expansion. The backend contract must be replaced with typed, coherent patch sources plus safe file-list and file-read operations.

Web editing is deliberately excluded from the first release. A high-quality read-only viewer has clear semantics; editing requires versioned saves, conflict handling, permissions, and agent/editor coordination that this project does not yet define.

## Implementation record

The implementation pins `@pierre/diffs` `1.3.0-beta.11` and `@pierre/trees` `1.0.0-beta.4`. The read-only paths work without T3's private Pierre editor patch. `DiffViewer` remains as a compatibility export for the two task-detail call sites, but its old manual renderer is no longer in the runtime path; it now mounts `WorkspaceInspector`.

The delivered surfaces are:

- **Changes:** a stacked, virtualized `CodeView` with coherent all/branch/working-tree scopes, historical turn selection, split/unified layouts, wrapping, whitespace filtering, refresh, sticky headers, and semantic low-chroma Git layers;
- **Files:** a virtualized, read-only Pierre tree filtered with `matchesAllTokens()`, plus syntax-highlighted file tabs and explicit binary/read-error states;
- **Chat receipts:** immutable per-interaction file summaries with the T3 auto-expansion threshold (latest turn, at most five files and 200 changed lines), compact previews for large latest turns, aggregate directory stats, expansion persistence, and deep links into the historical turn diff.

Diff and Files are first-class task-toolbar views rather than nested inspector tabs. This removes a redundant navigation row and makes the active surface visible beside Desktop and Details. Open source files still get closable tabs within the Files surface. Diff presentation controls are compact icon buttons with tooltips and accessible pressed states for unified/split layout, wrapping, and whitespace filtering.

File paths expose contextual clipboard actions. Right-clicking a Files tree row opens a keyboard-accessible menu for copying its absolute sandbox path and, for readable text files within the preview limit, its contents. Right-clicking a filename header in Diff opens the same full-path affordance without changing the active view. The workspace root comes from the authorized workspace API rather than a frontend path convention, producing agent-ready paths such as `/home/retro/work/keel/extension/credentialshelper/aws/aws.go`. Binary, truncated, loading, and failed file reads are represented explicitly rather than copying incomplete data silently.

Both Pierre `CodeView` roots are explicit scroll containers. The surrounding inspector owns the bounded panel height, while each code view owns vertical and horizontal overflow; this preserves virtualized scrolling for long source files and multi-file diffs instead of letting their internal content overflow into a clipped ancestor.

A file selected directly in the tree is already visible. Controlled-selection synchronization updates expansion and selection without issuing a second `scrollToPath`; the duplicate reveal recenters the virtualized tree after the click and moves the row away from the pointer. Files opened from Diff, a file tab, or a URL deep link carry an explicit reveal intent and use `scrollToPath(..., { offset: "nearest" })` so off-screen external selections still become visible without centering.

The backend uses shared response types and generated client methods. Live review requests return one patch per Git scope. Historical review never trusts a checkpoint ref from the browser: it resolves the interaction after session authorization and reads the stored before/after refs. Checkpoint capture uses a temporary Git index and parentless hidden commits, preserving the user's index, worktree, `HEAD`, and branch refs.

Untracked files reach the review through the same temporary-index mechanism rather than through a separate synthesised patch: one throwaway `GIT_INDEX_FILE` seeded from `HEAD` receives `git add -N -A`, and every scope that wants untracked content runs its ordinary `git diff` against it. Each scope therefore stays a single coherent patch with Git's own rename detection, blob hashes, and `numstat` counts, and the request costs two setup processes instead of one `git diff --no-index` subprocess plus a 1 MiB read per untracked path. The review endpoint is polled, so that fan-out was its dominant cost.

Checkpoint refs are pruned per session to the most recent `checkpointRefsPerSession` (200, roughly the last 100 turns). Without a bound, every turn left two permanent refs pinning the trees `git add -A` wrote, growing without limit inside a repository the user also pushes from.

Every desktop round-trip made for capture carries a real deadline. Both capture paths run on the external-agent WebSocket read goroutine, which also dispatches pongs under a 60s read deadline; an unbounded capture stalls the session's whole sync stream and, past 60s, tears the connection down. The connection deadline is what makes the timeout real — a context bounds only `Dial`. A capture that cannot finish inside the budget is recorded as a failed receipt rather than waited on, and chat renders that failure explicitly instead of silently omitting the receipt.

Boundaries retained for the first release: the browser is read-only, file reads are capped at 1 MiB, raw patch previews at 512 KiB, and file listings at 20,000 entries. The inspector can switch among detected Git workspaces, but the previous special `helix-specs` branch projection is not mixed into the repository browser. Markdown rendering, image preview, line comments, and editing remain follow-ups rather than partially implemented modes.

The public Pierre package does not expose the private lazy-context integration described in the proposal, so the unused `workspace-review/file-contents` route was removed rather than retained as a speculative API. The workspace review patch remains the sole diff contract. The superseded `diff` route and its manual assembler were removed with the old renderer. The unused HTTP input proxy was removed in favor of `ws/input`, and the unused external-agent video-stats proxy was removed while retaining Hydra's active internal stats path.

### Implementation decisions and deviations

The compatibility spike approved the Pierre packages for the read-only path and kept them in a lazy `WorkspaceInspector` chunk. Helix does not carry T3's private Pierre patch. The package's internal syntax pipeline owns tokenization; an additional Helix worker-pool wrapper was not added because it would duplicate that runtime without evidence that the default is a bottleneck.

The file tree keeps Helix's required `matchesAllTokens()` semantics instead of enabling Pierre's single-query search. It filters the flat path input, retains every ancestor, expands matching subtrees, and preserves Pierre's virtualization. Selection callbacks read the current path map and file-open handler through refs because the Pierre model retains the callback supplied at construction; closing over initial React state silently makes later-loaded rows non-interactive.

Add-to-chat context actions remain deferred because Helix does not yet have a durable file-reference message model. Copy actions are implemented through Pierre's supported context-menu composition: file rows can copy the agent-visible absolute path or complete readable contents, Diff headers can copy the absolute path, and open file tabs expose the same full-path action. The implemented navigation paths are tree row to file tab, diff header to file tab, chat **Open diff** to immutable turn diff, and chat file row to the selected file within that turn.

### Verification record

The initial inspector was verified on `helix-ubuntu:c9eca0`. The clipboard-path contract was then rebuilt and provisioned on `helix-ubuntu:8282e6`; its live `/workspaces` response exposed `/home/retro/work/keel` and did not expose the internal `/data/workspaces/...` mount. These were provisioned spec-task sessions, not seeded-session or isolated DOM tests.

The disposable workspace fixture contained:

- a committed Go file followed by an unstaged edit to the same path;
- a staged TypeScript file;
- untracked Markdown, JSON, and CSS files;
- two consecutive agent turns with independent changes.

The live source results were distinct and coherent:

| Scope | Result | Meaning |
|---|---:|---|
| Branch changes | 1 file, `+6 -0` | committed portion of `committed.go` |
| Working tree | 5 files, `+28 -0` | staged, unstaged, and untracked changes relative to `HEAD` |
| All task changes | 5 files, `+34 -0` | complete result relative to `master`, with `committed.go` represented once |

Two completed live turns persisted separate receipts: the first was 2 files, `+7 -0`; the immediately following turn was 2 files, `+7 -1`. Each card displayed the T3-style directory/file aggregation and each **Open diff** deep link rendered its own historical patch rather than the current workspace. Testing the second turn caught a lifecycle race: the synchronous before-checkpoint metadata was durable, but completion could still hold an older interaction value. Finalization now reloads the durable receipt before declaring it missing, and a regression test covers that seam.

The browser test also exercised:

- AND-token file search with automatic ancestor expansion;
- opening an untracked JSON file from the virtualized tree and rendering syntax-highlighted contents;
- combined, branch, and working-tree scope switching;
- unified/split layout, long-line wrapping, whitespace filtering, and manual refresh;
- dark and light theme adapters, including syntax foregrounds over restrained Git backgrounds;
- an 800 by 700 px viewport, where the outer task layout collapses chat and gives the inspector the available width;
- a fresh page load with no React console errors or failed workspace API requests.
- long source and multi-file diff surfaces whose `scrollTop` advanced inside their own `CodeView` roots;
- a scrolled file tree whose viewport remained unchanged after selecting an already-visible row;
- absolute-path and file-content clipboard menus, including the non-secure-origin clipboard fallback;
- collapsed and expanded in-chat changed-file receipts, folder expansion, per-file stats, and **Open diff**.

Verification commands completed successfully:

```text
cd api && go test ./pkg/desktop -count=1
cd api && go test ./pkg/server -count=1
cd api && go build ./pkg/server/ ./pkg/store/ ./pkg/types/
cd frontend && yarn vitest run \
  src/components/session/ChangedFilesCard.test.tsx \
  src/components/session/changedFilesTree.test.ts \
  src/components/workspace-inspector/WorkspaceFileTree.test.ts \
  src/components/workspace-inspector/WorkspaceInspector.test.tsx \
  src/components/workspace-inspector/clipboard.test.ts \
  src/components/workspace-inspector/pierreStyles.test.ts \
  src/components/workspace-inspector/workspaceTabs.test.ts \
  src/lib/specTaskAutoOpen.test.ts
cd frontend && yarn tsc --noEmit
cd frontend && yarn build
git diff --check
```

The final production build emits the inspector as a 780.30 KiB lazy chunk (215.31 KiB gzip); the main application chunk is 4,965.18 KiB (1,463.97 KiB gzip). Large main-chunk warnings predate this work and remain visible, while the inspector does not enter the initial route chunk.

The 10,000-entry tree and 100-file/20,000-line synthetic performance acceptance test has not been run. The implementation is virtualized and enforces server bounds, but that is architecture, not measured performance; the large-fixture measurement remains required before treating those numbers as a supported performance envelope.

### Review follow-up (post-live-verification)

A code review of this branch found defects that the live pass above did not exercise, because they need failure conditions (a slow or wedged desktop, a bounded git listing overflowing, a poll failing mid-review) rather than a working workspace. They were fixed after that live run:

- unbounded desktop I/O during checkpoint capture, on the WebSocket sync read goroutine;
- checkpoint refs and their objects accumulating without any retention bound;
- a `git diff --no-index` subprocess plus a 1 MiB read per untracked file, on a 3s-polled endpoint;
- truncation flags read from bounded `ls-files`/`numstat`/`name-status` calls and then discarded, so a partial change set reported itself as complete;
- a failed background poll replacing a rendered review with a blocking error;
- presentation preferences lost on every unmount;
- the live review polling behind an immutable turn diff;
- diff-header clicks trusting scraped Pierre DOM text as a workspace path;
- turns whose capture failed rendering nothing, indistinguishable from turns that changed no files.

Backend coverage was extended to the scenarios this design's verification plan named but the first implementation did not test: a file changed in both the branch and the working tree (the headline acceptance criterion), rename/delete/binary classification, patch and summary truncation, absolute/traversal/encoded/directory path rejection, unborn `HEAD` capture, two consecutive turns keeping independent receipts after a third edit, per-session ref pruning, and the untracked index leaving the user's staged state byte-identical. Frontend coverage was extended to the diff surface itself — previously mocked out of every test — covering stale-data-on-error, empty versus unavailable, preference persistence across remounts, and poll suppression behind a turn diff.

Re-verified after those changes: `go test ./pkg/desktop ./pkg/server -count=1`, `go build ./pkg/server/ ./pkg/store/ ./pkg/types/`, the workspace-inspector and chat-receipt Vitest suites, `yarn tsc --noEmit`, `yarn build`, and a clean Air rebuild of the running dev API. **Not re-run: the live provisioned spec-task workflow.** The Git semantics behind the untracked-index change were checked directly against real repositories (both in the new tests and by hand), but the browser-facing paths have not been re-exercised end-to-end since the fixes, and the deadline/pruning behaviour has not been observed against a real wedged desktop.

## Goals

- Show all task changes correctly, including committed, staged, unstaged, untracked, renamed, deleted, and binary files.
- Let a reviewer switch between all task changes, branch commits, and the working tree.
- Render all changed files in one scrollable review surface with unified/split and wrap controls.
- Open any repository file from the tree or from a diff header in a syntax-highlighted viewer.
- Attach a durable, expandable changed-files receipt to each completed assistant turn.
- Keep large repositories, files, and diffs responsive through virtualization and bounded payloads.
- Match T3 Code's restrained density and color hierarchy in Helix light and dark themes.
- Use typed server responses and the generated frontend API client.

## Non-goals

- Editing or saving files from the browser.
- Replacing Zed as the task's editor.
- Review comments, line annotations, or GitHub review submission in the first release.
- Terminal, browser, plan, or sub-agent tabs from T3 Code's general-purpose right panel.
- Browsing ignored build output or files outside the selected workspace.
- Replacing the existing Helix filestore browser. Workspace files and uploaded user files are different resources.

## Reference and method

The analysis is pinned to T3 Code commit [`4f5834ba72c5905a318c00456dd21271b2fa9d6f`](https://github.com/pingdotgg/t3code/tree/4f5834ba72c5905a318c00456dd21271b2fa9d6f), dated 2026-08-06. Pinning matters because the right panel is active code and has changed materially from older screenshots.

Primary source files:

- [`rightPanelStore.ts`](https://github.com/pingdotgg/t3code/blob/4f5834ba72c5905a318c00456dd21271b2fa9d6f/apps/web/src/rightPanelStore.ts)
- [`RightPanelTabs.tsx`](https://github.com/pingdotgg/t3code/blob/4f5834ba72c5905a318c00456dd21271b2fa9d6f/apps/web/src/components/RightPanelTabs.tsx)
- [`PreviewPanelShell.tsx`](https://github.com/pingdotgg/t3code/blob/4f5834ba72c5905a318c00456dd21271b2fa9d6f/apps/web/src/components/PreviewPanelShell.tsx)
- [`FileBrowserPanel.tsx`](https://github.com/pingdotgg/t3code/blob/4f5834ba72c5905a318c00456dd21271b2fa9d6f/apps/web/src/components/files/FileBrowserPanel.tsx)
- [`FilePreviewPanel.tsx`](https://github.com/pingdotgg/t3code/blob/4f5834ba72c5905a318c00456dd21271b2fa9d6f/apps/web/src/components/files/FilePreviewPanel.tsx)
- [`DiffPanel.tsx`](https://github.com/pingdotgg/t3code/blob/4f5834ba72c5905a318c00456dd21271b2fa9d6f/apps/web/src/components/DiffPanel.tsx)
- [`AnnotatableCodeView.tsx`](https://github.com/pingdotgg/t3code/blob/4f5834ba72c5905a318c00456dd21271b2fa9d6f/apps/web/src/components/AnnotatableCodeView.tsx)
- [`DiffWorkerPoolProvider.tsx`](https://github.com/pingdotgg/t3code/blob/4f5834ba72c5905a318c00456dd21271b2fa9d6f/apps/web/src/components/DiffWorkerPoolProvider.tsx)
- [`diffRendering.ts`](https://github.com/pingdotgg/t3code/blob/4f5834ba72c5905a318c00456dd21271b2fa9d6f/apps/web/src/diffRendering.ts)
- [`index.css`](https://github.com/pingdotgg/t3code/blob/4f5834ba72c5905a318c00456dd21271b2fa9d6f/apps/web/src/index.css)
- [`@pierre/diffs` documentation](https://diffs.com/docs)
- [`@pierre/trees` documentation](https://trees.software/docs)

The public marketing image was used only as a visual cross-check. Component structure, dimensions, and behavior below are source-derived.

## T3 Code architecture

### Right-panel shell

T3 does not implement files and diffs as two unrelated sidebars. It has a persisted, per-thread right-panel workspace whose ordered surfaces include a singleton diff, a singleton explorer, and one tab per open file. Terminal, browser, plan, and agent surfaces use the same tab model.

The inline panel is resizable from its left edge. Its normal constraints are a 540 px default, 360 px minimum, and 70 viewport-width-percent maximum. It can be maximized and becomes a sheet/sidebar on narrow layouts. Tabs are 24 px high, use an icon that becomes a close control on hover, and support close, close others, close right, and close all.

Helix already owns the equivalent outer layout in `SpecTaskDetailContent.tsx` through `react-resizable-panels`. Adding a second nested resizable outer shell would duplicate behavior. Helix should reuse the existing task content panel and adopt only the code-inspection surface model inside it.

### File explorer

T3 uses `@pierre/trees`, not a hand-built recursive list. Its repository model is a flat list of path/kind entries. The tree derives hierarchy, flattens empty directory chains, expands one level initially, virtualizes rows, and hides non-matches during search.

Notable mechanics:

- compact 12 px sans text and roughly 28 px rows;
- transparent tree background with 7% foreground hover, 12% selected, and 14% border blends;
- externally opened files expand their ancestors, become selected, and scroll to the center;
- internal selection and external reveal are separated to avoid reopen/feedback loops;
- file paths can be copied or inserted into chat from a context menu;
- the explorer is on the right of the file preview once a file is open, capped at 22 rem/46% and at least 256 px wide;
- with no selected file, the explorer consumes the entire surface.

The last point is important: this is not a permanent changed-files column beside every diff. The diff gets the full panel width. The file tree is contextual to browsing files.

### Source-file viewer

T3 uses the `File` and `VirtualizedFile` primitives from `@pierre/diffs`, so file preview and diff rendering share syntax themes and code metrics. It has breadcrumbs, an explorer toggle, wrap-aware source view, a rendered/source mode for Markdown, image previews, a 1 MiB read limit, and virtualization for large/truncated files. The same syntax pipeline can become an editor, but editing is a distinct mode.

This is a better fit for Helix than Monaco's diff editor. Helix already ships Monaco and should keep it for editable surfaces, but Monaco is oriented around one editor/file pair. It does not provide T3's stacked, virtualized multi-file review surface. Using Monaco for the viewer and a separate renderer for diffs would also create two syntax and theme systems.

### Diff viewer

T3 uses `@pierre/diffs` `CodeView` for one vertically stacked collection of files. Each file has a sticky 32 px header and can be collapsed. Hunk separators are 24 px. The toolbar exposes:

- change scope and base ref;
- total additions and deletions;
- refresh and collapse-all;
- unified or split layout;
- line wrapping;
- whitespace-ignore mode.

It parses raw unified patches, sorts files, hashes inputs for stable caching, and lazily loads complete old/new file contents when hidden context is expanded. Syntax work runs in a two-to-six worker pool, with long-line tokenization bounded at 1,000 characters and parsed syntax trees held in a bounded LRU cache. The surface itself is lazy-loaded.

T3 supports line selection and comments through an annotatable wrapper. Helix should not add that state until it has a durable review-comment model.

### Diff colors and density

T3 maps Pierre's shadow-DOM variables onto its semantic theme. It does not turn every added character bright green or every deleted character bright red. Syntax foreground remains syntax-colored; change type is conveyed through the line background, line-number gutter, counts, and selection marker.

In dark mode, the base is near-black (`neutral-950`), primary text is near-white, and passive borders are 6% white. Addition and deletion lines are blends of the base with semantic success/destructive colors, with stronger gutter blends. Context lines stay close to the base. File headers use 12 px sans, counts use 11 px mono, and there are no card shadows or decorative gradients.

This hierarchy is the main visual correction Helix needs. The current full-line neon green/orange text competes with syntax highlighting and makes ordinary code difficult to scan.

### In-chat changed-files tracking

T3's chat card is backed by orchestration checkpoints, not the current working-tree response. At turn completion it captures the complete workspace through an isolated temporary Git index, writes the resulting tree as a parentless commit under a hidden per-thread/per-turn ref, and diffs it against the previous turn's ref. The checkpoint summary is persisted with the turn and contains:

- turn and assistant-message identity;
- checkpoint ref and turn number;
- ready/missing/error status;
- changed path, change kind, additions, and deletions;
- completion timestamp.

The card is then a projection of immutable turn data. Opening it selects that turn in the full diff viewer, which reads the patch between the two checkpoint refs. Continued edits cannot rewrite the receipt shown under an older message.

Presentation behavior:

- header: `N changed files`, aggregate `+A -D`, expand/collapse label, and **Open diff**;
- expanded view: compacted directory tree, per-directory aggregate stats, per-file stats, and expand/collapse-all;
- latest small change auto-expands only at five files or fewer and 200 changed lines or fewer;
- a latest large change shows a compact preview with up to three representative files selected across top-level scopes;
- older collapsed changes remain one-line receipts;
- clicking a file opens that turn's diff scrolled to the file;
- expansion is stored per thread and turn, not globally.

Helix should copy these semantics, including the threshold behavior. A card backed by the current `/diff` endpoint would be incorrect: every historical message would change as the workspace changes.

## Current Helix implementation

### Frontend

The current surface is split across:

- `frontend/src/components/tasks/DiffViewer.tsx`
- `frontend/src/components/tasks/DiffContent.tsx`
- `frontend/src/components/tasks/DiffFileList.tsx`
- `frontend/src/hooks/useLiveFileDiff.ts`

It uses a fixed 280 px changed-file column and renders one selected file at a time. The line renderer classifies strings by prefixes such as `@@`, `+`, and `-`, then creates one DOM row per line. It has no syntax highlighting, intra-line changes, split layout, virtualization, or expandable hidden context. Git metadata such as mode and rename lines is treated as ordinary context.

The current color scheme applies saturated `greenRoot` and `redRoot` foregrounds to whole code lines. Status/icon colors are repeated in multiple components instead of coming from a code-review token set.

`useLiveFileDiff` polls every three seconds, casts a generic API response, converts errors to apparently successful empty state, and loads selected-file detail outside React Query. The new implementation must move these operations into a typed service and let errors remain errors.

`frontend/src/components/files/FilesSidebar.tsx` browses Helix's uploaded filestore. It has a different authorization and storage model and should not be repurposed as a Git workspace tree.

### Backend

`GET /api/v1/external-agents/{sessionID}/diff` proxies to the desktop bridge and returns a manually assembled per-file response.

The present aggregation is not a reliable model of task changes:

- committed and unstaged `numstat` results are added, but staged changes are not included in the working-tree totals;
- a selected file asks for its committed patch first and only requests its working patch if the committed patch is empty, so one side is hidden when a file has both kinds of change;
- status, counts, and patch text are computed through separate commands and can disagree;
- a lossy per-file projection cannot expand hidden context or faithfully preserve rename, mode, binary, and other patch metadata;
- branch and working-tree scopes are not independently selectable;
- responses are not represented by generated client types.

These are data-contract defects. Styling `DiffContent` cannot solve them.

## Proposed Helix design

### Component boundary

Introduce a code-inspection feature boundary rather than growing the task-specific `DiffViewer`:

```text
SpecTaskDetailContent (existing resizable content panel)
└── WorkspaceInspector
    ├── InspectorTabs
    │   ├── Changes (singleton)
    │   ├── Files (singleton until a file opens)
    │   └── file:<path> (one per open file)
    ├── DiffSurface
    │   ├── DiffToolbar
    │   └── ReviewCodeView (@pierre/diffs CodeView)
    └── FileSurface
        ├── FileBreadcrumbBar
        ├── SourceFileViewer (@pierre/diffs File/VirtualizedFile)
        └── WorkspaceFileTree (@pierre/trees, right-hand aside)

Interaction (existing chat turn)
└── TurnChangedFilesCard
    ├── TurnChangedFilesHeader
    └── TurnChangedFilesTree
```

Suggested location: `frontend/src/components/workspace-inspector/`. `DiffViewer` may become a thin call-site compatibility wrapper while the task page is migrated, but there must be only one runtime renderer after the switch.

The state is scoped by session and workspace:

- active surface;
- ordered open file tabs;
- selected workspace, diff scope, and base ref;
- unified/split, wrap, and ignore-whitespace preferences;
- whether the file explorer is open.

Persist presentation preferences (scope, unified/split, wrap, ignore-whitespace) in local storage — the inspector unmounts whenever the task switches away from Changes/Files, so component state loses the reviewer's layout on every tab switch. Encode the active mode and file in the URL (`view=changes` or `view=files&file=<relative-path>`) so task links are useful after refresh. Do not persist patch or file contents.

This deliberately stops short of cloning T3's general terminal/browser/agent surface registry. Helix needs a coherent code-inspection surface, not another application shell.

### Turn checkpoints and chat receipts

Add a checkpoint capture path for spec-task sessions with connected external-agent desktops.

At the start of a turn, capture a **before** checkpoint. At terminal completion, capture an **after** checkpoint, compute the patch between the two, derive the changed-file summary from that patch, and persist the summary on the interaction before publishing the final interaction update. Interrupted and failed turns may still have file changes; capture an after checkpoint when the workspace remains reachable and mark the receipt status accordingly.

Checkpoint refs are deterministic and isolated from user branches:

```text
refs/helix/checkpoints/<session-id>/<interaction-id>/before
refs/helix/checkpoints/<session-id>/<interaction-id>/after
```

Capture uses a temporary `GIT_INDEX_FILE` under the repository's Git common directory:

1. seed the temporary index from `HEAD` when it exists;
2. `git add -A -- .` into that index;
3. `git write-tree`;
4. `git commit-tree` with fixed Helix checkpoint author/committer identity;
5. `git update-ref` to publish the hidden ref;
6. remove the temporary index on every exit path.

The operation must never modify the real index, working tree, `HEAD`, or user branches. Parentless checkpoint commits are intentional: the ref names provide identity and `git diff <before>^{commit} <after>^{commit}` compares complete tree snapshots without inheriting branch history.

Persist the following typed shape on `types.Interaction` as JSONB:

```go
type InteractionCodeChanges struct {
    Status        string                      `json:"status"` // ready, missing, error
    Workspace     string                      `json:"workspace"`
    BeforeRef     string                      `json:"before_ref,omitempty"`
    AfterRef      string                      `json:"after_ref,omitempty"`
    PatchHash     string                      `json:"patch_hash,omitempty"`
    Files         []InteractionCodeChangeFile `json:"files"`
    TotalAdditions int                        `json:"total_additions"`
    TotalDeletions int                        `json:"total_deletions"`
    CapturedAt    time.Time                   `json:"captured_at"`
    Error         string                      `json:"error,omitempty"`
}
```

The database stores the compact summary and refs, not the full patch. The patch remains derivable from Git objects. Existing interactions have no receipt and render unchanged.

Checkpoint capture must be idempotent. Repeating the before/after request for the same interaction may replace the same hidden ref with the newly captured tree, but completion orchestration must not overwrite an already-ready receipt with a later missing/error result.

Capture must also be bounded and retained. Every desktop round-trip runs under a deadline applied to the connection itself (a context bounds only the dial), because both capture paths execute on the WebSocket sync read goroutine. And refs must be pruned per session: unbounded hidden refs pin unbounded objects in a repository the user pushes from. Seeding the temporary index tolerates an unborn `HEAD` — a workspace with no commit yet starts from an empty index rather than failing capture.

The API server should invoke dedicated desktop endpoints through the existing `dialDesktop`/`callDesktopJSON` path:

```http
POST /workspace/checkpoints/capture
POST /workspace/checkpoints/diff
```

The browser never calls capture. It reads the summary already present on the interaction and requests the turn patch through an authorized session endpoint:

```http
GET /api/v1/external-agents/{sessionID}/workspace-review/turn/{interactionID}
    ?workspace=<name>&ignore_whitespace=<bool>
```

The handler loads the interaction, verifies it belongs to the authorized session, takes refs only from the stored receipt, and asks the desktop to diff them. Client-supplied Git refs are not accepted for turn diffs.

`TurnChangedFilesCard` renders below the assistant's final response. It receives only `interaction.code_changes` and an `onOpenDiff(interactionID, filePath?)` callback. **Open diff** navigates the spec-task content panel to Changes with `scope=turn`, `interaction=<id>`, and an optional file path. The inspector then selects the turn source and reveals that file. This keeps chat presentation independent from API fetching and lets historical receipts render offline even when the desktop is unavailable.

### Interaction model

#### Changes

- Default to **All task changes**.
- Render every changed file in one stacked, virtualized surface.
- Make file headers sticky and independently collapsible.
- Clicking a file path opens that file in a file tab; it does not replace the review scroll position.
- Keep unified/split, wrap, whitespace, scope, base-ref, refresh, and collapse-all controls in one compact toolbar.
- Show explicit loading, error, empty, binary, and truncated states. Never represent an API failure as “No changes,” and never let a failed background poll blank a review the reviewer is reading: React Query retains the last successful data and flips status to error, so the blocking error state is only correct when nothing has loaded. Distinguish “no net changes in this scope” from “this scope is unavailable.”
- Remove the permanent 280 px changed-file list. A changed-files jump menu can be added later if large reviews prove hard to navigate.

Diff scopes:

| Scope | Meaning | Git basis |
|---|---|---|
| All task changes | Everything since the branch point, including the current working tree | merge base of selected base and `HEAD` to working tree, plus untracked files |
| Branch changes | Commits on the task branch | selected base `...HEAD` |
| Working tree | Staged, unstaged, and untracked changes since `HEAD` | `HEAD` to working tree, plus untracked files |

T3 exposes branch and working-tree sources. Helix should add the combined default because “Changes” on a spec task is expected to include the complete task result, not make the reviewer mentally combine two views.

#### Files

- Initially show the tree at full width.
- Selecting a file opens a file tab, puts the viewer on the left, and retains the tree on the right.
- Reveal externally opened files by expanding ancestors and scrolling only enough to bring the selected row into view. A row selected inside the tree must not issue a second reveal or change the tree's scroll position.
- Flatten empty directory chains and preserve the user's expansion state.
- Search with `matchesAllTokens()` against relative paths. Preserve matching ancestors so results remain understandable. If `@pierre/trees` cannot accept the Helix predicate, filter the flat input before updating its model; do not fall back to raw substring matching.
- Provide **Copy full path** for files and directories, using the agent-visible workspace root returned by the API, plus **Copy contents** for readable, complete text files. Do not expose the host's internal storage path.
- Opening a file from a diff header depends on scraping `@pierre/diffs`' private `[data-title]` node. Resolve that scraped text against the paths actually present in the rendered patch, so a markup change in a future beta makes header clicks inert rather than opening tabs for paths that do not exist.
- On narrow screens, use tree → viewer drill-down with a Back to files control rather than forcing a two-column minimum width.

#### File view

First release:

- syntax highlighting and line numbers;
- wrap toggle;
- breadcrumbs and copy path;
- refresh;
- virtualized rendering for large/truncated text;
- clear binary, missing, too-large, and changed-since-load states.

Follow-up additions may include rendered Markdown and image preview. Editing must remain out until a separate design defines optimistic concurrency, content hashes/version preconditions, atomic writes, dirty state, and coordination with live agent/Zed changes.

### API contract

Add typed DTOs under `api/pkg/types/workspace_review.go`, desktop-bridge handlers under `api/pkg/desktop/`, and authenticated proxy handlers alongside the current external-agent workspace routes. Resolve workspaces from the session's server-provided workspace list; never accept a client-supplied absolute working directory.

#### Review sources

```http
GET /api/v1/external-agents/{sessionID}/workspace-review
    ?workspace=<name>&base=<ref>&ignore_whitespace=<bool>
```

```go
type WorkspaceReviewResponse struct {
    Workspace   string                  `json:"workspace"`
    GeneratedAt time.Time               `json:"generated_at"`
    Sources     []WorkspaceReviewSource `json:"sources"`
}

type WorkspaceReviewSource struct {
    ID        string `json:"id"` // all, branch, working_tree
    Title     string `json:"title"`
    BaseRef   string `json:"base_ref,omitempty"`
    HeadRef   string `json:"head_ref,omitempty"`
    Patch     string `json:"patch"`
    PatchHash string `json:"patch_hash"`
    Truncated bool   `json:"truncated"`
}
```

Return coherent raw unified patches. Bound each patch at 512 KiB and report truncation explicitly — including truncation of the *summary* listings, not only the patch preview, so a bounded `ls-files`/`numstat` read can never present a partial change set as the whole one. Use no-color/no-external-diff Git invocation and make untracked files visible through a throwaway intent-to-add index so each scope stays one coherent diff. Binary patches remain metadata-only unless a later image preview endpoint is added.

The response does carry a small per-file summary (path, kind, counts) alongside the patch, deviating from the original "no file/status/count DTOs in Go" rule. Pierre still owns all *rendering*; the summary exists so the toolbar's file and +/- counts stay correct when the patch preview is truncated, which a client-side parse of a cut patch cannot do.

The all-changes implementation must diff the selected base's merge base against the working tree, not concatenate a branch patch and a working-tree patch. Concatenation duplicates files and cannot produce a coherent old/new pair.

#### Lazy diff contents

```http
GET /api/v1/external-agents/{sessionID}/workspace-review/file-contents
    ?workspace=<name>&source=<id>&base=<ref>
    &old_path=<path>&new_path=<path>&change_type=<type>
```

Return old and new text content, byte lengths, and truncation flags. This supports expanding hidden context without making the initial patch unbounded. Cap each side at 1 MiB and return a typed binary-file state rather than invalid UTF-8.

#### Repository tree

```http
GET /api/v1/external-agents/{sessionID}/workspace-files?workspace=<name>
```

```go
type WorkspaceFilesResponse struct {
    Entries   []WorkspaceFileEntry `json:"entries"`
    Truncated bool                 `json:"truncated"`
}

type WorkspaceFileEntry struct {
    Path string `json:"path"`
    Kind string `json:"kind"` // file or directory
    Size int64  `json:"size,omitempty"`
}
```

Build the MVP list from tracked and non-ignored untracked files (`git ls-files --cached --others --exclude-standard -z`) and derive directories. Bound the number of entries and return `truncated`; do not silently return an incomplete tree. Ignored artifacts are intentionally outside the first release.

#### File read

```http
GET /api/v1/external-agents/{sessionID}/workspace-file
    ?workspace=<name>&path=<relative-path>
```

Return relative path, UTF-8 contents, byte length, content hash, and truncation state. Cap reads at 1 MiB. The content hash is observational in the read-only release and becomes a precondition only if editing is designed later.

#### Path safety

Every file endpoint must:

- serve only what the tree endpoint advertises — tracked, or untracked and not ignored. Real-path containment is necessary but not sufficient: `.git` lives *inside* the workspace root, so containment alone leaves `.git/config` (which routinely carries a token in its remote URL) and `.git/credentials` readable by anyone with session read access, which through project grants includes org members who are not the session owner. Ignored files such as `.env` are the same class. Gate reads on the same `git ls-files --cached --others --exclude-standard` query the tree is built from, on both the file endpoint and diff context expansion;
- reject absolute, empty, and parent-traversal paths;
- clean and join only beneath the selected workspace root;
- resolve the workspace root and target with `realpath`/equivalent;
- reject symlinks whose resolved target escapes the workspace;
- reject directories where a file is required;
- retain the existing session authorization checks;
- avoid returning absolute sandbox paths to the browser.

Extract the repeated RevDial proxy/request handling into a helper while adding these endpoints. This should reduce duplication, not introduce a second transport path.

Use concrete Swagger response types, run `./stack update_openapi`, and consume only `api.getApiClient()` methods through `frontend/src/components/workspace-inspector/workspaceReviewService.ts`. React Query query functions extract `.data`; refresh invalidates the corresponding query. Do not poll permanently when the inspector is hidden. While visible, use a modest refetch interval and retain the manual refresh control.

### Rendering dependencies

Preferred dependencies:

- `@pierre/diffs` for patch parsing, Shiki syntax, unified/split layouts, file rendering, context expansion, and virtualization;
- `@pierre/trees` for the virtualized tree, hierarchy, reveal, selection, and icon set.

At the inspected commit, T3 pins `@pierre/diffs` `1.3.0-beta.10` and `@pierre/trees` `1.0.0-beta.4`. The current published diffs version inspected for this design is `1.3.0-beta.11`; both packages declare React 18.3 support and use the Apache-2.0 license. They are still beta dependencies. T3 also carries a local `@pierre/diffs` patch for editor-selection/export behavior. Helix must not copy that patch implicitly. Phase 0 must pin candidate versions, verify React 18.3 compatibility, and prove the required read-only paths without `patch-package`. If read-only use requires a private fork or an unexplained runtime patch, stop and reassess the dependency rather than carrying hidden vendor debt.

Both packages render important parts inside shadow DOM. Centralize their CSS-variable/`unsafeCSS` adapter in one module, document every overridden variable, and test both color modes. Do not scatter deep selectors through MUI `sx` props.

Lazy-load the inspector renderer and create one syntax worker provider per task route. Start with at most four workers, cap tokenization at 1,000 characters per line, and measure before increasing either value.

### Visual specification

Define code-review semantic tokens derived from the active MUI theme. Do not reuse Helix's saturated brand green/red as code foregrounds.

| Role | Dark direction | Light direction |
|---|---|---|
| Canvas | `#0a0a0a` | `#ffffff` |
| Primary text | `#e4e4e7` | `#27272a` |
| Muted text | `#a1a1aa` | `#71717a` |
| Border | 6% white | `#e4e4e7` |
| Hover | 3–7% white over canvas | 3–5% black over canvas |
| Addition line | restrained success blend over canvas | approximately `#edf8f0` |
| Addition gutter | stronger success blend | approximately `#d9f0df` |
| Deletion line | restrained destructive blend over canvas | approximately `#fff0f0` |
| Deletion gutter | stronger destructive blend | approximately `#f7dddd` |
| Hunk separator | slightly raised neutral | approximately `#f7f7f8` |
| Selection | muted blue/modified blend with 4 px gutter marker | same semantic treatment |

Exact dark addition/deletion hex values should be produced through `color-mix()` from the canvas and semantic success/destructive tokens, then visually and contrast-tested. Hard-coding a second disconnected palette would recreate the current problem.

Rules:

- keep syntax token foregrounds transparent to the line-state layer;
- show additions/deletions through background, gutter, count, and marker, not full-line text color;
- use JetBrains Mono at 12 px with approximately 1.55 line height for code;
- use DM Sans at 12 px for tree rows, tabs, and file headers;
- use 11 px tabular monospace figures for diff counts;
- use 32 px sticky file headers, 24 px hunk separators, 28 px tree rows, and 36–40 px primary toolbars;
- use 5–6 px radii, one-pixel separators, no shadows, and no gradients;
- use visible keyboard focus rings independent of selection color;
- verify text and controls against WCAG AA in both themes.

### Error, stale-data, and refresh behavior

- Keep the last successful patch/file visible during background refresh and show a small refreshing indicator.
- Replace it with an error state only when no successful data exists; otherwise show a non-blocking refresh error with retry.
- Hash patches and file contents. Avoid reparsing/re-tokenizing when the hash is unchanged.
- If a file changes after it was opened, refetch it and preserve scroll where possible; never pretend a stale snapshot is current.
- If the task session or desktop bridge is unavailable, distinguish that from a clean Git tree.
- Show truncation at the surface and affected file, with an explanation that the browser view is partial.

## Delivery plan

### Phase 0: dependency and rendering spike

- Pin candidate Pierre versions in an isolated branch/change.
- Render fixtures for Go, TSX, Markdown, rename, delete, binary metadata, long lines, and a large multi-file patch.
- Verify React 18.3, Vite production build, CSP/worker loading, light/dark themes, unified/split, wrapping, virtualization, and keyboard navigation.
- Confirm the required read-only behavior works without T3's local package patch.
- Record bundle impact before approving the dependencies.

Exit criterion: the chosen versions render the required fixtures in a production build with no local vendor patch.

### Phase 1: correct typed backend

- Add review, lazy-content, tree, and file-read types and routes.
- Implement all/branch/working-tree semantics, untracked files, limits, hashing, and path containment.
- Add isolated before/after checkpoint capture, interaction summary persistence, and authorized turn-diff reads.
- Add table-driven command/result and path-security tests.
- Regenerate the OpenAPI client and add the React Query service layer.

Exit criterion: fixtures covering committed, staged, unstaged, and untracked changes produce correct and distinct scopes, including a file changed in both branch and working tree. A two-turn fixture retains different immutable summaries and patches after a third workspace edit.

### Phase 2: file browser and read-only viewer

- Add the tree model, token-aware search, external reveal, breadcrumbs, file tabs, and responsive drill-down.
- Add syntax file rendering, wrap, loading/error/binary/truncated states, and hash-based refresh.
- Add full-path and text-content clipboard actions. Keep add-to-chat deferred until file references have a durable message representation.

Exit criterion: a reviewer can find and open tracked or untracked source files in a live spec-task workspace on desktop and narrow layouts.

### Phase 2b: in-chat receipts

- Add the compact changed-files tree and stat aggregation utilities.
- Render receipts under completed assistant turns with T3's latest/small auto-expansion rules.
- Wire **Open diff** and file-row actions to the corresponding turn source in `WorkspaceInspector`.
- Persist expansion state per session and interaction.

Exit criterion: two consecutive agent turns show separate durable receipts, and each opens its own historical patch at the requested file.

### Phase 3: multi-file diff review

- Add the stacked `CodeView`, scope/base controls, unified/split, wrap, whitespace mode, refresh, and collapse-all.
- Add the centralized Pierre theme adapter and semantic code-review tokens.
- Link diff headers to file tabs and preserve review scroll state.
- Remove the selected-file-only runtime renderer.

Exit criterion: all task changes are reviewable in one syntax-highlighted surface and each scope matches Git fixtures.

### Phase 4: integration and hardening

- Persist safe presentation state and add URL deep links.
- Validate screen-reader labels, focus order, keyboard operation, and responsive behavior.
- Exercise large repositories/diffs and tune limits/workers from measurements.
- Remove obsolete diff hooks/components once all call sites use `WorkspaceInspector`.
- Update the relevant user/developer documentation.

Follow-ups, in order of likely value: changed-file jump menu, Markdown/image preview, review annotations. Browser editing needs its own design and is not an automatic follow-up.

## Verification plan

### Backend tests

Use temporary Git repositories to cover:

- clean repository;
- committed-only task changes;
- staged-only, unstaged-only, and untracked-only changes;
- one file changed in both the branch and working tree;
- before/after checkpoint capture without modifying the real index, worktree, `HEAD`, or branch refs;
- idempotent capture and protection against replacing a ready receipt with an error;
- two consecutive interaction checkpoints with distinct summaries and historical patches;
- rename, copy-like content, deletion, mode change, binary file, and whitespace-only change;
- missing/invalid base ref;
- patch, tree, and file truncation;
- absolute path, `..`, encoded traversal, missing path, directory path, and symlink escape.

Assert the immediately following operation too: refresh after another Git edit must return a new hash and updated content.

### Frontend tests

- patch-source selection and base-ref changes;
- loading, stale refresh, error, empty, binary, and truncated states;
- unified/split and wrap persistence;
- file-tab/deep-link restoration;
- latest/small auto-expand, latest/large preview, and older-turn collapsed receipt behavior;
- directory/file stat aggregation and turn/file diff navigation;
- `matchesAllTokens()` filtering with preserved ancestors;
- external file reveal without selection loops;
- theme adapter in light/dark modes;
- keyboard focus and narrow-layout tree/viewer navigation.

### Required builds and live verification

- `go build ./pkg/server/ ./pkg/store/ ./pkg/types/`
- targeted desktop/server tests for the new handlers
- `cd frontend && yarn build`
- live inner-Helix test against `http://localhost:8080`

The live test must use a provisioned spec task with a connected Zed workspace. Create committed, staged, unstaged, and untracked edits, including a file changed in more than one state. Complete two agent turns that modify different paths. Verify both chat receipts and that each **Open diff** action retains its historical turn patch. Then verify all three live scopes, refresh, opening from both diff and tree, search/reveal, unified/split, wrapping, both themes, and a narrow viewport. Make another agent/editor change and verify the next refresh/open operation, not only the initial screen.

Performance acceptance should be measured with at least 10,000 tree entries and a 100-file/20,000-line patch. The tree and diff must remain virtualized rather than producing one DOM node per repository/diff line.

## Risks and decisions

| Risk | Decision |
|---|---|
| Pierre packages are beta | Run a gated spike, pin exact versions, and do not carry T3's private patch by default. |
| Syntax workers increase bundle/runtime cost | Lazy-load the inspector, share a bounded pool, and measure production output. |
| Git scope semantics become ambiguous | Name and test all three scopes; default Helix tasks to the coherent all-changes range. |
| File endpoints expose sandbox data | Resolve only known workspaces and enforce real-path containment after symlink resolution. |
| Large repositories exceed response limits | Use virtualized flat entries, explicit truncation, bounded file reads, and lazy context. |
| Web editing races with Zed/agents | Keep the first release read-only; require a separate versioned-save design before enabling writes. |
| Checkpoint capture mutates user Git state | Use an isolated temporary index and hidden refs; test that the real index, worktree, `HEAD`, and branch refs are byte-for-byte unchanged. |
| Historical chat receipts drift | Persist per-interaction summaries and deterministic before/after refs; never derive old cards from the live working tree. |
| A T3 clone overfits Helix | Reuse the existing task panel and implement only Changes, Files, and file tabs. |

## Acceptance criteria

- No legitimate Git change category is silently omitted from the default task view.
- A file changed in committed and uncommitted states is represented coherently, not by whichever patch was queried first.
- The diff is multi-file, syntax-highlighted, virtualized, and supports unified/split and wrapping.
- The workspace tree is searchable, virtualized, reveal-aware, and cannot escape its workspace root.
- Source files use the same syntax/theme pipeline as diffs and handle large/binary content explicitly.
- Every completed turn with code changes has an immutable chat receipt whose file action opens that turn's patch.
- Added/deleted code retains syntax foreground colors; change type is communicated through restrained semantic layers.
- All new frontend calls use generated API methods and React Query.
- The old manual line renderer is no longer in the runtime path.
- The production frontend build, backend build/tests, and live inner-Helix workflow pass before merge.
