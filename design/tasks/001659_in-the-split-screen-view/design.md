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
