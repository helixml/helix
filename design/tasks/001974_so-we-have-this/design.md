# Design

## Affected Files

- `frontend/src/components/spec-tasks/DesignReviewContent.tsx` — owns `viewedTabs`, `viewedContentRef`, `activeTab`, and renders `ReviewActionFooter`.
- `frontend/src/components/spec-tasks/ReviewActionFooter.tsx` — renders the Approve / Reject / Request Changes buttons.

No backend, no API client, no new types/endpoints. No new state management primitives.

## Current Behaviour (relevant snippets)

`DesignReviewContent.tsx:311-330` — the unread-marking effect:

```ts
useEffect(() => {
  if (!review) return;
  const tabs: DocumentType[] = ["requirements", "technical_design", "implementation_plan"];
  const invalidated: DocumentType[] = [];
  for (const tab of tabs) {
    const snapshot = viewedContentRef.current.get(tab);
    if (snapshot !== undefined && snapshot !== getTabContent(tab)) {
      invalidated.push(tab);
      viewedContentRef.current.delete(tab);
    }
  }
  if (invalidated.length > 0) {
    setViewedTabs(prev => {
      const next = new Set(prev);
      for (const tab of invalidated) next.delete(tab);
      return next;
    });
  }
}, [review?.requirements_spec, review?.technical_design, review?.implementation_plan]);
```

`ReviewActionFooter.tsx:96-114` — the Approve button:

```tsx
<Tooltip title={!allTabsViewed ? `Review all tabs before approving: ${unviewedTabNames.join(', ')}` : ''} placement="top">
  <span>
    <Button variant="contained" color="success" onClick={onApprove}
            disabled={unresolvedCount > 0 || !allTabsViewed}>
      Approve Design
    </Button>
  </span>
</Tooltip>
```

## Change 1 — Don't mark the active tab as unread

In the invalidation effect, skip the active tab. When the active tab's content has changed, instead of removing it from `viewedTabs`, **refresh its snapshot** to the new content so subsequent comparisons treat it as up-to-date.

```ts
useEffect(() => {
  if (!review) return;
  const tabs: DocumentType[] = ["requirements", "technical_design", "implementation_plan"];
  const invalidated: DocumentType[] = [];
  for (const tab of tabs) {
    const snapshot = viewedContentRef.current.get(tab);
    if (snapshot === undefined) continue;
    if (snapshot === getTabContent(tab)) continue;

    if (tab === activeTab) {
      // User is currently viewing this tab — they ARE seeing the change.
      // Refresh the snapshot so we don't flag it later either.
      viewedContentRef.current.set(tab, getTabContent(tab));
      continue;
    }
    invalidated.push(tab);
    viewedContentRef.current.delete(tab);
  }
  if (invalidated.length > 0) {
    setViewedTabs(prev => {
      const next = new Set(prev);
      for (const tab of invalidated) next.delete(tab);
      return next;
    });
  }
}, [review?.requirements_spec, review?.technical_design, review?.implementation_plan, activeTab]);
```

Notes:

- `activeTab` is added to the dependency array so that if the user happens to switch tabs at the exact instant new content arrives, the effect re-evaluates with the correct active tab. (`activeTab` is a string literal — cheap to compare, won't cause render thrash.)
- The snapshot refresh keeps the `viewedTabs` Set membership intact (the tab is already in it — by definition of being the active tab the user has interacted with it).
- Edge case: if the active tab is the initial-mount default `"requirements"` and content arrives before the user has ever interacted, the existing `viewedContentRef.current.set("requirements", ...)` block at lines 305-309 already snapshots it; the new logic refreshes that snapshot in place. No regression.

## Change 2 — "Next Document" button when tabs are unread

### Where the logic lives

The "next unread tab" calculation lives in `DesignReviewContent.tsx` (it has `activeTab`, `viewedTabs`, `ALL_TABS`). `ReviewActionFooter` stays a presentational component — we extend its props rather than giving it knowledge of tabs.

### New props on `ReviewActionFooter`

```ts
interface ReviewActionFooterProps {
  // ...existing props...
  onNextDocument?: () => void;     // called when user clicks "Next Document"
  hasNextDocument?: boolean;       // true when there's an unread tab to jump to
}
```

### Button rendering logic in `ReviewActionFooter`

Replace the single Approve `<Tooltip>+<Button>` block with a conditional:

```tsx
{hasNextDocument && unresolvedCount === 0 ? (
  <Button
    variant="contained"
    color="primary"
    onClick={onNextDocument}
  >
    Next Document
  </Button>
) : (
  <Tooltip title={getApproveTooltip()} placement="top">
    <span>
      <Button
        variant="contained"
        color="success"
        onClick={onApprove}
        disabled={unresolvedCount > 0 || !allTabsViewed}
      >
        Approve Design
      </Button>
    </span>
  </Tooltip>
)}
```

`getApproveTooltip()` returns the unresolved-comments message when `unresolvedCount > 0`, otherwise empty (the `!allTabsViewed` branch is unreachable here because that case is handled by the "Next Document" button).

`hasNextDocument` is just `!allTabsViewed` from the parent's perspective — pass it through directly. No need for a separate prop name, but keeping it semantically named makes the footer easier to read.

### `onNextDocument` handler in `DesignReviewContent`

```ts
const handleNextDocument = () => {
  // Find next unread tab in canonical tab order, starting after the active tab
  const order: DocumentType[] = ALL_TABS;
  const startIdx = order.indexOf(activeTab);
  for (let i = 1; i <= order.length; i++) {
    const candidate = order[(startIdx + i) % order.length];
    if (!viewedTabs.has(candidate)) {
      handleTabChange(candidate);
      return;
    }
  }
  // No unread tabs — defensive no-op (button shouldn't be visible in this state).
};
```

Reuses the existing `handleTabChange` so snapshot capture, scroll-to-top, and `viewedTabs` update happen the same way as a manual click.

### Wiring

In `DesignReviewContent`, where `<ReviewActionFooter ... />` is rendered, add:

```tsx
onNextDocument={handleNextDocument}
hasNextDocument={!allTabsViewed}
```

### Why this UX, not the obvious alternatives

- **Why not just enlarge the tooltip / use a snackbar?** The user is asking us to make the button do something useful, not just shout louder. A button that progresses the workflow is self-explanatory and matches the "click three times in the worst case" goal in the task description.
- **Why does `unresolvedCount > 0` block "Next Document"?** Unresolved comments mean the user can't approve regardless of read state. Switching to "Next Document" while the user can't actually approve at the end would be misleading. The unresolved-comments alert (rendered just to the left) already explains the block — a disabled "Approve Design" with that alert is clearer than offering a button that leads nowhere.
- **Why not change the disabled-tooltip text instead?** It's still a disabled button with a hover-only hint. The whole point is people don't notice tooltips on disabled buttons — the fix has to be in the button itself.

## Risks & Notes

- **Race between `interaction_patch` streaming and active-tab guard**: if the user is on Requirements and the agent streams partial updates to Technical Design, Technical Design correctly gets a red dot — that's the desired behaviour. The guard only protects the *active* tab.
- **Cypress / Playwright tests**: no existing E2E coverage for these flows was found. Manual browser testing in the inner Helix is the verification path (per CLAUDE.md guidance).
- **Type safety**: `DocumentType` union is `"requirements" | "technical_design" | "implementation_plan"`. `ALL_TABS` is already typed `DocumentType[]`. No new types needed.
