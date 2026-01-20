# Helix Projects UX Refactor

**Date:** 2026-01-20
**Status:** In Progress

## Executive Summary

This document captures the comprehensive UX refactor of Helix Projects to:
1. Replace floating windows with proper browser-navigable pages
2. Make Projects accessible to non-technical users
3. Improve the Workspace (multi-panel) experience
4. Add Team Desktop as a first-class feature

---

## Part 1: Completed Work

### 1.1 Navigation Refactor

**Projects is now the landing page:**
- Route `/` → Projects page (was Chat/Home)
- Route `/chat` → Chat page (AI assistant, formerly Home)
- Navigation order: Projects, Chat, Agents, Q&A, Tasks
- Legacy routes `/home` and `/projects` redirect for backward compatibility

**Files changed:**
- `router.tsx` - Swapped paths, added legacy redirects
- `UserOrgSelector.tsx` - Reordered navigation, renamed Home→Chat
- `Sidebar.tsx`, `SidebarContextHeader.tsx` - Updated route references
- `AccessDenied.tsx`, `Page.tsx` - Updated breadcrumb defaults
- `SessionToolbar.tsx`, `SessionButtons.tsx`, `Create.tsx` - Updated post-action navigation

### 1.2 Workspace View (Tiled View Rename)

- Renamed "Tiled View"/"Tabs View" → "Workspace"
- Query param: `?tab=workspace`
- State persisted in localStorage (`helix_workspace_state`)
- Remembers: open panels, tabs, active tab, layout direction
- Restored when returning to Workspace view

### 1.3 Workspace Tab Types

Extended `TabData.type` to support:
- `'task'` - Spec task details (SpecTaskDetailContent)
- `'review'` - Spec review (DesignReviewContent)
- `'desktop'` - Team Desktop (ExternalAgentDesktopViewer)

Reviews now open in Workspace panes instead of navigating away.

### 1.4 Team Desktop Page

- New route: `/projects/:id/desktop/:sessionId`
- Breadcrumbs: Projects → {Project Name} → Team Desktop
- "Open in Workspace" button to embed in multi-panel view
- Consistent naming: "Team Desktop" (was "Exploratory Session")

### 1.5 Mobile/Touch UX Improvements

DesktopStreamViewer enhancements:
- iOS custom CSS fullscreen (native fullscreen blocks interaction)
- Picture-in-Picture support detection
- Larger toolbar icons for touch devices
- Better tap detection thresholds
- Reduced cursor sensitivity (0.8 vs 1.5)

### 1.6 Spec Task Detail & Review Pages

Created proper pages with browser navigation:
- `SpecTaskDetailPage.tsx` - `/projects/:id/tasks/:taskId`
- `SpecTaskReviewPage.tsx` - `/projects/:id/tasks/:taskId/review/:reviewId`
- Browser back button works correctly
- Bookmarkable URLs

---

## Part 2: Roadmap - Non-Technical User Experience

### Priority: HIGH

The entire Projects experience should be usable by:
- **Non-technical users** - No Git/code knowledge required
- **Technical users** - Full access to Git features when needed

### 2.1 Rename "Code" to "Files"

**Current:** Header shows "Code" tab in project view
**Change:** Rename to "Files"

**Rationale:** "Code" implies programming, but users may upload Word docs, PDFs, images, etc.

**Files to modify:**
- `SpecTasksPage.tsx` - Tab label
- `SpecTaskKanbanBoard.tsx` - Any references
- Any component showing the "Code" tab

### 2.2 Drag-and-Drop File Upload Everywhere

**Requirement:** Users should be able to drag files onto any project view to upload them.

**Where it should work:**
- Kanban board view
- Workspace view
- Task detail view
- Project settings page
- Files/Code tab

**Implementation:**
1. Create `ProjectDropZone` wrapper component
2. Wrap all project views with drop zone
3. Show overlay when dragging files over project
4. Upload to project's primary repo
5. Show progress indicator
6. Refresh file list after upload

**UX Details:**
- Drop zone overlay: semi-transparent with "Drop files to upload" message
- Accept any file type (Word, PDF, images, code, etc.)
- Show toast notification on success/failure
- Handle multiple files at once

