# Design Review Agent Comment Integration

**Date:** 2025-11-11
**Author:** Claude (via Claude Code)
**Status:** Design Phase

## Overview

Implement a simplified, Google Docs-style comment system for design document review where comments automatically trigger agent responses. The agent can view comments, respond in real-time via WebSocket streaming, and update design documents to address feedback.

## Current State

### Existing Infrastructure

1. **DesignReviewViewer Component** (`frontend/src/components/spec-tasks/DesignReviewViewer.tsx`)
   - 711 lines, feature-complete review UI
   - Text selection and commenting already implemented
   - Currently uses Dialog (modal) - blocks interaction with other UI
   - Has comment types: general, question, suggestion, critical, praise
   - Location: Lines 258-709 wrap content in `<Dialog>`

2. **Design Review Service** (`frontend/src/services/designReviewService.ts`)
   - React Query hooks for CRUD operations
   - Comment type system with 5 types
   - Mutation hooks: `useCreateComment`, `useResolveComment`

3. **Backend Handlers** (`api/pkg/server/spec_task_design_review_handlers.go`)
   - Endpoints: list reviews, get review, submit review, create comment, list comments, resolve comment
   - Authorization fixed for personal projects (user_id check)
   - Routes registered at `/api/v1/spec-tasks/{spec_task_id}/design-reviews`

4. **WebSocket Infrastructure**
   - External agent sync via WebSocket (`api/pkg/server/websocket_external_agent_sync.go`)
   - Sessions API for sending messages to agents
   - Message streaming already works for spec generation agents

5. **Spec Task Agent System**
   - Planning agents run in Zed containers with git access
   - Agents have access to helix-specs worktree at `~/work/helix-specs`
   - Design docs stored at `~/work/helix-specs/design/tasks/{date}_{name}_{taskid}/`
   - Post-push hooks detect changes and auto-transition tasks

### Current Issues

1. **DesignReviewViewer is a Modal (Dialog)**
   - User wants floating window like SpecTaskDetailDialog
   - Need draggable, resizable, tiling support
   - Pattern exists in SpecTaskDetailDialog.tsx (lines 52-299)

2. **Comment Types Are Overcomplicated**
   - User wants single comment type
   - Remove: general, question, suggestion, critical, praise
   - Keep: just "comment"

3. **No Agent Integration**
   - Comments don't trigger agent interactions
   - Need to send comment as message to agent's session
   - Need to stream response back to comment

4. **Missing Features**
   - No X button to dismiss comments
   - No comment log panel (Google Docs style)
   - No auto-resolution when highlighted text disappears

## Requirements

### Functional Requirements

1. **Simplified Comment System**
   - Single comment type (no categories)
   - Highlight text → add comment
   - Comment appears inline in document view

2. **Automatic Agent Integration**
   - When comment created → immediately send as prompt to agent
   - Agent sees: "User commented on design doc: '{highlighted_text}' - Comment: '{comment_text}'"
   - Agent response streams into comment thread
   - Agent can update design doc and push changes

3. **Auto-Resolution**
   - Monitor design doc updates (git commits)
   - If quoted_text no longer exists in document → auto-resolve comment
   - Mark as "Resolved (text updated)"

4. **Manual Dismissal**
   - X button on each comment
   - Marks comment as resolved
   - Preserves in comment log

5. **Comment Log Panel**
   - Google Docs style comment sidebar/panel
   - Shows all comments (resolved and unresolved)
   - Displays quoted text from version when comment was made
   - Can toggle show/hide

6. **Floating Window UI**
   - Not a blocking modal - user can interact with Kanban board behind it
   - Draggable by title bar
   - Resizable with corner/edge handles
   - Snap to screen positions (full, half-left, half-right, corners)
   - Same pattern as SpecTaskDetailDialog

