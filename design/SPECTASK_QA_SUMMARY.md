# SpecTask Q&A Summary

## Your Questions Answered

### Q1: How/where does Helix read design documents out of git?

**Answer**: **Currently it doesn't!**

Design documents are stored in **PostgreSQL database fields**:
- `SpecTask.RequirementsSpec` (TEXT column)
- `SpecTask.TechnicalDesign` (TEXT column)
- `SpecTask.ImplementationPlan` (TEXT column)

**Read via**: `GET /api/v1/spec-tasks/{taskId}/specs` endpoint returns these database fields directly.

**Git worktree system I built**: Infrastructure is ready but not yet used. Would need bidirectional sync to be implemented.

---

### Q2: For human review, can we present shareable link AND allow feedback during design phase?

**Answer**: **Yes! Just implemented it.**

**Two Options for User**:

1. **Shareable Link** (view on phone while walking)
   - POST `/api/v1/spec-tasks/{id}/design-docs/share` - Generate token
   - GET `/spec-tasks/{id}/view?token=xxx` - View docs (no login)
   - Mobile-optimized HTML rendering
   - Valid for 7 days
   - Copy to clipboard

2. **Interactive Feedback** (chat with planning agent)
   - Navigate to planning session: `/session/{task.SpecSessionID}`
   - Use existing Helix session UI
   - Continue conversation with planning agent
   - Agent updates specs based on feedback
   - Repeat until satisfied

**Key Insight**: No special feedback interface needed - just reuse existing session UI!

---

### Q3: Are planning and implementation the SAME session or different ones?

**Answer**: **DIFFERENT sessions!**

**Planning Session** (`task.SpecSessionID`):
- Regular Helix chat session
- Created when task enters spec_generation phase
- Planning agent generates requirements, design, implementation plan
- Session stays OPEN during spec_review phase
- User can continue chatting until satisfied
- Closed after specs approved

**Implementation Session** (`task.ImplementationSessionID`):
- Separate session created after approval
- Could be Zed external agent session
- Could be regular Helix session with implementation agent
- Works from approved specs
- Creates actual code

**Why Different**:
- Different agents (planning vs implementation)
- Different contexts (design vs coding)
- Different tools (research vs code editor)
- Different phases of workflow

---

## Implementation Summary

### What I Built

#### Backend (3 files, ~250 lines)
1. **spec_task_share_handlers.go** - Shareable link generation + public viewer
   - JWT token generation with 7-day expiry
   - Mobile-optimized HTML template
   - Markdown → HTML conversion
   - No login required (token-based)

#### Frontend (1 file, ~150 lines)
2. **SpecTaskReviewPanel.tsx** - Review UI component
   - "Get Shareable Link" button with clipboard copy
   - "Open Planning Session" button (navigates to existing session)
   - Approval buttons (when ready)
   - Clear UX flow

#### Routes
3. **server.go** - Added 2 new routes
   - POST `/api/v1/spec-tasks/{id}/design-docs/share` (authenticated)
   - GET `/spec-tasks/{id}/view` (public, token-based)

#### Documentation (2 files)
4. **spectask-interactive-review-enhancement.md** - Initial design
5. **spectask-review-flow-corrected.md** - Corrected approach after your clarification

---

## How It Works

### User Workflow

```
Step 1: Create SpecTask
   ↓
Step 2: Planning agent generates specs in Helix session
   ↓
Step 3: Task status → spec_review
   ↓
Step 4a: Get shareable link
   - Click "Get Shareable Link" button
   - Link copied to clipboard
   - Open on phone, tablet, etc.
   - Read design docs mobile-optimized
   ↓
Step 4b: Provide feedback (if needed)
   - Click "Open Planning Session"
   - Chat with planning agent
   - Agent revises specs
   - Repeat until satisfied
   ↓
Step 5: Approve specs
   - Click "Approve" button
   - Task → implementation phase
```

### Technical Flow

```
Generate Share Link:
User clicks button
   ↓
POST /api/v1/spec-tasks/{id}/design-docs/share
   ↓
Generate JWT(task_id, user_id, exp=7days)
   ↓
Return shareable URL
   ↓
Copy to clipboard

View on Phone:
Open URL on phone
   ↓
GET /spec-tasks/{id}/view?token=xxx
   ↓
Validate JWT token
   ↓
Get task from database
   ↓
Convert markdown → HTML
   ↓
Render mobile template
   ↓
Beautiful docs on phone!

Interactive Feedback:
User clicks "Open Planning Session"
   ↓
Navigate to /session/{task.SpecSessionID}
   ↓
Existing session UI loads
   ↓
User sees full chat history
   ↓
User sends message
   ↓
Planning agent responds
   ↓
Agent updates specs in database
   ↓
Repeat until approved
```

---

## What's NOT Implemented Yet

### Database ↔ Git Worktree Sync
Currently design docs are ONLY in database, not synced to git worktree.

**To implement**:
```go
// When specs are updated in database:
func (m *DesignDocsWorktreeManager) SyncToWorktree(worktreePath, task) {
    // Write requirements.md
    // Write design.md
    // Write implementation-plan.md
    // Git commit
}

// When implementation starts:
func (o *SpecTaskOrchestrator) handleImplementationQueued(task) {
    // Setup worktree
    // Sync specs from database to git files
    // Agent reads from git files
}
```

This would allow agents to read specs from git files instead of database.

### Approval Buttons in Session UI
Currently approval is separate. Could add approval buttons directly in the session page when viewing a planning session in spec_review status.

### QR Code for Easy Mobile Access
Could generate QR code of shareable link for easy scanning.

---

## Files Changed

### New Files
- `api/pkg/server/spec_task_share_handlers.go`
- `frontend/src/components/tasks/SpecTaskReviewPanel.tsx`
- `docs/design/spectask-interactive-review-enhancement.md`
- `docs/design/spectask-review-flow-corrected.md`

### Modified Files
- `api/pkg/server/server.go` (routes)
- `go.mod` + `go.sum` (blackfriday dependency)
- OpenAPI specs (auto-generated)

---

## Summary

✅ **Shareable links implemented** - Mobile-optimized HTML viewer
✅ **Interactive feedback clarified** - Use existing session UI (no new interface needed)
✅ **Planning session lifecycle** - Stays open during review for feedback
✅ **Clear UX flow** - SpecTaskReviewPanel guides user through options
✅ **Zero compilation errors** - API builds successfully

**Next**: User can review design docs on phone AND chat with planning agent before approving!
