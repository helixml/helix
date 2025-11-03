# SpecTask Review Flow - Corrected Design

## Key Insight

**The planning session IS ALREADY a Helix chat session!**

Users don't need a special feedback interface - they just continue chatting in the existing session where the planning agent is working.

---

## Current Flow (What Exists)

```
1. User creates SpecTask from prompt
   ↓
2. SpecDrivenTaskService creates planning Helix session (task.SpecSessionID)
   ↓
3. Planning agent generates specs in that session
   ↓
4. Specs saved to database (RequirementsSpec, TechnicalDesign, ImplementationPlan)
   ↓
5. Task status → spec_review
   ↓
6. User goes to separate approval UI to review
   ↓
7. Binary approve/reject decision
```

**Problem**: User can't talk to planning agent during review!

---

## Enhanced Flow (What We Should Build)

```
1. User creates SpecTask from prompt
   ↓
2. SpecDrivenTaskService creates planning Helix session (task.SpecSessionID)
   ↓
3. Planning agent generates specs in that session
   ↓
4. Specs saved to database
   ↓
5. Task status → spec_review (but session stays OPEN)
   ↓
6. User has TWO options:

   Option A: View on phone (shareable link)
   ┌────────────────────────────────┐
   │ GET /spec-tasks/{id}/view      │
   │ Token-based, mobile-optimized  │
   │ Read-only HTML rendering       │
   └────────────────────────────────┘

   Option B: Interactive feedback (existing session UI)
   ┌────────────────────────────────┐
   │ Navigate to session page:      │
   │ /session/{task.SpecSessionID}  │
   │                                │
   │ User sees full chat history    │
   │ User can send messages         │
   │ Planning agent responds        │
   │ Agent updates specs            │
   │ Repeat until satisfied         │
   └────────────────────────────────┘

7. When satisfied, user clicks "Approve" button
   ↓
8. Task transitions to implementation
```

---

## What Needs to be Built

### 1. Shareable Design Doc Viewer (NEW)

**Endpoint**: `GET /api/v1/spec-tasks/{id}/design-docs/share`

**Purpose**: Generate shareable token

**Response**:
```json
{
  "share_url": "https://helix.example.com/spec-tasks/spec_123/view?token=eyJ...",
  "expires_at": "2025-10-15T10:00:00Z"
}
```

**Endpoint**: `GET /spec-tasks/{id}/view?token={jwt}`

**Purpose**: Public mobile-optimized viewer

**Features**:
- No login required (JWT validates access)
- Mobile-responsive HTML
- Markdown → HTML conversion
- Syntax highlighting
- Clean typography
- Tabs for Requirements / Design / Implementation Plan

### 2. Keep Planning Session Open (MODIFY EXISTING)

**Current Problem**: Planning session might close when specs are done

**Fix**: Planning session should stay open until specs approved

**Changes Needed**:
```go
// api/pkg/services/spec_driven_task_service.go

func (s *SpecDrivenTaskService) handleSpecGenerationComplete(task *types.SpecTask) {
    // DON'T close the planning session!
    // Just transition task to spec_review
    task.Status = types.TaskStatusSpecReview
    // Session stays active for user feedback
}
```

### 3. Link to Planning Session from Review UI (NEW FRONTEND)

**Component**: `SpecTaskReviewPanel`

**Shows**:
```
┌─────────────────────────────────────────┐
│ Design Documents Ready for Review       │
│                                         │
│ [📱 Get Shareable Link]                │ ← Mobile viewing
│                                         │
│ [💬 Continue Conversation with Agent]  │ ← Opens session page
│                                         │
│ [✅ Approve Specs]  [❌ Request Changes]│ ← Final decision
└─────────────────────────────────────────┘
```

The "Continue Conversation" button just navigates to `/session/{task.SpecSessionID}` - reuses existing session UI!

### 4. Approval Button in Session UI (ENHANCE EXISTING)

When viewing the planning session, if task is in spec_review, show approval buttons directly in the session UI.

---

## Simplified Implementation

### Backend

#### 1. Share Token Generation
```go
// api/pkg/server/spec_driven_task_handlers.go

func (s *HelixAPIServer) generateDesignDocsShareLink(w, r) {
    // Get task
    // Generate JWT with task_id, expires in 7 days
    // Return shareable URL
}
```

#### 2. Public Viewer
```go
func (s *HelixAPIServer) viewDesignDocsPublic(w, r) {
    // Validate JWT token
    // Get task from database
    // Convert markdown to HTML
    // Render mobile-optimized HTML template
}
```

#### 3. Keep Session Alive
```go
// api/pkg/services/spec_driven_task_service.go

// When transitioning to spec_review:
// - DON'T close planning session
// - User can continue chatting
// - Session only closes after approval
```

### Frontend

#### 1. Add "Continue Conversation" Button
```typescript
// In SpecTask review UI:
<Button
  startIcon={<ChatIcon />}
  onClick={() => router.navigate(`/session/${task.spec_session_id}`)}
>
  Continue Conversation with Planning Agent
</Button>
```

#### 2. Add Approval Buttons to Session Page
```typescript
// In Session.tsx, when session is a planning session in spec_review:
{isPlanningSessio && task?.status === 'spec_review' && (
  <Box sx={{ p: 2, bgcolor: 'info.light' }}>
    <Typography>Ready to approve these specs?</Typography>
    <Button onClick={approveSpecs}>Approve</Button>
    <Button onClick={requestChanges}>Request Changes</Button>
  </Box>
)}
```

---

## You're Absolutely Right!

Interactive feedback = just use the existing session UI!

**No need for a special feedback interface.**

Just need:
1. ✅ Shareable mobile link (NEW)
2. ✅ Button to navigate to planning session (SIMPLE)
3. ✅ Keep planning session open during review (MODIFY)
4. ✅ Approval buttons in session UI (ENHANCE)

Much simpler than I originally designed!

**Should I implement this corrected approach now?**