## Technical Design

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Browser (Frontend)                        │
│                                                              │
│  DesignReviewViewer (Floating Window)                       │
│  ├── Document Tabs (requirements, design, tasks)            │
│  ├── Text Selection Handler                                 │
│  ├── Comment Form (simplified - no type selector)           │
│  ├── Comment Display (inline markers + sidebar)             │
│  ├── Comment Log Panel (toggle)                             │
│  └── WebSocket Listener (for agent responses)               │
│                                                              │
│  Comment Creation Flow:                                     │
│  1. User highlights text                                     │
│  2. Comment form appears                                     │
│  3. On submit:                                               │
│     a. POST /api/v1/spec-tasks/{id}/design-reviews/{rid}/comments
│     b. POST /api/v1/sessions/{spec_session_id}/chat         │
│        - Message: "Address design review comment..."         │
│     c. Subscribe to WebSocket for response                   │
│     d. Stream response into comment.agent_response           │
│                                                              │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    Backend (Go API)                          │
│                                                              │
│  Design Review Comment Handlers                             │
│  ├── createDesignReviewComment()                            │
│  │   ├── Create comment in DB                               │
│  │   ├── Send prompt to agent session via Sessions API      │
│  │   └── Return comment ID                                  │
│  │                                                           │
│  └── WebSocket External Agent Sync                          │
│      ├── Receive agent response                             │
│      ├── Store response in comment.agent_response           │
│      └── Broadcast update to frontend                       │
│                                                              │
│  Post-Push Hook                                             │
│  ├── Detect helix-specs branch push                         │
│  ├── Read updated documents                                 │
│  ├── Check if any comment.quoted_text still exists          │
│  └── Auto-resolve comments with missing text                │
│                                                              │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│              Zed Agent Container (Planning)                  │
│                                                              │
│  ~/work/helix-specs/design/tasks/{date}_{name}_{taskid}/   │
│  ├── requirements.md                                         │
│  ├── design.md                                               │
│  └── tasks.md                                                │
│                                                              │
│  Agent receives prompt:                                      │
│  "A comment was left on the design document:                 │
│   Quoted text: '{highlighted_text}'                          │
│   Comment: '{comment_text}'                                  │
│                                                              │
│   Please:                                                    │
│   1. Respond to the comment explaining your approach         │
│   2. Update the design document if needed                    │
│   3. Push changes to helix-specs branch                      │
│                                                              │
│   Your response will be shown to the reviewer."              │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Data Model Changes

**Database Schema** (PostgreSQL - `api/pkg/store/spec_task_design_review_store.go`)

```go
type SpecTaskDesignReviewComment struct {
    ID           string    `gorm:"type:varchar(255);primaryKey"`
    ReviewID     string    `gorm:"type:varchar(255);index;not null"`
    CommentedBy  string    `gorm:"type:varchar(255);not null"`

    // Document context
    DocumentType string    `gorm:"type:varchar(50);not null"` // requirements, technical_design, implementation_plan
    QuotedText   string    `gorm:"type:text"` // Text that was highlighted
    StartOffset  int       `gorm:"type:integer"` // Character offset (for precise matching)
    EndOffset    int       `gorm:"type:integer"`

    // Comment content
    CommentText  string    `gorm:"type:text;not null"`

    // REMOVE: CommentType string `gorm:"type:varchar(50)"` // DELETED - no longer needed

    // Agent integration (NEW FIELDS)
    AgentResponse    string    `gorm:"type:text"` // Agent's response to comment
    AgentResponseAt  *time.Time `gorm:"type:timestamp with time zone"` // When agent responded
    InteractionID    string    `gorm:"type:varchar(255)"` // Link to Helix interaction

    // Resolution
    Resolved     bool      `gorm:"default:false"`
    ResolvedBy   string    `gorm:"type:varchar(255)"`
    ResolvedAt   *time.Time `gorm:"type:timestamp with time zone"`
    ResolutionReason string `gorm:"type:varchar(100)"` // "manual", "auto_text_removed", "agent_updated"

    CreatedAt    time.Time `gorm:"autoCreateTime"`
    UpdatedAt    time.Time `gorm:"autoUpdateTime"`
}
```

