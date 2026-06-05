# Requirements

## Background

PR https://github.com/helixml/helix/pull/2364 ("Fix active-tab unread bug and add Next Document button to design review") replaced the previously disabled-with-tooltip "Approve Design" button with a primary "Next Document" button when the user has unread tabs. The PR removed the tooltip wrapper entirely from the footer button. As a side-effect, when the "Approve Design" button is *still* shown disabled (because the user has unresolved comments), there is no longer a tooltip explaining why the button is disabled — only the warning Alert rendered to the left of the button row.

The user request explicitly calls out the regression for the **pending-comments** case: a tooltip must be restored to communicate why the user cannot proceed.

## Current behavior (regression)

In `frontend/src/components/spec-tasks/ReviewActionFooter.tsx` the footer renders one of two buttons (when `reviewStatus !== 'approved' && !== 'superseded'`):

| Condition | Button shown | Disabled? | Tooltip? |
|---|---|---|---|
| `hasNextDocument && unresolvedCount === 0` | "Next Document" (primary) | no | n/a |
| otherwise | "Approve Design" (success) | `unresolvedCount > 0 \|\| !allTabsViewed` | **none — regression** |

So when the user has pending comments (with or without unread tabs), they see a disabled "Approve Design" button with no hover hint at all.

## User stories

**US1 — Pending comments must be resolved**
As a reviewer with one or more unresolved comments, I want to hover over the disabled "Approve Design" button and see a tooltip telling me I have unresolved comments, so I understand why the button is disabled and what to do next.

**US2 — Next Document flow stays unchanged**
As a reviewer with unread tabs and no unresolved comments, I should continue to see the enabled "Next Document" primary button (no tooltip needed) — exactly as PR 2364 introduced.

## Acceptance criteria

- [ ] AC1: When `unresolvedCount > 0` and the disabled "Approve Design" button is shown, hovering over it (or focusing it) shows a tooltip whose text references the unresolved comments (count + plural-aware noun).
- [ ] AC2: When `unresolvedCount === 0 && hasNextDocument` (i.e. unread tabs but no pending comments), the primary "Next Document" button is shown enabled with no tooltip — unchanged from PR 2364.
- [ ] AC3: When `unresolvedCount === 0 && allTabsViewed`, the "Approve Design" button is enabled with no tooltip — unchanged from PR 2364.
- [ ] AC4: The fix introduces no other visual or behavioural changes to the footer (no extra props plumbed unnecessarily, no changes to other branches of the `reviewStatus` switch).
- [ ] AC5: TypeScript compiles cleanly (`yarn tsc`) and the production build succeeds (`yarn build`).

## Out of scope

- Any further changes to the spec-task viewer navigation, "Next Document" wrap-around behaviour, or document reading flow.
- Tooltip text for the `!allTabsViewed && unresolvedCount === 0` branch — this branch already shows "Next Document" (enabled), so no tooltip is needed there.
- Restoring the previous `unviewedTabNames` prop. The unread-tab affordance is already covered by the orange dots on tab labels and the "Next Document" button itself.
