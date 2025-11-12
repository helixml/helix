# Spec Tasks UI Improvements

**Date**: 2025-10-30
**Status**: In Progress
**Author**: System / Luke Marsden

## Overview

This document captures a series of UI improvements for the Spec Tasks feature, focusing on agent selection, duplicate element removal, and task initiation workflow.

## Completed Improvements

### 1. Agent Selection for Spec Tasks ✅

**Problem**: When creating a new Spec Task, the system always used a hardcoded default agent for spec generation. There was no way for users to select which Helix agent should generate the specifications.

**Solution Implemented**:
- Added `app_id` field to `ServicesCreateTaskRequest` backend struct
- Updated backend logic to accept and use the provided `app_id` when creating tasks
- Implemented agent selection dropdown in the "New SpecTask" dialog
- Dropdown shows all available agents the user has access to (same list as Agents page)
- Defaults to system default agent if no selection is made

**Files Changed**:
- `api/pkg/services/spec_driven_task_service.go`: Added `app_id` field and logic
- `frontend/src/pages/SpecTasksPage.tsx`: Added agent selection dropdown
- Regenerated TypeScript API client to include new field

**Benefits**:
- Users can now choose specialized agents for different types of specifications
- Better control over the spec generation process
- Leverages existing custom agents users have created

**Default Agent Creation (Lazy Pattern)**:
- Dropdown shows "Create Default Agent ..." at the bottom of the list
- When selected and submitted, automatically creates a new external agent with:
  - Name: "Default Spec Agent"
  - Type: External Agent (Zed)
  - System prompt: General-purpose agent for both planning and implementation
  - No additional configuration required
- Next time the dialog opens, the created agent appears in the regular list
- Auto-selection logic:
  - If "Default Spec Agent" exists → auto-select it
  - If no agents exist → auto-select "Create Default Agent ..."
  - If agents exist but no default → auto-select first agent

### 2. Removed Duplicate UI Elements ✅

**Problem**: The Spec Tasks page had duplicate controls:
- "New Task" button appeared both in the top bar (from SpecTasksPage) and inside the Kanban board
- "Refresh" button was duplicated in the same way
- The inside-board version had a less polished dialog compared to the top bar version

**Solution Implemented**:
- Removed duplicate "Refresh" and "New Task" buttons from inside the Kanban board component
- Removed the entire Create Task dialog from the Kanban board (lines 935-992 removed)
- Kept only the top bar buttons which have the better implementation

**Files Changed**:
- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx`

**Benefits**:
- Cleaner, less confusing UI
- Single source of truth for task creation
- Better user experience with the improved dialog

### 3. Removed Non-Functional "Assign Zed Agent" Button ✅

**Problem**: Tasks in the "Planning" phase showed an "Assign Zed Agent" button that didn't work properly and was unnecessary.

**Solution Implemented**:
- Removed "Assign Zed Agent" button from task cards
- Removed associated `handleAssignAgent` function reference
- Cleaned up unused state variables (`createDialogOpen`, `newTaskName`, `newTaskDescription`, `refreshing`)

**Files Changed**:
- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx`

**Benefits**:
- Removed confusing non-functional UI element
- Cleaner task cards
- Agent assignment now happens automatically when task is created

## Pending Improvements

### 4. Task Initiation Workflow 🔄

**Current Behavior**: When a Spec Task is created, it automatically starts spec generation immediately.

**Discussion Points**:
- Should tasks start in the "Backlog" column initially?
- Should there be an explicit "Start Planning" button on backlog tasks?
- Or should dragging a task from "Backlog" to "Planning" column trigger spec generation?
- Current auto-start may be acceptable - needs user feedback

**Recommendation**: Keep current auto-start behavior for now, but consider adding:
- Visual feedback when spec generation starts
- Clear status indicators in the Backlog column
- Future enhancement: Add drag-and-drop to manually transition tasks between phases

### 5. Drag and Drop Support (Roadmap) 📋

**Goal**: Allow users to drag tasks between Kanban columns to change their phase/status.