### Component Structure Changes

**DesignReviewViewer** - Convert from Dialog to Floating Window

```tsx
// BEFORE (Current - Lines 278-279)
return (
  <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
    ...
  </Dialog>
)

// AFTER (New - Copy pattern from SpecTaskDetailDialog)
return (
  <>
    {/* Snap Preview Overlay */}
    {snapPreview && <Box sx={{ position: 'fixed', ... }} />}

    {/* Floating Window */}
    <Paper
      ref={nodeRef}
      sx={{
        position: 'fixed',
        ...getPositionStyle(), // Based on position state
        display: open ? 'flex' : 'none',
        flexDirection: 'column',
        zIndex: 10000,
        // Draggable, resizable
      }}
    >
      {/* Resize Handles */}
      {position === 'center' && getResizeHandles().map(...)}

      {/* Draggable Title Bar */}
      <Box onMouseDown={handleMouseDown} sx={{ cursor: 'move', ... }}>
        <DragIndicatorIcon />
        <Typography>Design Review</Typography>
        <IconButton onClick={() => setTileMenuAnchor(e)}>
          <GridViewOutlined />
        </IconButton>
        <IconButton onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </Box>

      {/* Content */}
      {/* ... existing tabs, documents, comments ... */}
    </Paper>

    {/* Tiling Menu */}
    <Menu anchorEl={tileMenuAnchor} ...>
      <MenuItem onClick={() => handleTile('full')}>Full Screen</MenuItem>
      <MenuItem onClick={() => handleTile('half-left')}>Half Left</MenuItem>
      ...
    </Menu>
  </>
)
```

**State Management**

```tsx
// Add dragging/positioning state (copy from SpecTaskDetailDialog lines 52-68)
const [position, setPosition] = useState<WindowPosition>('center')
const [isSnapped, setIsSnapped] = useState(false)
const [isDragging, setIsDragging] = useState(false)
const [dragStart, setDragStart] = useState<{ x: number; y: number } | null>(null)
const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 })
const [windowPos, setWindowPos] = useState({ x: 100, y: 100 })
const [snapPreview, setSnapPreview] = useState<string | null>(null)

// Add resize support
const { size, setSize, isResizing, getResizeHandles } = useResize({
  initialSize: { width: Math.min(1200, window.innerWidth * 0.6), height: window.innerHeight * 0.8 },
  minSize: { width: 600, height: 400 },
  maxSize: { width: window.innerWidth, height: window.innerHeight },
})

// Add comment log panel state
const [showCommentLog, setShowCommentLog] = useState(false)
```

### API Changes

**New Endpoint** - Send Comment to Agent

```go
// api/pkg/server/spec_task_design_review_handlers.go

func (s *HelixAPIServer) createDesignReviewComment(w http.ResponseWriter, r *http.Request) {
    // ... existing comment creation code ...

    // NEW: Send comment to agent immediately after creating
    if specTask.SpecSessionID != "" {
        go s.sendCommentToAgent(context.Background(), specTask, comment)
    }

    // Return comment
    json.NewEncoder(w).Encode(comment)
}

func (s *HelixAPIServer) sendCommentToAgent(
    ctx context.Context,
    task *types.SpecTask,
    comment *types.SpecTaskDesignReviewComment,
) error {
    // Build prompt for agent
    prompt := fmt.Sprintf(`A reviewer left a comment on your design document:

**Document:** %s
**Quoted Text:**
> %s

**Comment:**
%s

