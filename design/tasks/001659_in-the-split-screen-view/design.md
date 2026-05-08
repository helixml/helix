# Design: Split-Screen Tab Independence

## Architecture

`SpecTaskDetailContent` (`frontend/src/components/tasks/SpecTaskDetailContent.tsx`) manages its view with:

```typescript
const [currentView, setCurrentView] = useState<"chat" | "desktop" | "changes" | "details">(getInitialView);
```

The bug is two-way URL sync:
1. **Init**: reads `router.params.view` to set initial state (line ~295)
2. **Effect**: watches `router.params.view` changes and calls `setCurrentView` (lines ~312-325)
3. **Handler**: calls `router.mergeParams({ view: newView })` on every tab change (line ~332)

When two instances exist, step 3 from one triggers step 2 in the other.

## Fix

Add an `isInSplitScreen` prop (or detect it via context) to `SpecTaskDetailContent`. When `true`, skip the URL param sync:

- Do NOT call `router.mergeParams({ view: newView })` on tab change
- Do NOT `useEffect` watch `router.params.view` to update local state

The view state becomes purely local in split-screen mode.

### Option A: Prop-based (preferred — simple, explicit)

`TabsView` already renders `SpecTaskDetailContent` as tab content. Pass `isInSplitScreen={true}` (or `syncViewWithUrl={false}`) when rendering inside a panel.

```typescript
// In TabsView.tsx, where SpecTaskDetailContent is rendered:
<SpecTaskDetailContent ... syncViewWithUrl={false} />
```

```typescript
// In SpecTaskDetailContent.tsx:
interface SpecTaskDetailContentProps {
  syncViewWithUrl?: boolean; // default true
  // ...existing props
}

// Guard all router.mergeParams calls:
if (syncViewWithUrl) {
  router.mergeParams({ view: newView });
}

// Guard the useEffect:
useEffect(() => {
  if (!syncViewWithUrl) return;
  // existing URL sync logic
}, [router.params.view, syncViewWithUrl]);
```

### Option B: Context-based (if prop threading is hard)

Add a React context `SplitScreenContext` that `TabsView` provides, and `SpecTaskDetailContent` consumes to determine whether to sync URL.

**Prefer Option A** — the prop is straightforward and `TabsView` already controls the render site.

## Where to Find the Render Site in TabsView

Search for where `SpecTaskDetailContent` is rendered inside `TabsView.tsx`. It will be inside the tab content renderer, likely conditional on tab type.

## No Other State Sharing

The `activeTabId` per panel in `rootNode` is already local to each panel — only the URL sync causes the cross-panel bleed. No other shared state changes are needed.

## Implementation Notes

**Files modified:**
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx`
  - Added `syncViewWithUrl?: boolean` prop (default `true`).
  - `getInitialView()`: returns `"desktop"` immediately when `syncViewWithUrl` is false (does not read `router.params.view`).
  - URL-watching `useEffect`: early-return when `syncViewWithUrl` is false; included `syncViewWithUrl` in deps.
  - `handleViewChange`: only calls `router.mergeParams({ view })` when `syncViewWithUrl` is true.
  - "Default to appropriate view" `useEffect` (around old line 522): both branches now guard `router.mergeParams` with `syncViewWithUrl`.
- `frontend/src/components/tasks/TabsView.tsx`
  - Pass `syncViewWithUrl={false}` to `SpecTaskDetailContent` rendered inside a panel.

**Verification:**
- Frontend type-check (`npx tsc --noEmit`) passes with no errors.
- Vite transformed 21400 modules with no TS/lint errors (final write to `dist/` failed only due to bind-mount permissions in this environment, unrelated to the change).
- Code-path analysis:
  - Split-screen (`syncViewWithUrl={false}`): every code path that touches the URL param is gated; each `SpecTaskDetailContent` instance owns `currentView` purely locally → panels are independent.
  - Single-panel (`SpecTaskDetailPage` keeps default `syncViewWithUrl=true`): all original behaviour preserved (URL init, URL watcher, URL writes on change).
- Live browser verification was not possible in this session — the inner Helix stack containers were not running (Docker reports no containers; startup script still mid-build).

**Gotchas:**
- The 'Default to appropriate view' effect can still trigger a setCurrentView in split-screen (e.g. when `activeSessionId` toggles); only the URL write is suppressed. This is the desired behaviour.
- Do NOT remove the `setCurrentView(...)` calls when guarding — only the `router.mergeParams` calls.
