# Restore tooltip on disabled "Approve Design" when comments are pending

## Summary

Fixes a regression introduced by https://github.com/helixml/helix/pull/2364: when the user has unresolved comments, the "Approve Design" button is rendered disabled but the tooltip explaining *why* was removed along with the now-dead `unviewedTabNames` tooltip. This restores a hover/focus tooltip on that disabled button so the user knows to resolve their comments before approving.

## Changes

- `frontend/src/components/spec-tasks/ReviewActionFooter.tsx` — wrap the "Approve Design" `<Button>` in a `<Tooltip>` whose `title` is `"Resolve N comment(s) before approving"` when `unresolvedCount > 0` and an empty string otherwise (MUI suppresses rendering for empty titles). Uses the standard `<Tooltip><span><Button disabled>...</Button></span></Tooltip>` pattern already used elsewhere in this file (the "Start Implementation" tooltip) to make the disabled button's hover events bubble to the tooltip wrapper.

## Notes

- No new props, no other props removed. Single-file change, ~13 lines.
- The "Next Document" branch (`hasNextDocument && unresolvedCount === 0`) is untouched — that primary button is enabled and self-explanatory, no tooltip needed.
- The unresolved-comments warning `<Alert>` rendered to the left of the button row is kept as-is; the new tooltip complements it (the alert states the fact, the tooltip states the action required to unblock).
- Plural-aware text: "Resolve 1 comment…" vs "Resolve 2 comments…".

## Verification

Verified end-to-end against the running Vite dev server in the inner Helix by dynamically importing the updated `ReviewActionFooter.tsx` and mounting it with each prop combination. Spec-task creation in the inner Helix requires AI-provider API keys that aren't available in this environment, so direct mounting of the production component was used as a substitute — the component code under test is identical to what ships.

| Scenario | `unresolvedCount` | `allTabsViewed` | `hasNextDocument` | Expected | Result |
|---|---|---|---|---|---|
| 1 comment, all read | 1 | true | false | Disabled "Approve Design" + "Resolve 1 comment before approving" | ✅ |
| 2 comments, all read | 2 | true | false | Disabled "Approve Design" + "Resolve 2 comments before approving" | ✅ |
| No comments, all read | 0 | true | false | Enabled "Approve Design", no tooltip | ✅ |
| No comments, unread tabs | 0 | false | true | "Next Document" (unchanged from PR 2364) | ✅ |
| Comments + unread tabs | 3 | false | true | Disabled "Approve Design" + "Resolve 3 comments before approving" | ✅ |

`yarn tsc` and `yarn build` both pass.

## Screenshots

![Singular comment tooltip](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002001_restore-tooltip-on-next/screenshots/01-tooltip-pending-comments-singular.png)
![Plural comments tooltip](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002001_restore-tooltip-on-next/screenshots/02-tooltip-pending-comments-plural.png)
![Next Document button still works (no comments, unread tabs)](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002001_restore-tooltip-on-next/screenshots/03-next-document-no-comments.png)
![Comments + unread tabs: disabled Approve with tooltip (regression case)](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002001_restore-tooltip-on-next/screenshots/04-tooltip-pending-and-unread.png)
