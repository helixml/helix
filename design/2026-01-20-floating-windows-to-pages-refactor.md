# Floating Windows to Pages Refactor

**Date:** 2026-01-20
**Status:** In Progress

## Summary

Replaced floating window dialogs in the projects/spectask section with proper browser-navigable pages. This enables standard browser navigation (back button, bookmarkable URLs) and simplifies the codebase by removing floating window implementations.

## Changes Completed

### 1. Created New Page Components

#### SpecTaskDetailPage (`src/pages/SpecTaskDetailPage.tsx`)
- Standalone page for viewing spec task details
- Route: `/projects/:id/tasks/:taskId`
- Features:
  - Breadcrumb navigation (Projects > Project Name > Task Name)
  - Back button to return to project tasks
  - "Open in Workspace" button to switch to Workspace view
  - Embeds `SpecTaskDetailContent` component

#### SpecTaskReviewPage (`src/pages/SpecTaskReviewPage.tsx`)
- Standalone page for spec review with commenting
- Route: `/projects/:id/tasks/:taskId/review/:reviewId`
- Features:
  - Breadcrumb navigation (Projects > Project Name > Task Name > Spec Review)
  - Back button to return to task detail
  - "Open in Workspace" button
  - Embeds `DesignReviewContent` component with `hideTitle={true}`

#### DesignReviewContent (`src/components/spec-tasks/DesignReviewContent.tsx`)
- Clean content component extracted from former `DesignReviewViewer.tsx`
- Removed all floating window logic
- Removed serif fonts (Palatino, Georgia) - uses theme defaults
- Added `hideTitle` prop to hide redundant header when embedded in pages with breadcrumbs

### 2. Added Routes (`src/router.tsx`)

```typescript
{
  name: 'project-task-detail',
  path: '/projects/:id/tasks/:taskId',
  meta: { drawer: false, orgRouteAware: true, title: 'Task Details' },
}
{
  name: 'project-task-review',
  path: '/projects/:id/tasks/:taskId/review/:reviewId',
  meta: { drawer: false, orgRouteAware: true, title: 'Spec Review' },
}
```

### 3. Updated Navigation Throughout

- **SpecTaskKanbanBoard.tsx**: Task card clicks navigate to task detail page
- **TabsView.tsx**: "Review Spec" button navigates to review page
- **SpecTaskDetailContent.tsx**: Review button navigates to review page
- **TaskCard.tsx**: Context menu "View Details" navigates to task detail page

### 4. Deleted Floating Window Components

- `DesignReviewViewer.tsx` - Replaced by `DesignReviewContent.tsx`
- `SpecTaskDetailDialog.tsx` - Replaced by `SpecTaskDetailPage.tsx`

### 5. Terminology Changes

- Renamed "Design Review" → "Spec Review" consistently
- Renamed "Tiled View" / "Tabs View" → "Workspace"
- View mode value changed from `'tabs'` to `'workspace'`

### 6. Bug Fixes

- **"Open in Workspace" button**: Fixed query param handling so `tab=workspace`, `openTask`, and `openReview` params are properly read
- **Duplicate headers on review page**: Added `hideTitle` prop to prevent showing both breadcrumb header and component header

## UX Flows (Current State)

### Flow 1: Kanban → Task Detail
1. User clicks task card in Kanban board
2. Navigates to `/projects/:id/tasks/:taskId`
3. Full page with breadcrumbs, back button, desktop viewer

### Flow 2: Task Detail → Spec Review
1. From task detail page, user clicks "Review Spec"
2. Navigates to `/projects/:id/tasks/:taskId/review/:reviewId`
3. Full page with spec documents, inline commenting

### Flow 3: Any Page → Workspace
1. User clicks "Open in Workspace" button
2. Navigates to `/projects/:id/specs?tab=workspace&openTask=...`
3. Workspace view opens with requested task/review

### Flow 4: Workspace (IDE-like view)
1. User clicks Workspace icon in view toggle
2. Multi-panel view with tabs, drag-and-drop, split panels
3. Task detail content embedded in panels

## Roadmap

### Phase 1: Workspace State Persistence (Next)
**Priority: High**

Persist Workspace layout in localStorage so it remembers:
- Which panels are open
- Which tabs are in each panel
- Which tab is active in each panel
- Panel layout direction (horizontal/vertical)

Implementation:
```typescript
// localStorage key
const WORKSPACE_STATE_KEY = 'helix_workspace_state';

// Saved state structure
interface WorkspaceState {
  projectId: string;
  panels: {
    id: string;
    tabs: { taskId: string }[];
    activeTabId: string | null;
  }[];
  layoutDirection: 'horizontal' | 'vertical';
}
```