**Files to create/modify:**
- `src/components/project/ProjectDropZone.tsx` (new)
- `src/pages/SpecTasksPage.tsx` - Wrap with drop zone
- `src/components/tasks/SpecTaskKanbanBoard.tsx` - Ensure works with overlay
- `src/components/tasks/TabsView.tsx` - Ensure works with overlay

### 2.3 Simplified New Project Creation

**Current:** Project creation requires understanding Git repos

**New Flow:**
1. User clicks "New Project"
2. Dialog shows ONLY: Name, Description, optional icon
3. Clicking "Create" auto-creates internal Helix repo
4. "Advanced" expander reveals Git connection options

**Technical Details:**
- Auto-create internal Helix repo with project name
- Set as primary repo automatically
- No "Connect Repository" required for basic use

**Files to modify:**
- `src/pages/Projects.tsx` - Simplify create dialog
- Backend: Auto-create repo endpoint

### 2.4 File Browser Improvements

**Current:** Files tab shows Git repository structure

**Improvements:**
- Hide `.git` folder by default
- Show file icons based on type
- Double-click to open in desktop agent
- Right-click context menu: Open, Download, Delete, Rename
- Breadcrumb navigation within folders

---

## Part 3: Roadmap - Diff View

### Priority: HIGH

### 3.1 Live Diff Tab in Task Details

**Requirement:** Show file changes (diffs) as a tab in spec task detail page with LIVE updates.

**Use Cases:**
- Watch agent make changes in real-time
- Review code changes before approving
- See what the agent modified (including uncommitted changes)
- Compare versions of any file (not just code)

**Critical Feature: Live Updates from Container**

When the desktop container is running:
- Show LIVE diffs as the agent edits files
- Include uncommitted/unsaved changes
- Poll or WebSocket for real-time updates
- Show "watching" indicator when live

When container is NOT running:
- Fall back to git diff between task branch and main branch
- Show last known state of changes

**Implementation:**

**New Tab:** "Changes" in SpecTaskDetailContent
- Shows list of changed files (live updating when container running)
- Click file to see diff
- Badge showing number of changed files
- "Live" indicator when container is active
- Support for:
  - Code files (syntax highlighting)
  - Text files
  - Binary files (show "Binary file changed")
  - Image files (side-by-side preview)
  - New files (green, full content)
  - Deleted files (red, strikethrough)

**Diff Display:**
- Split view (old | new) or unified view toggle
- Line numbers
- Syntax highlighting for code
- Expand/collapse unchanged sections
- "View full file" link
- Auto-scroll to latest change (optional)

**Backend Requirements:**

1. **Container File Watch API** (new endpoint)
   - `GET /api/v1/external-agents/{sessionId}/files/diff`
   - Returns list of changed files + diff content
   - Compares container's working directory against main branch
   - Includes uncommitted changes

2. **Git Diff API** (fallback, may exist)
   - `GET /api/v1/git-repositories/{repoId}/diff?base=main&head={branch}`
   - Returns git diff between two refs

3. **WebSocket Updates** (optional, for true live)
   - Push file change events from container
   - Notify when files are modified/saved

**Files to create (frontend):**
- `src/components/tasks/DiffViewer.tsx` - Main diff component
- `src/components/tasks/DiffFileList.tsx` - List of changed files
- `src/components/tasks/DiffContent.tsx` - Individual file diff
- `src/hooks/useLiveFileDiff.ts` - Hook for polling/subscribing to file changes

**Files to create (backend):**
- `api/pkg/server/external_agents_diff.go` - Container file diff endpoint
- API types for diff response

**Libraries to consider:**
- Monaco Editor diff view (already using Monaco in codebase)
- `react-diff-viewer-continued` for simpler diff display

**Data Flow:**
```
Container Running:
[Desktop Container] -> [Hydra/API] -> [Frontend Poll/WS] -> [DiffViewer]
                      (file watch)   (every 2-5 sec)

Container Stopped:
[Git Repository] -> [API git diff] -> [DiffViewer]
```

---

## Part 4: Roadmap - Desktop Environment

### Priority: MEDIUM

### 4.1 Install OnlyOffice in Desktop Images

**Requirement:** Desktop agent should be able to open Office documents.

**Packages to install:**
```dockerfile
# Ubuntu/Sway images
RUN wget -qO- https://download.onlyoffice.com/install/desktop/editors/linux/onlyoffice-desktopeditors_amd64.deb && \
    dpkg -i onlyoffice-desktopeditors_amd64.deb && \
    apt-get install -f
```

