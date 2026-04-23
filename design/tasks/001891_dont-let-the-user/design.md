# Design: Gate Spec Approval on Tab Viewing

## Current State

The codebase already tracks viewed tabs in `DesignReviewContent.tsx:126-128`:

```tsx
const [viewedTabs, setViewedTabs] = useState<Set<DocumentType>>(
  new Set(["requirements"]),  // initial tab pre-viewed
);
```

And updates it on tab change at line 278:

```tsx
setViewedTabs((prev) => new Set(prev).add(newTab));
```

However, `viewedTabs` is **never read** — the approve button is always enabled (unless there are unresolved comments).

## Approach

Wire the existing `viewedTabs` state into `ReviewActionFooter` to disable the approve button.

### Changes

**1. `ReviewActionFooter.tsx`** — Add a new prop `allTabsViewed: boolean`. When `false`, disable the "Approve Design" button and show a tooltip listing unviewed tabs.

**2. `DesignReviewContent.tsx`** — Compute `allTabsViewed` from the existing `viewedTabs` state and pass it plus `unviewedTabNames` to `ReviewActionFooter`.

### Key Decisions

- **Frontend-only** — No backend changes. Tab viewing is ephemeral per review session, not auditable.
- **Only gates Approve** — "Request Changes" and "Reject" remain ungated. You don't need to read everything to know something is wrong.
- **Tooltip, not toast** — A tooltip on the disabled button is less intrusive and more discoverable than a toast/snackbar on click.

### Codebase Patterns Found

- `ReviewActionFooter` already has a pattern for disabling approve: `disabled={unresolvedCount > 0}` (line 96). We extend this condition.
- The component uses MUI `Tooltip` + `<span>` wrapper (see lines 51-68 for the "Start Implementation" button pattern with `isBlockedByDependencies`).
