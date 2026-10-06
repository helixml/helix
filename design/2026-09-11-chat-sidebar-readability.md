# Chat sidebar readability

## Problem

The chat sidebar used the same system font and 14px task-title size as T3 Code,
but inactive rows were materially harder to scan. Sidebar components bypassed
the central typography configuration with fixed 10px, 11px, and 14px values,
used regular weight for primary labels, and compounded muted colors with low
opacity. Cross-project rows also compressed project and task identity into two
lines while T3 uses a stable three-line card.

## Target hierarchy

- Primary task, project, bot, and person labels: 14px/20px, medium weight.
- Project metadata and time: 12px/16px, medium weight where it identifies the row.
- Section labels and compact status text: 11px.
- Stacked cross-project rows: 78px with project, task title, and branch lines.
- Branch metadata truncates on the left while the harness mark stays aligned to
  the right edge of the row.
- Pre-implementation tasks without an assigned branch show `n/a`; its tooltip
  explains that the task is still in planning mode.
- Primary, secondary, and surface colors come from shared sidebar theme tokens.
- Sidebar font sizes come from `styles/typography.ts` and remain rem-based.

## Verification

- Focused sidebar component tests.
- Production frontend build.
- Live dark- and light-mode review in the inner Helix at
  `http://localhost:8080`, including computed typography and row geometry.