### Phase 2: In-Workspace Spec Review (Next)
**Priority: High**

When clicking "Review Spec" from within the Workspace, open the review in a new panel instead of navigating away to the standalone page.

Implementation options:
1. Add a new panel type for review content
2. Pass `onOpenReview` callback from TabsView to SpecTaskDetailContent

### Phase 3: Keyboard Shortcuts
**Priority: Medium**

- `Esc` - Close active panel/tab
- `Ctrl+1/2/3` - Switch between tabs
- `Ctrl+\` - Split panel
- `Ctrl+W` - Close tab

### Phase 4: Panel State in URL (Optional)
**Priority: Low**

For sharing specific workspace configurations:
- Encode minimal panel state in URL hash
- Allow bookmarking specific layouts
- Support deep linking to multi-panel views

### Phase 5: Additional UX Improvements
**Priority: Low**

- Empty state improvements when no tasks exist
- Panel history (undo close tab)
- Saved workspace presets

## Files Modified

### New Files
- `src/pages/SpecTaskDetailPage.tsx`
- `src/pages/SpecTaskReviewPage.tsx`
- `src/components/spec-tasks/DesignReviewContent.tsx`

### Modified Files
- `src/router.tsx` - Added new routes
- `src/pages/SpecTasksPage.tsx` - Changed view mode to 'workspace', removed dialog usage
- `src/components/tasks/SpecTaskKanbanBoard.tsx` - Navigate instead of dialog
- `src/components/tasks/TabsView.tsx` - Updated comments
- `src/components/tasks/SpecTaskDetailContent.tsx` - Navigate to review page
- `src/components/tasks/TaskCard.tsx` - Navigate on click

### Deleted Files
- `src/components/tasks/SpecTaskDetailDialog.tsx`
- `src/components/spec-tasks/DesignReviewViewer.tsx`

## Mobile UX Analysis

### Design Goals

The refactor from floating windows to proper pages was partly motivated by mobile support:
- **Native navigation**: Browser back/forward buttons work correctly
- **Bookmarkable URLs**: Users can share links to specific tasks/reviews
- **Touch-friendly**: No floating windows to drag/resize with touch

### Device Profiles

#### Phone (320-480px)
**Primary use case**: Quick status checks, reading spec reviews, simple approvals

**Current issues**:
- Kanban board horizontal scroll awkward on narrow screens
- Desktop viewer in task detail takes too much vertical space
- Breadcrumbs can overflow
- Workspace multi-panel view not practical

**Recommendations**:
1. **Kanban**: Switch to vertical card stack on phone (no columns)
2. **Task Detail**: Stack layout - info at top, desktop viewer collapsed by default
3. **Breadcrumbs**: Collapse to just back arrow + current page title
4. **Workspace**: Hide workspace toggle on phone - use single-task pages only
5. **Touch targets**: Ensure all buttons are at least 44x44px

#### Tablet Portrait (768-1024px) - "Sofa/Bath Mode"
**Primary use case**: Reviewing specs, reading documents, casual monitoring

**Current issues**:
- Kanban works but columns feel cramped
- Desktop viewer aspect ratio awkward in portrait
- Workspace panels too narrow when split

**Recommendations**:
1. **Kanban**: 2-3 visible columns with horizontal scroll
2. **Task Detail**: Full-width desktop viewer, info below
3. **Spec Review**: Document tabs work well, comment sidebar could collapse
4. **Workspace**: Allow but default to single panel; show "split" only if user drags

#### Tablet Landscape + Keyboard (1024-1366px) - "Productivity Mode"
**Primary use case**: Full workflow - creating tasks, reviewing specs, managing multiple tasks

**Current state**: Works well - similar to desktop experience

**Recommendations**:
1. **Workspace**: Full multi-panel support, keyboard shortcuts
2. **Kanban**: All columns visible
3. **Desktop viewer**: Good aspect ratio for remote desktop viewing
4. **Keyboard shortcuts**: Essential for productivity users

### Responsive Breakpoints

```typescript
// Proposed breakpoints
const breakpoints = {
  phone: '(max-width: 599px)',
  tablet: '(min-width: 600px) and (max-width: 1023px)',
  desktop: '(min-width: 1024px)',
}
```

### Component-Specific Mobile Changes

#### SpecTasksPage (View Toggle)
- **Phone**: Hide Workspace option, show only Kanban + Audit
- **Tablet Portrait**: Show all, but Workspace opens in "single panel mode"
- **Tablet Landscape+**: Full experience

#### SpecTaskKanbanBoard
- **Phone**: Vertical stack view (one column at a time, swipe to change)
- **Tablet**: Horizontal scroll with 2-3 visible columns
- **Desktop**: All columns visible

#### SpecTaskDetailPage
- **Phone**:
  - Collapsed breadcrumbs (back arrow + "Task Name")
  - Desktop viewer collapsed by default with "Expand" button
  - Action buttons sticky at bottom
- **Tablet+**: Current layout works

#### SpecTaskReviewPage
- **Phone**:
  - Document tabs as horizontal scroll pills
  - Comment sidebar as bottom sheet (swipe up)
  - Inline comment markers simplified
- **Tablet Portrait**: Comment sidebar collapsible
- **Tablet Landscape+**: Full sidebar

#### Workspace (TabsView)
- **Phone**: Disabled - redirect to single-task pages
- **Tablet Portrait**: Enabled with touch optimizations; default single panel but allow split
- **Tablet Landscape+**: Full multi-panel support with keyboard shortcuts

**Tablet Workspace use cases:**
- Side-by-side: Desktop stream + chat (see agent work while chatting)
- Side-by-side: Task detail + spec review (review while seeing implementation)
- Split: Two different tasks for comparison

**Touch optimizations needed:**
- Larger drag handles for panel resize
- Swipe to close panel (instead of small X button)
- Long-press on tab for context menu (close, move to new panel)
- Pinch-to-resize panels (optional enhancement)

### Touch Interactions

| Desktop Action | Mobile Equivalent |
|----------------|-------------------|
| Hover for tooltip | Long-press for tooltip |
| Drag tab to split | Swipe actions or dedicated split button |
| Right-click context menu | Long-press context menu |
| Resize panels | Pinch or drag handle |

### Implementation Priority

1. **Phase A**: Basic responsive layouts (breakpoint detection, stacking)
2. **Phase B**: Mobile-specific navigation (bottom sheet comments, collapsed breadcrumbs)
3. **Phase C**: Touch optimizations (gesture handlers, larger targets)
4. **Phase D**: Mobile-specific features (swipe actions on cards)

## Team Desktop Refactor (Planned)

### Naming

- **UI name**: "Team Desktop" (user-facing, consistent)
- **Code name**: "exploratory session" (legacy, in API/code)
- **Action**: Rename all UI strings from "exploratory session" to "Team Desktop"

### Current State

The Team Desktop (internally called "exploratory session") currently uses a floating modal:

```
SpecTasksPage
  └── FloatingModal (context-based)
        └── ExternalAgentDesktopViewer
              ├── DesktopStreamViewer (video stream)
              ├── ScreenshotViewer (fallback)
              ├── SandboxDropZone (file upload)
              └── EmbeddedSessionView (chat panel)
