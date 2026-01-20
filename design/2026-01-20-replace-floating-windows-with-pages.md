# Plan: Replace Floating Windows with Proper Pages

## Summary

Replace floating window dialogs in the projects/spectask section with proper browser-navigable pages. This enables standard browser navigation (back button, bookmarkable URLs) and simplifies the codebase by removing floating window implementations. Users who want multi-window view can use the tiled mode.

## Current State

### Floating Window Components
- `SpecTaskDetailDialog.tsx` - Floating dialog for spec task details (~1400 lines)
- `DesignReviewViewer.tsx` - Floating window for design review (~1300 lines)
- `Window.tsx` - Base dialog wrapper component
- `FloatingModal` context - For exploratory session, RDP, logs

### Current Navigation
- `/projects/:id/specs` - SpecTasksPage with kanban/tabs view
- `/design-doc/:specTaskId/:reviewId` - DesignDocPage (already a proper page, but read-only)
- No dedicated routes for spec task detail or review pages

### Fonts Issue
- `DesignReviewViewer.tsx` uses serif fonts (Palatino Linotype, Georgia) - lines 1069, 1237, 1243, 1255, 1264
- Should be removed to use default theme fonts

## Implementation Plan

### Phase 1: Create New Pages

#### 1.1 Create SpecTaskDetailPage
**New file: `src/pages/SpecTaskDetailPage.tsx`**
- Refactor content from `SpecTaskDetailContent.tsx` into a proper page
- Route: `/projects/:id/tasks/:taskId`
- Features:
  - Task details, description, status
  - Agent selection and controls
  - Desktop viewer (embedded, not floating)
  - Design doc tabs
  - Review button that navigates to review page
- Add "Open in Tiled View" button in header

#### 1.2 Create SpecTaskReviewPage
**New file: `src/pages/SpecTaskReviewPage.tsx`**
- Refactor `DesignReviewViewer.tsx` content into a proper page
- Route: `/projects/:id/tasks/:taskId/review/:reviewId`
- Features:
  - Document tabs (requirements, technical design, implementation plan)
  - Inline commenting
  - Review submission
  - Comment sidebar
- Remove serif fonts - use theme defaults
- Add "Open in Tiled View" button in header

### Phase 2: Add Routes

**Edit: `src/router.tsx`**

Add new routes in `getOrgRoutes()`:
```typescript
{
  name: namePrefix + 'project-task-detail',
  path: routePrefix + '/projects/:id/tasks/:taskId',
  meta: { drawer: false, orgRouteAware: true, title: 'Task Details' },
  render: () => <SpecTaskDetailPage />,
},
{
  name: namePrefix + 'project-task-review',
  path: routePrefix + '/projects/:id/tasks/:taskId/review/:reviewId',
  meta: { drawer: false, orgRouteAware: true, title: 'Design Review' },
  render: () => <SpecTaskReviewPage />,
}
```

### Phase 3: Update Navigation

#### 3.1 Update SpecTaskKanbanBoard.tsx
- Change task card clicks to navigate to `/projects/:id/tasks/:taskId` instead of opening `SpecTaskDetailDialog`
- Remove `SpecTaskDetailDialog` usage
- Remove `designReviewViewerOpen` state

#### 3.2 Update TabsView.tsx
- Change review button to navigate to review page
- Remove `DesignReviewViewer` usage

#### 3.3 Update TaskCard.tsx
- Navigate to task detail page on click

### Phase 4: Add "Open in Tiled View" Feature

#### 4.1 Create TiledViewButton component
**New file: `src/components/common/TiledViewButton.tsx`**
- Button that appears in page headers
- On click: switches to tiled mode and adds current view

#### 4.2 Extend tiled view context
- Add ability to open current page in tiled workspace
- Store page URL/route for restoration

### Phase 5: Remove Deprecated Code

After confirming pages work:
- Delete or simplify `SpecTaskDetailDialog.tsx`
- Remove floating window logic from `DesignReviewViewer.tsx` (keep as embedded component)
- Remove unused Window tiling/dragging code

### Phase 6: Remove Serif Fonts

**Edit: `src/components/spec-tasks/DesignReviewViewer.tsx`**

