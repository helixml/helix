# Implementation Tasks

- [ ] In `TaskCard.tsx`, add a priority label to the status row after the phase name, using a `•` separator matching the existing `runningDuration` separator pattern
- [ ] Style `critical` as `#ef4444`, `high` as `#f59e0b`, and `low`/`medium` as `text.secondary` at low opacity — wrap in a `<Tooltip>` showing "Priority: {value}"
- [ ] Hide the label when `priority` is `"medium"` (the silent default) to avoid cluttering every card
- [ ] Run `cd frontend && yarn build` to verify no TypeScript errors
