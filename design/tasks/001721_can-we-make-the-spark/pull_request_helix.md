# Make spark lines and task list more compact in Kanban cards

## Summary
Reduces vertical space used by the usage pulse chart (spark lines) and task checklist in Kanban card views, making the board denser and easier to scan.

## Changes
- `UsagePulseChart.tsx`: height 50px → 30px, removed outer margins, tightened internal chart margins
- `TaskCard.tsx` `TaskProgressDisplay`: reduced padding on wrapper, progress header, task list container, and per-item rows
- `TaskCard.tsx` `TaskProgressDisplay`: reduced status icon sizes (16/14px → 14/12px)
- `TaskCard.tsx`: reduced label chips bottom margin (8px → 4px)

Estimated ~50px saved per card (~25-30% reduction in the spark+tasklist area).