**Implementation Considerations**:
- Already using `@dnd-kit` library (imported but drag disabled to prevent infinite loops)
- Need to implement proper drop handlers
- Need to prevent state update loops that were causing issues
- Should trigger appropriate backend actions (e.g., dragging to "Planning" starts spec generation)

**Priority**: Medium (nice-to-have, not critical)

## Technical Notes

### Backend Changes

The `CreateTaskRequest` struct now includes an optional `app_id` field:

```go
type CreateTaskRequest struct {
    ProjectID string `json:"project_id"`
    Prompt    string `json:"prompt"`
    Type      string `json:"type"`
    Priority  string `json:"priority"`
    UserID    string `json:"user_id"`
    AppID     string `json:"app_id"` // Optional: Helix agent to use
}
```

Logic prioritizes provided `app_id` over default:

```go
specAgent := s.helixAgentID
if req.AppID != "" {
    specAgent = req.AppID
}
```

### Frontend Changes

Agent selection dropdown implementation:

```typescript
<FormControl fullWidth>
  <InputLabel>Helix Agent</InputLabel>
  <Select value={selectedHelixAgent} onChange={(e) => setSelectedHelixAgent(e.target.value)}>
    {apps.apps.map((app) => (
      <MenuItem key={app.id} value={app.id}>
        {app.config?.helix?.name || app.name || 'Unnamed Agent'}
      </MenuItem>
    ))}
    <MenuItem value="__create_default__">
      <em>Create Default Agent...</em>
    </MenuItem>
  </Select>
</FormControl>
```

Default agent creation on form submission:

```typescript
// Create default agent if requested
if (selectedHelixAgent === '__create_default__') {
  const newAgent = await apps.createAgent({
    name: 'Default Spec Agent',
    systemPrompt: 'You are a planning agent that generates comprehensive specifications...',
    // Minimal configuration - external agent type
    reasoningModelProvider: '',
    reasoningModel: '',
    // ... other fields empty
  });
  agentId = newAgent.id;
  apps.loadApps(); // Reload so it appears in list next time
}
```

## Testing Plan

### Agent Selection Testing
1. ⏳ **First-time user flow**: Create task with no existing agents
   - Should auto-select "Create Default Agent ..."
   - Submit should create "Default Spec Agent"
   - Agent should appear in list on next task creation
2. ⏳ **Existing default agent**: Open dialog with "Default Spec Agent" present
   - Should auto-select "Default Spec Agent"
   - Submit should use existing agent (not create duplicate)
3. ⏳ **Custom agent selection**: Create a new Spec Task with a specific agent selected
   - Verify the correct agent is used for spec generation
   - Check that external agent session starts correctly
4. ⏳ **Multiple agents**: Open dialog with multiple agents
   - Should auto-select first agent (or "Default Spec Agent" if exists)
   - List should show all agents + "Create Default Agent ..." at bottom

### UI Cleanup Testing
1. ✅ Verify duplicate buttons are removed from Kanban board
2. ⏳ Confirm "New Task" dialog only appears from top bar button
3. ⏳ Verify "Assign Zed Agent" button no longer appears on tasks
4. ⏳ Check that task creation still works correctly

## Future Considerations

1. **Agent Filtering**: Consider filtering agent list to only show agents suitable for spec generation
2. **Agent Recommendations**: Could suggest optimal agents based on task type/description
3. **Multi-Agent Workflow**: Support for different agents at different phases (spec vs implementation)
4. **Status Indicators**: Better visual feedback for which agent is working on which task
5. **Session Linking**: Improved visibility into which external agent sessions belong to which tasks

## Related Issues & Discussions

- Tasks should start automatically vs. manual trigger - **needs decision**
- Drag and drop implementation - **postponed to roadmap**
- Agent selection at creation time - **implemented**
- Duplicate UI cleanup - **completed**

## Conclusion

The Spec Tasks feature now provides users with meaningful control over agent selection while maintaining a clean, intuitive UI. The removal of duplicate elements reduces confusion and provides a more cohesive experience. Future enhancements around task initiation workflow and drag-and-drop support will further improve usability.
