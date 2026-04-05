# Requirements: Compact Spark Lines & Task List in Kanban Cards

## User Story

As a user viewing the Kanban board, I want the spark lines (usage pulse chart) and the task checklist to take up less vertical space on each card, so that more cards are visible without scrolling and the board feels less cluttered.

## Acceptance Criteria

- [ ] The UsagePulseChart height is reduced from 50px to ~30px while remaining readable
- [ ] The TaskProgressDisplay has tighter vertical padding between items
- [ ] The overall card vertical footprint is noticeably shorter (target: ~20-30% reduction in the spark+tasklist area)
- [ ] Spark lines remain visually clear — the area gradient and line are still distinguishable
- [ ] Task checklist items remain legible — text is not clipped or unreadable
- [ ] Progress bar header stays functional (progress percentage still visible)
- [ ] No layout breakage on different column widths or mobile view
