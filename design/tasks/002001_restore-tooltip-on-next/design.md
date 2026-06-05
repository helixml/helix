# Design

## Summary

Wrap the disabled "Approve Design" button in `ReviewActionFooter.tsx` in a `<Tooltip>` whose `title` is non-empty only when `unresolvedCount > 0`. No other component changes, no new props.

## Affected file

- `frontend/src/components/spec-tasks/ReviewActionFooter.tsx` — single file change.

## Current code (post-PR-2364)

Lines ~98–115 of `ReviewActionFooter.tsx`:

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
  <Button
    variant="contained"
    color="success"
    onClick={onApprove}
    disabled={unresolvedCount > 0 || !allTabsViewed}
  >
    Approve Design
  </Button>
)}
```

`Tooltip` is already imported from `@mui/material` (used elsewhere in the file for the "Start Implementation" / dependency-blocked button), so no import change is needed.

## Proposed change

Wrap only the "Approve Design" branch in a `<Tooltip>`. Use the standard MUI pattern of wrapping a disabled button in `<span>` so the tooltip can still capture hover events on a disabled element.

```tsx
) : (
  <Tooltip
    title={
      unresolvedCount > 0
        ? `Resolve ${unresolvedCount} comment${unresolvedCount !== 1 ? 's' : ''} before approving`
        : ''
    }
    placement="top"
  >
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

MUI's `<Tooltip>` automatically suppresses rendering when `title` is empty, so the no-tooltip behaviour for the "all good" enabled state and for the `!allTabsViewed && unresolvedCount === 0` (which falls into the Next Document branch anyway) are preserved.

## Key decisions

### Why a tooltip only for `unresolvedCount > 0`, not `!allTabsViewed`?

The two disabled-button states are:

1. `unresolvedCount > 0` — there is no other discoverable affordance pointing the user at *which* button is broken; the Alert sits to the left, far from the button. **Tooltip needed.**
2. `!allTabsViewed && unresolvedCount === 0` — this branch never reaches "Approve Design": the parent component sets `hasNextDocument = !allTabsViewed`, so this falls into the "Next Document" branch and a tooltip is unnecessary. The user is being actively guided forward by the primary button.

So the only reachable disabled state on "Approve Design" is `unresolvedCount > 0` (optionally combined with `!allTabsViewed`). Tooltip text is keyed off `unresolvedCount`.

### Why match the wording style of the Alert (`"X unresolved comment(s)"`)?

The footer already shows an `Alert` with `"{unresolvedCount} unresolved comment{unresolvedCount !== 1 ? 's' : ''}"` (line ~80). The tooltip uses an action-oriented phrasing (`"Resolve X comment(s) before approving"`) that complements the alert: the alert states the fact, the tooltip states what the user must do to unblock the button. This avoids dead-end repetition while keeping count + plural correctness.

### Why no new props?

`unresolvedCount` is already a prop. No prop plumbing needed in `DesignReviewContent.tsx`. This keeps the blast radius to one file and matches the principle of minimal change for a regression fix.

### Why wrap the whole branch in `<Tooltip>` rather than a conditional `<Tooltip>`?

MUI handles empty `title` by not rendering the tooltip at all. A single, always-rendered `<Tooltip>` keeps the JSX simpler than splitting into two conditional branches and matches the existing pattern in this same file (the `Tooltip` around "Start Implementation" with `isBlockedByDependencies ? blockedReason : ''`).

## Test plan

Manual verification in the inner Helix browser, mirroring the four scenarios from PR 2364's test plan plus the regression case:

1. **No comments, all tabs viewed** → "Approve Design" enabled, no tooltip on hover. ✅ unchanged.
2. **No comments, unread tabs** → "Next Document" enabled, no tooltip. ✅ unchanged.
3. **Pending comments, all tabs viewed** → "Approve Design" disabled, hover shows tooltip "Resolve 1 comment before approving" (or `"Resolve N comments…"`). 🆕 fixed.
4. **Pending comments, unread tabs** → "Approve Design" disabled (Next Document branch suppressed by `unresolvedCount === 0` clause), hover shows the same tooltip. 🆕 fixed.
5. **Plural correctness** — insert 2 unresolved comments, confirm tooltip says "comments" not "comment".

`cd frontend && yarn tsc && yarn build` must pass.

## Notes for the implementer

- `<Tooltip>` is already imported on line 2 — do not re-add it.
- Existing wrapped-disabled-button precedent at lines 57–74 (the "Start Implementation" tooltip) uses the exact same `<Tooltip>...<span><Button disabled>...</Button></span></Tooltip>` pattern. Mirror it.
- The unresolved-comment Alert (lines 79–83) stays as-is. The two affordances complement each other.
- `eslint` / `prettier` are run on commit; trailing comma + 2-space indent are project conventions.
