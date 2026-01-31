# Command+K Spotlight-Style Search Overlay

**Date:** 2026-01-21
**Status:** Proposed
**Priority:** Medium

## Problem

The Projects page currently has two search/filter elements stacked close together:
1. `UnifiedSearchBar` - global "Search everything... (Cmd/Ctrl+K)"
2. Local filter `TextField` - "Filter projects..."

This creates UX confusion:
- Both look similar (text fields with search icons)
- Users can't quickly distinguish between global search and local filtering
- The global search bar takes up valuable vertical space on every page

## Proposed Solution

Transform the `UnifiedSearchBar` into a **Spotlight-style overlay** triggered by Cmd/Ctrl+K:

### Phase 1: Move Search to Global Sidebar (Quick Win)
- Add `UnifiedSearchBar` to `Sidebar.tsx` right after `SidebarContextHeader` (org/user name)
- Remove `UnifiedSearchBar` from `ProjectsListView` main content area
- Keep only the local "Filter projects..." field inline
- Global search is now consistently available in left nav on all pages + Cmd/Ctrl+K

### Phase 2: Spotlight Overlay Modal
- When user presses Cmd/Ctrl+K, show a centered modal overlay
- Similar to:
  - macOS Spotlight
  - VS Code Command Palette
  - Slack's Quick Switcher
  - Linear's Command+K menu
  - Raycast

**Design characteristics:**
- Centered on screen (not anchored to a text field)
- Semi-transparent backdrop (click to dismiss)
- Rounded corners, elevated shadow
- Search input auto-focused
- Results appear directly below in the same modal
- Keyboard navigation (up/down arrows, Enter to select, Escape to close)
- Type-ahead filtering with categories/tabs

### Phase 3: Move to Global Context
- Register Cmd/Ctrl+K handler at the app root level
- Ensure it works from any page, not just where UnifiedSearchBar is rendered
- Consider adding a small search icon in the top navigation bar as a visual affordance

## Implementation Details

### Files to Modify

1. **Create new component:** `frontend/src/components/common/SpotlightSearch.tsx`
   - Full-screen overlay with backdrop
   - Centered search modal
   - Reuse search logic from `UnifiedSearchBar`

2. **Modify:** `frontend/src/components/project/ProjectsListView.tsx`
   - Remove `UnifiedSearchBar` from lines 88-92
   - Keep only the local filter TextField

3. **Modify:** `frontend/src/pages/Layout.tsx` (or app root)
   - Add global Cmd/Ctrl+K listener
   - Render `SpotlightSearch` component (controlled by open state)

4. **Optional:** `frontend/src/components/system/TopBar.tsx`
   - Add search icon button that also opens the spotlight

### UI Mockup (ASCII)

```
┌─────────────────────────────────────────────────────────────┐
│                    (semi-transparent backdrop)               │
│                                                              │
│         ┌────────────────────────────────────────┐          │
│         │  🔍 Search everything...          ⌘K   │          │
│         ├────────────────────────────────────────┤          │
│         │  All │ Sessions │ Agents │ Code │ ...  │          │
│         ├────────────────────────────────────────┤          │
│         │  📁 Project: My API Backend            │          │
│         │  💬 Session: Debug login flow          │          │
│         │  📝 Task: Implement OAuth              │          │
│         │  📄 Code: auth.ts - line 42            │          │
│         │  🤖 Agent: Code Review Bot             │          │
│         └────────────────────────────────────────┘          │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## Benefits

1. **Cleaner page layouts** - No duplicate search bars
2. **Consistent access** - Cmd/Ctrl+K works from anywhere
3. **Familiar UX pattern** - Users expect this from modern apps
4. **Focus on content** - Search only appears when needed
5. **Mobile-friendly** - Can be triggered via a button on mobile

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Users don't know about Cmd+K | Add keyboard shortcut hint in top bar, onboarding tooltip |
| Accessibility concerns | Ensure proper focus management, ARIA labels, Escape to close |
| Mobile users can't use keyboard | Add search icon button in navigation |

## Success Metrics

- Reduced confusion reports about search vs filter
- Increased usage of global search (track Cmd+K triggers)
- Positive user feedback on cleaner UI

## Timeline

- [ ] Phase 1: Remove inline UnifiedSearchBar (1 hour)
- [ ] Phase 2: Create SpotlightSearch component (4-6 hours)
- [ ] Phase 3: Global context integration (2 hours)
- [ ] Phase 4: Add visual affordance/button (1 hour)

## Related

- Current implementation: `frontend/src/components/common/UnifiedSearchBar.tsx`
- Uses search service: `frontend/src/services/searchService.ts`