Please respond to this comment and explain your approach. If the reviewer's feedback requires changes to the design, update the relevant document and push your changes to helix-specs branch.`,
        comment.DocumentType,
        comment.QuotedText,
        comment.CommentText)

    // Send message to agent's session
    interaction := &types.Interaction{
        ID:            system.GenerateInteractionID(),
        SessionID:     task.SpecSessionID,
        UserID:        comment.CommentedBy,
        PromptMessage: prompt,
        State:         types.InteractionStateWaiting,
    }

    _, err := s.Store.CreateInteraction(ctx, interaction)
    if err != nil {
        log.Error().Err(err).Msg("Failed to create interaction for comment")
        return err
    }

    // Store interaction ID in comment for linking
    comment.InteractionID = interaction.ID
    s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment)

    return nil
}
```

**WebSocket Handler Update** - Link Agent Response to Comment

```go
// api/pkg/server/websocket_external_agent_sync.go

// In handleAgentMessage() or similar:
// When agent responds, check if interaction is linked to a comment

func (s *HelixAPIServer) linkAgentResponseToComment(
    ctx context.Context,
    interaction *types.Interaction,
    agentResponse string,
) {
    if interaction.ID == "" {
        return
    }

    // Find comment by interaction ID
    comment, err := s.Store.GetCommentByInteractionID(ctx, interaction.ID)
    if err != nil {
        return // Not a comment interaction
    }

    // Update comment with agent response
    comment.AgentResponse = agentResponse
    now := time.Now()
    comment.AgentResponseAt = &now

    s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment)

    log.Info().
        Str("comment_id", comment.ID).
        Str("interaction_id", interaction.ID).
        Msg("Linked agent response to design review comment")
}
```

**Auto-Resolution on Document Update**

```go
// api/pkg/services/git_http_server.go - in handlePostPushHook()

func (s *GitHTTPServer) handlePostPushHook(ctx context.Context, repoID, repoPath string) {
    // ... existing design doc detection ...

    // After moving task to review, check for comment resolution
    if hasDesignDocs {
        go s.checkCommentResolution(ctx, repoID, repoPath)
    }
}

func (s *GitHTTPServer) checkCommentResolution(ctx context.Context, repoID, repoPath string) {
    // Get all unresolved comments for this repository's spec tasks
    // For each comment:
    //   1. Read current version of document from helix-specs
    //   2. Check if comment.quoted_text still exists
    //   3. If not found → mark as resolved with reason="auto_text_removed"

    // Pseudo-code:
    specTasks := getSpecTasksForRepo(repoID)
    for task := range specTasks {
        comments := getUnresolvedComments(task.ID)
        for comment := range comments {
            docContent := readDocFromGit(repoPath, comment.DocumentType, task.ID)
            if !strings.Contains(docContent, comment.QuotedText) {
                comment.Resolved = true
                comment.ResolvedBy = "system"
                comment.ResolutionReason = "auto_text_removed"
                updateComment(comment)
            }
        }
    }
}
```

### Frontend Changes

**DesignReviewViewer Component**

1. **Convert Dialog → Floating Window**
   - Import `useResize` hook from `../../hooks/useResize`
   - Add positioning state (copy from SpecTaskDetailDialog lines 52-68)
   - Add mouse event handlers for dragging (lines 226-299)
   - Replace `<Dialog>` with `<Paper>` at fixed position
   - Add resize handles
   - Add tiling menu
   - Set `zIndex: 10000` to float above Kanban but not block clicks

2. **Simplify Comment Types**
   - Remove `commentType` state variable
   - Remove type selector from comment form UI
   - Remove `comment_type` from createComment API call
   - Update TypeScript types to make comment_type optional

3. **Add Agent Response Display**
   - Add `agentResponse` field to comment display
   - Show streaming indicator while agent is responding
   - Subscribe to WebSocket for interaction updates
   - Update comment UI when agent response arrives

4. **Add Comment Dismissal**
   - Add X button to each comment
   - Call `resolveCommentMutation` with `reason: 'manual'`
   - Grey out dismissed comments but keep in log

5. **Add Comment Log Panel**
   - Add toggle button in title bar
   - Slide-out panel on right side of floating window
   - List all comments (resolved + unresolved)
   - Show quoted text and resolution status
   - Click to jump to document section

**designReviewService.ts Updates**

```typescript
// Make comment_type optional
export interface DesignReviewComment {
  // ... existing fields ...
  comment_type?: string // Optional - simplified to single type
  agent_response?: string // NEW
  agent_response_at?: string // NEW
  interaction_id?: string // NEW
  resolution_reason?: 'manual' | 'auto_text_removed' | 'agent_updated' // NEW
}

