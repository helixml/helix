# Design: Standardise Review Spec & View PR Button Colours to Match Create Task

## Overview

Purely cosmetic change. Align the MUI `color` prop on the spec-task action
buttons to `secondary` so they match the "Create Task" button's brand teal/cyan.
No new components, no theme changes, no logic changes beyond removing the
now-unnecessary PR-state colour computation.

All edits are in
`frontend/src/components/tasks/SpecTaskActionButtons.tsx`.

## Key Decisions

**Standardise on `color="secondary"`.** This is the existing canonical primary
action colour (`NewSpecTaskForm.tsx:1280`) and is already used by the
multiple-PRs button variant (`SpecTaskActionButtons.tsx:882`, `:917`). It is
theme-aware, so light/dark consistency comes for free from `themes.tsx`.

**Drop PR-state-based button colour.** The single-PR button currently computes
`buttonColor` = `success` (merged) / `inherit` (closed) / `secondary` (open).
Replace this with a constant `secondary`. The `PRStateBadge` rendered alongside
the button already conveys merged/closed/open, so the button colour is redundant
and is the source of the inconsistency the user reported.

## Changes

1. **Review Spec — inline** (`CompactActionButton`, ~line 507):
   `color="info"` → `color="secondary"`.
2. **Review Spec — stacked** (`Button`, ~line 541):
   `color="info"` → `color="secondary"`.
3. **View Pull Request — single PR** (~lines 822–823, 831, 852):
   remove the `buttonColor` ternary; use `color="secondary"` on both the inline
   `CompactActionButton` and the stacked `Button`.
4. **Pulse-glow on Review Spec** (~lines 522–528, 556–563): the glow uses a
   hardcoded blue `rgba(41, 182, 246, …)` matching the old `info` colour.
   Optionally retint to the brand cyan so the glow matches the new button colour.
   Low priority; acceptable to leave as-is if it reads fine in both themes.

The multiple-PRs variant already uses `secondary` — no change needed there.

## Verification

- `cd frontend && yarn build` succeeds.
- In the inner Helix (`http://localhost:8080`): create a spec task and visually
  confirm the "Review Spec" button (spec_review status) and "View Pull Request"
  button (pull_request/done status) match the "Create Task" button colour in
  both light and dark mode. Confirm the PR-state badge still shows correctly for
  merged/closed PRs.

## Notes / Learnings
- Theme palette: `frontend/src/themes.tsx` — `secondary` = `#0e7490` (light) /
  `#00d5ff` (dark); `primary` = `#8989a5`; `info`/`success` use MUI defaults.
- `CompactActionButton` is a thin wrapper that forwards `color` straight to MUI
  `Button`, so the same `color="secondary"` value works for both inline and
  stacked variants.
