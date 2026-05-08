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

The review data is polled via React Query (`refetchInterval: 3000-5000ms`), so tab content can change while the user is reviewing (e.g. after a "Request Changes" round where the agent revises the spec).

## Approach

Wire the existing `viewedTabs` state into `ReviewActionFooter` to disable the approve button. Additionally, track content hashes so that when content changes, the affected tab is marked as unviewed again.

### Changes

**1. `ReviewActionFooter.tsx`** — Add a new prop `allTabsViewed: boolean`. When `false`, disable the "Approve Design" button and show a tooltip listing unviewed tabs.

**2. `DesignReviewContent.tsx`** — Compute `allTabsViewed` and `unviewedTabNames` from the existing `viewedTabs` state and pass them to `ReviewActionFooter`. Additionally:
- Track the content of each tab at the time it was viewed (using a `useRef<Map<DocumentType, string>>` to store content snapshots).
- In a `useEffect` watching the review data, compare current content against the snapshot for each viewed tab. If content has changed, remove that tab from `viewedTabs` — the user must re-view it.
- Add a visual indicator (small colored dot) on tab labels for tabs that have unread changes (i.e. were previously viewed but got invalidated).

### Key Decisions

- **Frontend-only** — No backend changes. Tab viewing is ephemeral per review session, not auditable.
- **Only gates Approve** — "Request Changes" and "Reject" remain ungated. You don't need to read everything to know something is wrong.
- **Tooltip, not toast** — A tooltip on the disabled button is less intrusive and more discoverable than a toast/snackbar on click.
- **Content-change detection via string comparison** — Compare the current tab content string against the snapshot taken when the user viewed it. Simple and reliable since the content is already in memory from the React Query poll. No hashing needed — direct string equality is fine for three short markdown documents.

**3. `InlineCommentBubble.tsx`** — Replace `CloseIcon` with `CheckCircleIcon` (green) on the resolve button (line 130). The X icon is ambiguous — it looks like "dismiss/close" rather than "resolve". A green tick makes the action clear.

**4. `CommentLogSidebar.tsx`** — Same change: replace `CloseIcon` with green `CheckCircleIcon` on the resolve button (line 78). Note: `CheckCircleIcon` is already imported here (line 4) but only used for the "Resolved" chip, not the button.

### Codebase Patterns Found

- `ReviewActionFooter` already has a pattern for disabling approve: `disabled={unresolvedCount > 0}` (line 96). We extend this condition.
- The component uses MUI `Tooltip` + `<span>` wrapper (see lines 51-68 for the "Start Implementation" button pattern with `isBlockedByDependencies`).

## Implementation Notes

- Refactored the three hardcoded `<Tab>` elements into a `.map()` over `ALL_TABS` — eliminates duplication and makes the unviewed dot indicator trivial to add.
- `getTabContent()` is a plain function (not memoized) since it's only called on tab change and in the effect — no performance concern.
- The content-change `useEffect` depends on `review?.requirements_spec, review?.technical_design, review?.implementation_plan` — these are primitive strings, so the effect only fires when actual content changes (not on every poll cycle).
- `CloseIcon` import removed from `CommentLogSidebar.tsx` since it was the only usage. `InlineCommentBubble.tsx` swapped `CloseIcon` for `CheckCircleIcon`.
- WARNING: Could not test in browser — inner Helix has no spec tasks or users. TypeScript type-check (`tsc --noEmit`) passes cleanly. `vite build` fails on an unrelated `dist/` directory permission issue (bind mount).