**Supported formats:**
- Word: .docx, .doc, .odt, .rtf
- Excel: .xlsx, .xls, .ods, .csv
- PowerPoint: .pptx, .ppt, .odp

**Files to modify:**
- `desktop/Dockerfile.ubuntu`
- `desktop/Dockerfile.sway` (if applicable)

**Testing:**
1. Upload Word doc to project
2. Create task: "Open and summarize the document"
3. Agent should open OnlyOffice and read content

---

## Part 5: Roadmap - UI/UX Polish

### Priority: MEDIUM

### 5.1 Chat Widget Consolidation

**Current:** Team Desktop uses `EmbeddedSessionView`, SpecTaskDetailContent uses different chat implementation.

**Change:** Consolidate to single chat component.

### 5.2 Workspace Info Button

Add info button (ℹ️) on Workspace page explaining:
- What Workspace view is for
- How to use split panes (drag dividers)
- How to add/remove panels
- Keyboard shortcuts if any

### 5.3 Mobile UX Polish

- Hide Workspace option on phones (< 768px)
- Optimize for tablet portrait/landscape
- Touch-friendly panel resizing
- Swipe gestures for panel navigation

### 5.4 Cleanup Deprecated Code

After migration stable:
- Remove `SpecTaskDetailDialog.tsx`
- Simplify `DesignReviewViewer.tsx` (remove floating window logic)
- Remove `FloatingModal` usage in SpecTasksPage
- Remove unused Window tiling/dragging code

### 5.5 Remove Serif Fonts

`DesignReviewViewer.tsx` uses Palatino/Georgia fonts.
Change to theme defaults for consistency.

---

## Part 6: Technical Details

### Routes Summary

| Route | Component | Description |
|-------|-----------|-------------|
| `/` | Projects | Landing page, project list |
| `/chat` | Home | AI chat assistant |
| `/projects/:id/specs` | SpecTasksPage | Project kanban/workspace |
| `/projects/:id/tasks/:taskId` | SpecTaskDetailPage | Task details |
| `/projects/:id/tasks/:taskId/review/:reviewId` | SpecTaskReviewPage | Spec review |
| `/projects/:id/desktop/:sessionId` | TeamDesktopPage | Team Desktop |
| `/projects/:id/settings` | ProjectSettings | Project config |

### localStorage Keys

| Key | Purpose |
|-----|---------|
| `helix_spectask_view_mode` | Kanban vs Workspace preference |
| `helix_workspace_state` | Panel layout, open tabs |

### Component Hierarchy

```
SpecTasksPage
├── Header (view toggle, actions)
├── Kanban View (SpecTaskKanbanBoard)
│   ├── TaskCard (with desktop thumbnail)
│   └── ProjectDropZone (drag-drop files)
├── Workspace View (TabsView)
│   ├── Panel (resizable)
│   │   ├── Tab Bar
│   │   └── Content (task | review | desktop)
│   └── ProjectDropZone
└── Audit View (ProjectAuditTrail)
```

---

## Implementation Order

1. ✅ Navigation refactor (Projects landing page)
2. ✅ Workspace rename and persistence
3. ✅ Team Desktop page
4. ✅ Rename "Code" to "Files"
5. ✅ Drag-drop file upload everywhere
6. 🔲 Diff view tab in task details
7. ✅ Simplified project creation
8. ✅ OnlyOffice installation
9. 🔲 Chat widget consolidation
10. 🔲 Workspace info button
11. 🔲 Mobile polish
12. 🔲 Code cleanup

---

## Verification Checklist

### Navigation
- [ ] Visiting `/` shows Projects page
- [ ] Clicking logo goes to Projects
- [ ] "Chat" nav item goes to `/chat`
- [ ] Back button works on all pages

### File Upload
- [ ] Can drag files onto Kanban view
- [ ] Can drag files onto Workspace view
- [ ] Progress indicator shows
- [ ] Files appear in Files tab after upload

### Diff View
- [ ] "Changes" tab visible in task details
- [ ] Changed files listed
- [ ] Clicking file shows diff
- [ ] Syntax highlighting works

### Non-Technical UX
- [ ] "Code" renamed to "Files"
- [ ] Can create project without Git knowledge
- [ ] Can upload Word docs and have agent open them