Remove these fontFamily declarations:
- Line 1069: `fontFamily: "'Palatino Linotype', Georgia, serif"`
- Line 1237: `fontFamily: "'Palatino Linotype', 'Book Antiqua', Palatino, Georgia, serif"`
- Line 1243, 1255, 1264: Similar Palatino declarations

Keep `fontFamily: 'Monaco, Consolas, monospace'` for code blocks.

## Files to Modify

### New Files
- `src/pages/SpecTaskDetailPage.tsx`
- `src/pages/SpecTaskReviewPage.tsx`
- `src/components/common/TiledViewButton.tsx`

### Modified Files
- `src/router.tsx` - Add new routes
- `src/components/tasks/SpecTaskKanbanBoard.tsx` - Navigate instead of dialog
- `src/components/tasks/TabsView.tsx` - Navigate instead of dialog
- `src/components/tasks/TaskCard.tsx` - Navigate on click
- `src/components/spec-tasks/DesignReviewViewer.tsx` - Remove fonts, simplify to embedded component
- `src/pages/SpecTasksPage.tsx` - Remove dialog usage

### Files to Potentially Delete Later
- `src/components/tasks/SpecTaskDetailDialog.tsx` (after migration)

## Verification

1. **Navigation Test**:
   - Click task in kanban board → navigates to `/projects/:id/tasks/:taskId`
   - Browser back button returns to kanban
   - Click review button → navigates to review page
   - Back button returns to task detail

2. **Tiled View Test**:
   - Click "Open in Tiled View" → adds to workspace
   - Multiple views can be tiled side-by-side

3. **Font Test**:
   - Review page uses default theme fonts (no Palatino)
   - Code blocks still use monospace

4. **Build Test**:
   ```bash
   cd frontend && yarn test && yarn build
   ```

## User Decisions

1. **Workspace Button**: Only on task detail, review, and Team Desktop pages (not all pages)
2. **Design Doc Route**: Keep both routes - `/design-doc` for public sharing, new review route for authenticated review with commenting

---

## Changes Completed (2026-01-20)

### 1. Renamed Tiled View to Workspace
- "Tiled View"/"Tabs View" → "Workspace" throughout UI
- Query param: `?tab=workspace`
- localStorage key: `helix_spectask_view_mode`

### 2. Workspace State Persistence
- Layout saved to localStorage (`helix_workspace_state`)
- Remembers open panels, tabs, active tab, and layout direction
- Restored when returning to Workspace view

### 3. Created Team Desktop Page
- New route: `/projects/:id/desktop/:sessionId`
- Proper breadcrumbs: Projects → {Project Name} → Team Desktop
- "Open in Workspace" button to embed in Workspace

### 4. Extended Workspace Tab Types
- `TabData.type`: `'task' | 'review' | 'desktop'`
- Desktop tabs show ExternalAgentDesktopViewer with full streaming
- Reviews open in Workspace panes (not navigate away)

### 5. Mobile/Touch UX Improvements (DesktopStreamViewer)
- iOS custom CSS fullscreen (native fullscreen blocks interaction)
- Picture-in-Picture support detection
- Larger toolbar icons for touch devices (medium vs small)
- Better tap detection thresholds for mobile
- Reduced cursor sensitivity for precision (0.8 vs 1.5)

---

## Remaining Roadmap

### Phase 7: Chat Widget Consolidation
The Team Desktop currently uses `EmbeddedSessionView` in a side panel.
SpecTaskDetailContent uses a different chat widget implementation.
Consolidate to use the same chat component for consistency.

### Phase 8: Info Button for Workspace
Add an info button on the Workspace page that explains:
- What the Workspace view is for
- How to use split panes
- How to add/remove panels

### Phase 9: Mobile UX Polish
- Hide Workspace option on phones (< 768px)
- Optimize for tablet portrait/landscape
- Ensure touch interactions work in Workspace view

### Phase 10: Cleanup Deprecated Code
After migration is stable:
- Simplify or remove `SpecTaskDetailDialog.tsx`
- Remove unused floating window code from `DesignReviewViewer.tsx`
- Remove `FloatingModal` usage in SpecTasksPage