// Update create comment mutation
export function useCreateComment(specTaskId: string, reviewId: string) {
  return useMutation({
    mutationFn: async (request: {
      document_type: string
      quoted_text?: string
      comment_text: string
      // REMOVE: comment_type
    }) => {
      const response = await apiClient.post(
        `/api/v1/spec-tasks/${specTaskId}/design-reviews/${reviewId}/comments`,
        request
      )
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries(designReviewKeys.comments(specTaskId, reviewId))
    },
  })
}
```

### WebSocket Integration

**Frontend** - Subscribe to Comment Updates

```typescript
// In DesignReviewViewer component

useEffect(() => {
  if (!review?.spec_task?.spec_session_id) return

  // Subscribe to session updates
  const handleSessionUpdate = (event: any) => {
    if (event.type === 'interaction_updated') {
      // Check if interaction is linked to any of our comments
      const updatedComment = allComments.find(c => c.interaction_id === event.interaction_id)
      if (updatedComment) {
        // Refetch comments to get updated agent response
        queryClient.invalidateQueries(designReviewKeys.comments(specTaskId, reviewId))
      }
    }
  }

  streaming.subscribeToSession(review.spec_task.spec_session_id, handleSessionUpdate)

  return () => {
    streaming.unsubscribeFromSession(review.spec_task.spec_session_id, handleSessionUpdate)
  }
}, [review?.spec_task?.spec_session_id, allComments])
```

## Implementation Plan

### Phase 1: Structural Changes (Foundation)

**File:** `frontend/src/components/spec-tasks/DesignReviewViewer.tsx`

1. Import `useResize` hook
2. Add state variables for positioning (lines 52-68 from SpecTaskDetailDialog)
3. Add mouse event handlers for dragging (lines 226-299 from SpecTaskDetailDialog)
4. Replace `<Dialog>` wrapper with:
   - Snap preview overlay
   - `<Paper>` at fixed position
   - Resize handles
   - Tiling menu
5. Update `zIndex` to not block other UI elements
6. Test: Floating window can be dragged, resized, tiled

### Phase 2: Simplify Comment System

**Files:**
- `frontend/src/services/designReviewService.ts`
- `frontend/src/components/spec-tasks/DesignReviewViewer.tsx`
- `api/pkg/types/spec_task_design_review.go`

1. Make `comment_type` optional in TypeScript interface
2. Remove comment type selector from UI (lines 70-73)
3. Remove comment type dropdown from comment form
4. Update `handleCreateComment` to not send `comment_type`
5. Backend: Make `comment_type` column nullable in DB schema
6. Test: Can create comments without selecting type

### Phase 3: Agent Integration - Send Comments to Agent

**Files:**
- `api/pkg/server/spec_task_design_review_handlers.go`
- `api/pkg/types/spec_task_design_review.go`

1. Add new fields to SpecTaskDesignReviewComment struct:
   ```go
   AgentResponse   string
   AgentResponseAt *time.Time
   InteractionID   string
   ResolutionReason string
   ```

2. Run `AutoMigrate` to add columns (GORM handles this automatically)

3. Update `createDesignReviewComment` handler:
   ```go
   // After creating comment in DB
   comment, err := s.Store.CreateSpecTaskDesignReviewComment(ctx, &commentData)

   // Send to agent asynchronously
   if specTask.SpecSessionID != "" {
       go s.sendCommentToAgent(context.Background(), specTask, comment)
   }
   ```

4. Implement `sendCommentToAgent`:
   - Build prompt with quoted text + comment
   - Create interaction in agent's session
   - Link interaction_id back to comment
   - Agent receives prompt via existing WebSocket infrastructure

5. Test: Creating comment sends message to agent session

### Phase 4: Stream Agent Responses to Comments

**Files:**
- `api/pkg/server/websocket_external_agent_sync.go`
- `frontend/src/components/spec-tasks/DesignReviewViewer.tsx`

1. **Backend**: In WebSocket message handler, detect comment-linked interactions
   ```go
   func handleInteractionUpdate(interaction *types.Interaction) {
       // Check if this interaction has a comment
       comment := getCommentByInteractionID(interaction.ID)
       if comment != nil {
           // Update comment with agent's response
           comment.AgentResponse = interaction.ResponseMessage
           comment.AgentResponseAt = &now
           updateComment(comment)
       }
   }
   ```

2. **Frontend**: Subscribe to comment updates via WebSocket
   - Use existing streaming context
   - Listen for session updates
   - When interaction updates, refetch comments
   - Display agent response below user comment

3. **UI Changes**:
   - Add "Agent is responding..." indicator
   - Stream response text (updating in real-time)
   - Mark comment as "Addressed" when agent responds

4. Test: Agent response appears in comment thread in real-time

### Phase 5: Auto-Resolution

**Files:**
- `api/pkg/services/git_http_server.go`
- `api/pkg/store/spec_task_design_review_store.go`

1. Add `GetCommentByInteractionID` to store interface
2. Add `GetUnresolvedCommentsForTask` to store
3. In `handlePostPushHook`, after detecting design docs:
   ```go
   // Read updated document content
   for each unresolved comment {
       currentDocContent := readDocFromHelix Specs(comment.DocumentType, taskID)
       if !contains(currentDocContent, comment.QuotedText) {
           resolveComment(comment, "auto_text_removed")
       }
   }
   ```
4. Test: Update design doc to remove commented text → comment auto-resolves

### Phase 6: Manual Dismissal

**Files:**
- `frontend/src/components/spec-tasks/DesignReviewViewer.tsx`

1. Add X button to comment UI:
   ```tsx
   <IconButton
     size="small"
     onClick={() => resolveCommentMutation.mutate({
       commentId: comment.id,
       reason: 'manual'
     })}
   >
     <CloseIcon />
   </IconButton>
   ```

2. Update `useResolveComment` to accept reason parameter
3. Grey out resolved comments (keep visible for audit trail)
4. Test: X button dismisses comment

### Phase 7: Comment Log Panel

**Files:**
- `frontend/src/components/spec-tasks/DesignReviewViewer.tsx`

1. Add comment log toggle button to title bar:
   ```tsx
   <IconButton onClick={() => setShowCommentLog(!showCommentLog)}>
     <Badge badgeContent={unresolvedCount} color="error">
       <CommentIcon />
     </Badge>
   </IconButton>
   ```

2. Add slide-out panel (right side of window):
   ```tsx
   {showCommentLog && (
     <Box sx={{
       width: 300,
       borderLeft: 1,
       borderColor: 'divider',
       overflowY: 'auto',
     }}>
       <Typography variant="h6">Comments</Typography>
       {allComments.map(comment => (
         <Box key={comment.id} sx={{ p: 1, opacity: comment.resolved ? 0.6 : 1 }}>
           <Typography variant="caption">{comment.document_type}</Typography>
           <Typography variant="body2" sx={{ fontStyle: 'italic' }}>
             "{comment.quoted_text}"
           </Typography>
           <Typography variant="body2">{comment.comment_text}</Typography>
           {comment.agent_response && (
             <Typography variant="body2" color="primary">
               Agent: {comment.agent_response}
             </Typography>
           )}
           {comment.resolved && (
             <Chip label={comment.resolution_reason} size="small" />
           )}
         </Box>
       ))}
     </Box>
   )}
   ```

3. Test: Comment log shows all comments with status

## File Locations

### Frontend Files to Modify
- `frontend/src/components/spec-tasks/DesignReviewViewer.tsx` (main component)
- `frontend/src/services/designReviewService.ts` (API hooks)
- `frontend/src/hooks/useResize.ts` (check if exists, may need to reference from SpecTaskDetailDialog)

### Backend Files to Modify
- `api/pkg/types/spec_task_design_review.go` (add new fields)
- `api/pkg/server/spec_task_design_review_handlers.go` (agent integration)
- `api/pkg/store/spec_task_design_review_store.go` (new queries)
- `api/pkg/services/git_http_server.go` (auto-resolution in post-push hook)
- `api/pkg/server/websocket_external_agent_sync.go` (link responses to comments)

### New Files to Create
- None - all features fit into existing files

## Testing Strategy

### Manual Testing Checklist

1. **Floating Window**
   - [ ] Click "Review Documents" opens floating window
   - [ ] Can drag window by title bar
   - [ ] Can resize from corners and edges
   - [ ] Can snap to full, half-left, half-right, corners
   - [ ] Can interact with Kanban board while window is open
   - [ ] Window stays on top but doesn't block clicks

2. **Comment Creation**
   - [ ] Highlight text in design doc
   - [ ] Comment form appears
   - [ ] Type comment (no type selector visible)
   - [ ] Submit sends comment to agent
   - [ ] Comment appears in document
   - [ ] Agent session receives prompt

3. **Agent Response**
   - [ ] Agent response streams into comment
   - [ ] "Agent is responding..." indicator shows
   - [ ] Response updates in real-time via WebSocket
   - [ ] Can see full response when complete

4. **Auto-Resolution**
   - [ ] Agent updates design doc to remove commented text
   - [ ] Agent pushes to helix-specs
   - [ ] Comment auto-resolves with "text removed" status
   - [ ] Resolved comment greyed out but still visible

5. **Manual Dismissal**
   - [ ] Click X on comment
   - [ ] Comment immediately marked resolved
   - [ ] Greyed out but preserved in log

6. **Comment Log Panel**
   - [ ] Toggle button in title bar with unresolved count badge
   - [ ] Panel slides in from right
   - [ ] Shows all comments (resolved + unresolved)
   - [ ] Displays quoted text and status
   - [ ] Click comment to jump to document section

## Key Technical Decisions

### 1. Why Floating Window Instead of Modal?

**Decision:** Convert Dialog to fixed-position Paper with dragging support

**Rationale:**
- User needs to interact with Kanban board while reviewing docs
- Can position review window beside active agent session
- Familiar pattern from SpecTaskDetailDialog
- Better for multi-monitor workflows

**Implementation:** Copy positioning logic from SpecTaskDetailDialog (lines 52-357)

### 2. Comment Type Simplification

**Decision:** Remove all comment types, just have "comment"

**Rationale:**
- User feedback: "I don't think we need comment types"
- Simpler UX - fewer decisions for users
- All comments trigger same agent workflow
- Can add types back later if needed

**Implementation:** Make comment_type optional/nullable, remove UI selector

### 3. Agent Prompt Format

**Decision:** Send structured prompt with quoted text and comment

**Example:**
```
A reviewer left a comment on your design document:

**Document:** requirements
**Quoted Text:**
> User can toggle dark mode with a button in settings

**Comment:**
Should this be in the header instead for quick access?

Please respond to this comment and explain your approach. If changes are needed, update the design document and push to helix-specs branch.
```

**Rationale:**
- Clear context for agent
- Quoted text shows what user is referring to
- Explicit instruction to update docs if needed
- Agent can respond conversationally

### 4. Auto-Resolution Logic

**Decision:** Check quoted_text existence on every helix-specs push

**Rationale:**
- Simple string matching (contains check)
- Happens async in post-push hook (doesn't slow down commits)
- False positives rare (user unlikely to recreate exact text)
- Comment preserved in log even when resolved

**Edge Cases:**
- If text moved but still exists → stays unresolved (correct)
- If text partially changed → stays unresolved (user can manually dismiss)
- If agent adds text back → stays resolved (was resolved due to removal)

### 5. WebSocket Streaming Strategy

**Decision:** Reuse existing session WebSocket infrastructure

**Rationale:**
- Already streams interaction updates
- No new WebSocket connections needed
- Just link interaction_id to comment_id
- Frontend already has streaming context

**Flow:**
1. Comment created → interaction created
2. Interaction ID stored in comment
3. Agent responds → interaction updated
4. WebSocket event → frontend refetches comment
5. UI shows updated agent_response

## Database Migration

**Auto-Migration via GORM**

```go
// api/pkg/store/postgres.go - in AutoMigrate()

