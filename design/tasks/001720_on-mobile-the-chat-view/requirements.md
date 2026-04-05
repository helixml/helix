# Requirements: Default to Chat View on Mobile

## User Story

As a mobile user viewing a spec task, I want the chat view to be the default view instead of "desktop", so that I see the most useful view for my screen size without manually switching tabs.

## Acceptance Criteria

- [ ] On mobile (below `md` / 900px breakpoint), the initial view defaults to `"chat"` instead of `"desktop"` when no `view` URL param is set
- [ ] On desktop (at or above `md` breakpoint), behavior is unchanged — still defaults to `"desktop"`
- [ ] The existing `useIsBigScreen` hook with `breakpoint: "md"` is reused (same breakpoint as the split-view logic)
- [ ] If a `view` URL param is explicitly set, it is respected regardless of screen size
- [ ] The useEffect that switches view on session activation (line ~530) continues to work correctly with this change
