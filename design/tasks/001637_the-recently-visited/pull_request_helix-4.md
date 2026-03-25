# Polish notifications panel: dedup, time-ago, icon & layout improvements

## Summary

Cleans up the notifications panel in four ways:

- **Recently visited deduplication**: if you visited both the task-detail and review page for the same task, only the most-recent entry now shows (dedup by `taskId` at render time, matching the existing alert-filter logic)
- **Recently visited time-ago**: each entry now shows a dim relative timestamp on the right (e.g. "3m ago"), using the existing `timeAgo()` helper
- **Notification items layout**: task name (the prompt) is now the bold primary line and wraps up to 2 lines; the event type label ("Spec ready", "Agent finished") moves to the secondary dim line
- **Icons**: all emoji icons replaced with lucide-react SVG icons — `Sparkles` for spec-ready, `Hand` for agent-finished, `AlertCircle` for failures, `GitMerge` for PR-ready — colored using the existing accent-color system

## Changes

- `frontend/src/components/system/GlobalNotifications.tsx` — single file changed
  - Added `timeAgoMs()` helper
  - Added `eventIcon()` replacing `eventEmoji()`
  - Imported `Sparkles`, `Hand`, `AlertCircle`, `GitMerge` from `lucide-react`
  - Updated `RecentPageItem` to show time label and use `Bell` icon instead of clock emoji
  - Updated `recentPages` filter to deduplicate by `params.taskId`
  - Updated `AttentionEventItem` to show task name as primary (2-line wrap) and event title as secondary
