# Requirements

## Background

The design review screen (`DesignReviewContent.tsx` + `ReviewActionFooter.tsx`) shows a spec across three tabs: **Requirements**, **Technical Design**, **Implementation Plan**. A red dot appears on tabs whose content has changed since the user last viewed them, and the **Approve Design** button is disabled until every tab has been viewed at least once with its current content.

Two issues need fixing:

1. **Bug** — When the agent pushes a content change to the tab the user is *currently* viewing, that tab gets a red dot and the user is forced to click away and back to clear it. The user is already looking at the new content; marking it unread is wrong.
2. **UX** — Users miss the tooltip on the disabled "Approve Design" button explaining why it's disabled. They get stuck not realising they need to view the other tabs.

## User Stories

### Story 1: Don't mark the active tab as unread

> As a reviewer reading a tab, when the agent updates that same tab's content while I'm looking at it, I should see the updated content without the tab being flagged as unread — because I'm clearly already viewing it.

**Acceptance criteria**

- When the active tab's content changes (via WebSocket / poll refresh), the active tab MUST NOT be added to the unread set.
- The active tab's stored content snapshot is updated to the new content, so navigating away and back doesn't show the dot either.
- The other (non-active) tabs continue to be marked unread when their content changes — that behaviour is unchanged.
- Switching to a tab that was previously marked unread continues to clear its red dot (existing behaviour preserved).

### Story 2: Replace "Approve Design" with "Next Document" when tabs are unread

> As a reviewer with one or more unread tabs, instead of seeing a disabled "Approve Design" button with a tooltip I might miss, I should see a "Next Document" button that takes me to the next unread tab.

**Acceptance criteria**

- When `allTabsViewed` is `false` AND `unresolvedCount === 0` AND review status is reviewable (not `approved`/`superseded`):
  - The button reads **"Next Document"**.
  - The button is **enabled**.
  - Clicking it switches the active tab to the next unread tab in tab order (Requirements → Technical Design → Implementation Plan, wrapping if needed).
- When `allTabsViewed` is `true`:
  - The button reads **"Approve Design"** and behaves exactly as today.
- When `unresolvedCount > 0`:
  - The "unresolved comments" path takes priority — the button stays as **"Approve Design"** and disabled, with the existing unresolved-comments alert. (Rationale: the user must address comments first regardless of tab read state; flipping to "Next Document" when they can't approve anyway would be misleading.)
- The tooltip on the disabled "Approve Design" button is preserved for the `unresolvedCount > 0` case.
- After a user clicks "Next Document" enough times to view every tab, the button MUST update to "Approve Design" without a page reload.

## Out of Scope

- Backend changes — both fixes are pure frontend.
- Changing what counts as "viewed" (still: clicking the tab).
- Adjusting the unresolved-comments gating logic.
- Reworking the tooltip styling/placement for the remaining disabled cases.
