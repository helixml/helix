# Design: Kanban Board in Split View

## Architecture Overview

The split-view workspace (`TabsView.tsx`) uses a tree-based panel system where each leaf node contains tabs. Tabs have a `type` field that determines rendering:
- `"task"` - Task detail view
- `"review"` - Design review content
- `"desktop"` - External agent desktop viewer
- `"create"` - New task form

We'll add `"kanban"` as a new tab type.

## Key Files

| File | Purpose |
|------|---------|
| `frontend/src/components/tasks/TabsView.tsx` | Split view container, tab management, panel rendering |
| `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` | Kanban board component |
| `frontend/src/pages/SpecTasksPage.tsx` | Parent page that switches between kanban/workspace views |

## Implementation Approach

### 1. Extend TabData Interface

```typescript
// TabsView.tsx - existing interface
interface TabData {
  id: string;
  type: "task" | "review" | "desktop" | "create" | "kanban"; // Add "kanban"
  // ... existing fields
}
```

### 2. Add Kanban Rendering in TaskPanel

In the `TaskPanel` component's content area (around line 1345), add a new conditional:

```typescript
activeTab.type === "kanban" ? (
  <SpecTaskKanbanBoard
    projectId={projectId}
    onTaskClick={(task) => {
      // Open task in new split or existing tab
      onOpenTaskInSplit(task.id);
    }}
    // Compact mode for narrower pane
  />
) : activeTab.type === "desktop" ...
```

### 3. Add "Open Kanban" Action

Add a button in the workspace toolbar (alongside existing "+" button):
- Icon: Kanban grid icon from lucide-react
- Behavior: Call `handleAddKanban()` which creates/focuses kanban tab

### 4. Task Click Handler

When a task is clicked in the embedded Kanban:
1. Check if task already open → activate that tab
2. Otherwise, create new task tab in a split pane (right side of Kanban)
3. Use existing `handleSplitPanel` or `handleAddTab` patterns

## Patterns Found in Codebase

- **Tab creation**: Uses `handleAddTab(panelId)` to add generic tabs, `handleAddDesktop(panelId, sessionId, title)` for desktop tabs
- **Split behavior**: `handleSplitPanel(panelId, direction, tabId)` creates vertical/horizontal splits
- **Deduplication**: See `handleOpenReview` which checks `allLeaves.find(...)` before creating duplicate tabs
- **Persistence**: Workspace state saved to localStorage via `serializeNode`/`deserializeNode`

## Decisions

| Decision | Rationale |
|----------|-----------|
| One Kanban tab per workspace | Simplifies UX, prevents confusion with multiple boards |
| Kanban opens tasks in adjacent split | Keeps board visible while viewing task details |
| Reuse existing `SpecTaskKanbanBoard` | No need for new component, just wire up `onTaskClick` differently |

## Serialization

Add kanban to the persistence logic:

```typescript
// In serializeNode - kanban tabs are persistable
if (tab.type === "kanban") {
  return `kanban:${tab.id}`;
}

// In deserializeNode
if (tabId.startsWith("kanban:")) {
  return { id: tabId, type: "kanban" };
}
```