```

**Files involved:**
- `src/contexts/floatingModal.tsx` - Context for showing floating modals
- `src/components/admin/FloatingModal.tsx` - Floating window container with drag/resize
- `src/components/external-agent/ExternalAgentDesktopViewer.tsx` - Desktop viewer component
- `src/pages/SpecTasksPage.tsx` - Launches exploratory sessions via floating modal

### Problems with Current Approach

1. **No URL/bookmarking** - Can't share link to a team desktop session
2. **Mobile unfriendly** - Floating windows don't work on touch devices
3. **Not embeddable in Workspace** - Can't have team desktop alongside task details
4. **Complex state management** - FloatingModal context adds unnecessary abstraction

### Proposed Changes

#### 1. Create TeamDesktopPage

**New file: `src/pages/TeamDesktopPage.tsx`**
- Route: `/projects/:id/desktop` or `/projects/:id/desktop/:sessionId`
- Embeds `ExternalAgentDesktopViewer` as page content
- Breadcrumb navigation (Projects > Project Name > Team Desktop)
- "Open in Workspace" button

```typescript
// Route
{
  name: 'project-team-desktop',
  path: '/projects/:id/desktop/:sessionId?',
  meta: { drawer: false, orgRouteAware: true, title: 'Team Desktop' },
  render: () => <TeamDesktopPage />,
}
```

#### 2. Create TeamDesktopContent Component

**New file: `src/components/team-desktop/TeamDesktopContent.tsx`**
- Clean content component extracted from ExternalAgentDesktopViewer
- Can be embedded in both TeamDesktopPage and Workspace panels
- Props: `sessionId`, `projectId`, `onClose?`

#### 2b. Unify Chat Widget Layout

Both SpecTaskDetailContent and ExternalAgentDesktopViewer use `EmbeddedSessionView` for chat, but with different layouts:
- **SpecTaskDetailContent**: Chat below desktop viewer (vertical stack)
- **ExternalAgentDesktopViewer**: Chat on right side (horizontal split)

**Consolidation:**
- Use the same layout pattern as SpecTaskDetailContent (chat below or collapsible)
- In Workspace, users can split panels to get side-by-side desktop + chat if desired
- Remove the hardcoded right-side chat panel from ExternalAgentDesktopViewer

#### 3. Update Navigation

**SpecTasksPage changes:**
- "Start Exploratory Session" → navigates to `/projects/:id/desktop/:sessionId`
- "Resume Session" → navigates to desktop page
- Remove `floatingModal.showFloatingModal` calls for exploratory sessions

#### 4. Add to Workspace

**TabsView changes:**
- New panel type: `'desktop'` alongside `'task'`
- Panel can contain either task detail or team desktop
- Interface update:

```typescript
interface PanelTab {
  id: string
  type: 'task' | 'desktop' | 'review'
  taskId?: string      // for task/review
  sessionId?: string   // for desktop
  reviewId?: string    // for review
}
```

**Killer use case:** Side-by-side desktop + chat on tablet - see what the agent is doing while chatting with it.

### FloatingModal Future

After team desktop refactor, FloatingModal will only be used for:
- `logs` - Log viewer (admin only)
- `rdp` - Quick screenshot/RDP viewer (admin only)

Could potentially convert logs viewer to a page as well (`/admin/logs/:runnerId`).

### Implementation Order

1. Create `TeamDesktopContent.tsx` - extract from ExternalAgentDesktopViewer
2. Create `TeamDesktopPage.tsx` - page wrapper
3. Add route to router.tsx
4. Update SpecTasksPage to navigate instead of floating modal
5. Add desktop panel type to TabsView (Workspace)
6. Test all flows
7. Clean up unused FloatingModal code for exploratory_session type

## Roadmap

### Phase 1: Workspace State Persistence
**Priority: High**

Persist Workspace layout in localStorage so it remembers:
- Which panels are open
- Which tabs are in each panel
- Which tab is active in each panel
- Panel layout direction (horizontal/vertical)

Implementation:
```typescript
const WORKSPACE_STATE_KEY = 'helix_workspace_state';

