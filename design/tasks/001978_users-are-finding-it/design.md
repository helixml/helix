# Design

## Current State

`SpecTasksPage.tsx` (lines 812–1022) renders the project header. The top-right area contains direct buttons (Open Desktop, Resume/Stop, New Chat) followed by a single `IconButton` with the `MoreHorizontal` (lucide) icon (lines 933–938). Clicking it opens an MUI `Menu` (lines 940–1020) with:

1. Files
2. **Settings** ← buried here, line 973
3. Sharing
4. Show/Hide Archived Tasks
5. Show/Hide Metrics
6. Show/Hide Merged

Settings opens a modal dialog via `openDialog('project-settings', { projectId })`. The dialog itself (`ProjectSettings.tsx`) is fine; the problem is purely the entry point.

## Brainstormed Options

Five options, ranked from minimal to most ambitious. Recommendation: **Option B** as the default, with Option C as a stretch if we want to do more.

### Option A — Tooltip-only (smallest possible fix)

Add a `<Tooltip title="More options">` around the existing `MoreHorizontal` button, and make the icon slightly larger (e.g. 22px). One-line change.

- Pros: Trivial, zero risk.
- Cons: Settings is still two clicks away. Doesn't really fix discoverability.

### Option B — Promote Settings out of the menu (recommended)

Add a dedicated `IconButton` with the MUI `Settings` (gear) icon to the right of the "New Chat" button, before the kebab. Wrap with `<Tooltip title="Project settings">`. Keep the kebab menu for everything else, but **remove** the Settings entry from it (Files, Sharing, view toggles stay). Also add a tooltip to the kebab.

- Pros: Settings is one click and uses the universally-recognised gear icon. Header stays compact. Existing kebab pattern preserved for less-common items.
- Cons: Adds one more icon to the header row. No URL change (still a modal).

### Option C — Settings + Sharing both promoted

Same as Option B, plus promote `Sharing` (a `PersonAdd` or `Share` icon button). Both are "high signal" actions a user typically wants without hunting.

- Pros: Mirrors common SaaS patterns (Notion, Linear, GitHub).
- Cons: Three icon buttons + kebab can feel cluttered; risk on smaller desktop widths.

### Option D — Replace kebab with a labelled "Settings" button

Drop the kebab entirely. Show a `<Button startIcon={<SettingsIcon/>}>Settings</Button>` and move Files/Sharing/view-toggles into the settings dialog or its own labelled menu.

- Pros: Most discoverable.
- Cons: Bigger refactor; toggle items (Show Archived, Show Metrics, Show Merged) don't naturally belong in a settings dialog. Risk of clutter.

### Option E — Move settings to a real `/project/:id/settings` route

Replace the modal with a routed page, accessed via a sidebar link or breadcrumb action.

- Pros: Web-standard UX, deep-linkable, matches sidebar nav patterns.
- Cons: Out of scope per requirements; large change to dialog system; would touch many files.

## Recommended Design (Option B)

In `SpecTasksPage.tsx`, in the header `Stack` (around line 932, just before the existing `IconButton`):

```tsx
<Tooltip title="Project settings">
  <IconButton
    size="small"
    onClick={() => openDialog('project-settings', { projectId })}
    aria-label="Project settings"
  >
    <SettingsIcon fontSize="small" />
  </IconButton>
</Tooltip>
```

Then:

1. Wrap the existing `MoreHorizontal` button in `<Tooltip title="More options">`.
2. Remove the `Settings` `MenuItem` (lines ~970–981) from the kebab menu, since it now has a dedicated button.
3. Import `SettingsIcon` from `@mui/icons-material/Settings` (already used elsewhere in the codebase).

## Key Decisions & Rationale

- **Gear icon over text label**: keeps the header tight; gear is universal.
- **Keep the kebab**: view-toggle items (Show Archived/Metrics/Merged) are not "settings" — they are view state. Putting them in a kebab "more options" menu is correct.
- **Tooltip on the kebab**: even if we don't go further, a tooltip on `MoreHorizontal` is free and removes mystery-meat.
- **Don't touch `Projects.tsx`**: the right-click menu on cards is a separate UX and works fine.

## Implementation Notes (after the fact)

- **Used the lucide `Settings` icon, not MUI's**, because lucide `Settings` was already imported in `SpecTasksPage.tsx` (line 32) and was the icon previously used in the kebab menu item — keeps the visual identical.
- Header `Box` was `display: { xs: "none", md: "block" }` with a single child; with two children we changed it to `display: flex` + `alignItems: "center"` + `gap: 0.5` for clean spacing.
- Wrapped the new gear in `{projectId && (...)}` because `openDialog('project-settings', { projectId })` requires a non-null `projectId` — same guard the original menu item had.
- `cd frontend && yarn build` succeeded (~36s); no type errors.
- **Couldn't take a runtime screenshot**: the inner Helix at `localhost:8080` wasn't running. The startup script (`/home/retro/work/helix-specs/.helix/startup.sh`) failed during `docker build` of the API image with a pre-existing compile error in `pkg/openai/manager/provider_manager.go` (lines 350–351: `m.runnerController undefined`). Unrelated to this change.
- Single-file change: `frontend/src/pages/SpecTasksPage.tsx` (+27/-18).

## Codebase Notes (for implementer)

- UI library: **MUI v5** (`@mui/material`, `@mui/icons-material`). Mixed with `lucide-react` for some icons.
- The dialog system uses a `useDialog()` hook with `openDialog('project-settings', { projectId })`. Settings dialog content lives in `ProjectSettings.tsx` and is rendered via `settingsDialog.tsx`.
- Header is desktop-only: wrapped in `display: { xs: "none", md: "flex" }`. Don't break that.
- Existing `IconButton` patterns in the file already use `size="small"` and `<Tooltip>` wrappers (e.g. line 845).
