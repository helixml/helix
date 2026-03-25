# Implementation Tasks

All changes in: `frontend/src/components/system/GlobalNotifications.tsx`

- [x] Add `timeAgoMs(ms: number)` helper that converts a Unix-ms timestamp to a relative time string using the existing `timeAgo()` function
- [x] Deduplicate `recentPages` by `params.taskId` (keep first/most-recent occurrence per task, after the existing route and alertTaskIds filters)
- [x] Update `RecentPageItem` to show the time-ago label on the right side (matching the layout of notification event items)
- [x] Swap primary and secondary text in `AttentionEventItem`: task name (`spec_task_name || spec_task_id`) becomes the bold primary line, event title becomes secondary
- [x] Allow primary line to wrap up to 2 lines (`WebkitLineClamp: 2`, `whiteSpace: 'normal'`, `-webkit-box` display) instead of single-line ellipsis
- [x] Update grouped event item to also show task name as primary and "Spec ready & agent finished" as secondary
- [x] Replace `eventEmoji()` with `eventIcon()` returning lucide icon components colored by `eventAccentColor()`
- [x] Import `Sparkles`, `Hand`, `AlertCircle`, `GitMerge` from `lucide-react`
- [x] Update `AttentionEventItem` icon render: replace emoji `<Box>` with icon component output, add `display: 'flex'` alignment on the wrapper
- [x] Update grouped event icon to use `<Sparkles>` instead of the `📋` emoji literal