interface WorkspaceState {
  projectId: string;
  panels: {
    id: string;
    tabs: { taskId: string }[];
    activeTabId: string | null;
  }[];
  layoutDirection: 'horizontal' | 'vertical';
}
```

### Phase 2: In-Workspace Spec Review
**Priority: High**

When clicking "Review Spec" from within the Workspace, open the review in a new panel instead of navigating away.

### Phase 3: Workspace Info/Onboarding
**Priority: Medium**

Add an info button (?) on the Workspace view that explains:
- What the Workspace is for (multi-task view, IDE-like experience)
- How to use it (drag tabs, split panels, keyboard shortcuts)
- When to use it vs single-task pages

Could be a dismissable tooltip or a "?" button that opens a brief modal.

### Phase 4: Mobile Responsive - Basic
**Priority: Medium**

- Add `useMediaQuery` hooks for breakpoint detection
- Hide Workspace toggle on phone
- Stack layouts for task detail and review pages
- Collapse breadcrumbs on small screens

### Phase 5: Mobile Responsive - Enhanced
**Priority: Medium**

- Kanban vertical stack view for phone
- Bottom sheet for comments on spec review
- Touch-optimized context menus
- Larger touch targets (44x44px minimum)

### Phase 6: Keyboard Shortcuts
**Priority: Medium**

- `Esc` - Close active panel/tab
- `Ctrl+1/2/3` - Switch between tabs
- `Ctrl+\` - Split panel
- `Ctrl+W` - Close tab

### Phase 7: Additional UX Improvements
**Priority: Low**

- Panel history (undo close tab)
- Saved workspace presets
- Swipe gestures on kanban cards (phone)

## Testing Checklist

### Desktop
- [ ] Kanban card click navigates to task detail page
- [ ] Back button returns to kanban
- [ ] "Review Spec" navigates to review page
- [ ] "Open in Workspace" button switches to workspace view
- [ ] Workspace view toggle works
- [ ] Breadcrumbs navigate correctly
- [ ] No duplicate headers on review page

### Tablet Landscape
- [ ] All desktop features work
- [ ] Workspace multi-panel works
- [ ] Touch targets are adequate

### Tablet Portrait
- [ ] Kanban scrolls horizontally
- [ ] Task detail readable
- [ ] Spec review comment sidebar usable

### Phone
- [ ] Navigation works (back button, breadcrumbs)
- [ ] Task detail page readable
- [ ] Spec review document tabs work
- [ ] Workspace toggle hidden (if implemented)

### Build
- [ ] `cd frontend && yarn test && yarn build` passes