db.AutoMigrate(
    &SpecTaskDesignReviewComment{}, // Will add new columns automatically
)
```

**New Columns:**
- `agent_response` TEXT
- `agent_response_at` TIMESTAMP
- `interaction_id` VARCHAR(255)
- `resolution_reason` VARCHAR(100)

**Make Nullable:**
- `comment_type` VARCHAR(50) NULL (currently NOT NULL)

## Success Criteria

1. ✅ User can move/resize design review window while keeping Kanban visible
2. ✅ Comments have single type (no category selection)
3. ✅ Creating comment immediately sends prompt to agent
4. ✅ Agent response streams into comment in real-time
5. ✅ Agent can update design docs and push changes
6. ✅ Comments auto-resolve when quoted text removed
7. ✅ User can manually dismiss comments with X button
8. ✅ Comment log panel shows all comments with status
9. ✅ No regression in existing design review features

## Risks & Mitigations

### Risk 1: WebSocket Message Routing

**Risk:** Multiple interactions happening simultaneously, responses might route to wrong comments

**Mitigation:**
- Use interaction_id as unique key
- Backend validates comment exists before linking response
- Frontend refetches specific comment by ID

### Risk 2: Auto-Resolution False Positives

**Risk:** Agent might rephrase text, triggering incorrect auto-resolution

**Mitigation:**
- Use exact string matching (conservative)
- Only resolve if quoted_text completely absent
- Preserve resolved comments in log
- User can see why it was resolved

### Risk 3: Performance with Many Comments

**Risk:** Large comment threads could slow down UI

**Mitigation:**
- Virtualize comment list if >50 comments
- Lazy load agent responses
- Paginate comment log panel
- Cache document content

## Open Questions

1. **Comment Ordering**: Should comments be ordered by creation time or document position?
   - **Recommendation**: Document position (top-to-bottom of doc)

2. **Multiple Agents**: If user creates multiple planning sessions, which agent responds?
   - **Recommendation**: Use current spec_session_id from task

3. **Concurrent Comments**: What if user adds 5 comments quickly?
   - **Recommendation**: Queue agent prompts, respond to each sequentially

4. **Share Link**: Existing share functionality still works?
   - **Recommendation**: Yes, preserve existing share button feature

## Implementation Estimate

**Time Breakdown:**
- Phase 1 (Floating Window): 30 min
- Phase 2 (Simplify Types): 15 min
- Phase 3 (Agent Integration Backend): 45 min
- Phase 4 (WebSocket Streaming): 30 min
- Phase 5 (Auto-Resolution): 30 min
- Phase 6 (Manual Dismissal): 15 min
- Phase 7 (Comment Log): 30 min
- Testing & Debugging: 45 min

**Total:** ~4 hours (using LLMs reduces to ~1-2 hours in practice)

## References

- Existing Pattern: SpecTaskDetailDialog.tsx (draggable floating window)
- WebSocket Infrastructure: websocket_external_agent_sync.go
- Design Review Database: spec_task_design_review_store.go
- Comment UI: DesignReviewViewer.tsx (lines 135-200 handle text selection)
- Share Feature: Already implemented (preserve functionality)
