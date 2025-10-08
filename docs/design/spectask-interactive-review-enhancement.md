# SpecTask Interactive Review Enhancement

## Current State

**Design Document Storage**: Currently stored in database fields
- `SpecTask.RequirementsSpec` (text field)
- `SpecTask.TechnicalDesign` (text field)
- `SpecTask.ImplementationPlan` (text field)

**Review Process**: Static approval endpoint
- GET `/api/v1/spec-tasks/{id}/specs` - Returns markdown documents
- POST `/api/v1/spec-tasks/{id}/approve-specs` - Binary approve/reject

**Problems**:
1. No easy way to share design docs (e.g., view on phone)
2. No interactive feedback during design phase
3. No conversation thread with planning agent
4. Specs are static once generated

---

## Enhanced Design

### 1. Shareable Design Document Links

**Goal**: User can view design docs on phone while walking/commuting

**Implementation**:
```
GET /api/v1/spec-tasks/{id}/design-docs/view?token={share_token}

Returns HTML page with:
- Clean markdown rendering
- Mobile-responsive design
- No login required (token-based access)
- Shareable URL for any device
```

**Example URL**:
```
https://helix.example.com/api/v1/spec-tasks/spec_abc123/design-docs/view?token=eyJhbGc...
```

### 2. Interactive Review During Design Phase

**Goal**: User provides feedback to planning agent while it's designing

**Architecture**:
```
Planning Agent (Helix Session ses_planning_123)
    ↓
Generates specs iteratively
    ↓
User sees specs in real-time
    ↓
User sends feedback message
    ↓
Feedback goes to SAME planning session
    ↓
Agent revises specs based on feedback
    ↓
Continues until user approves
```

**Key Insight**: Reuse the existing planning Helix session for interactive feedback!

### 3. Dual Storage Strategy

**Database Fields** (current): For API access and metadata
- `RequirementsSpec`
- `TechnicalDesign`
- `ImplementationPlan`

**Git Worktree** (new): For agent access and version control
- `.git-worktrees/helix-design-docs/requirements.md`
- `.git-worktrees/helix-design-docs/design.md`
- `.git-worktrees/helix-design-docs/implementation-plan.md`

**Sync Strategy**: Write to both
1. Planning agent generates specs → Save to database
2. On save → Also write to git worktree files
3. On git commit → Update database fields
4. Keep both in sync bidirectionally

---

## Implementation Plan

### Backend Changes

#### 1. Add Share Token Generation

```go
// api/pkg/server/spec_driven_task_handlers.go

// @Summary Get shareable design docs link
// @Router /api/v1/spec-tasks/{taskId}/share-link [post]
func (s *HelixAPIServer) getDesignDocsShareLink(w http.ResponseWriter, r *http.Request) {
    // Generate JWT token with task_id claim
    // Token valid for 7 days
    // Return shareable URL
}
```

#### 2. Add Public Design Docs Viewer

```go
// api/pkg/server/spec_driven_task_handlers.go

// @Summary View design docs (public, token-based)
// @Router /api/v1/spec-tasks/{taskId}/design-docs/view [get]
func (s *HelixAPIServer) viewDesignDocsPublic(w http.ResponseWriter, r *http.Request) {
    // Validate share token
    // Get task specs from database
    // Render HTML with:
    //   - Mobile-responsive CSS
    //   - Markdown → HTML conversion
    //   - Syntax highlighting for code
    //   - Clean typography
}
```

#### 3. Add Interactive Feedback Endpoint

```go
// api/pkg/server/spec_driven_task_handlers.go

// @Summary Send feedback to planning agent
// @Router /api/v1/spec-tasks/{taskId}/feedback [post]
func (s *HelixAPIServer) sendPlanningFeedback(w http.ResponseWriter, r *http.Request) {
    // Get SpecTask
    // Check status is spec_generation or spec_review
    // Get planning session (task.SpecSessionID)
    // Create new interaction in that session with user feedback
    // Agent continues working on specs with feedback
}
```

#### 4. Enhance SpecDrivenTaskService

```go
// api/pkg/services/spec_driven_task_service.go

// SendFeedbackToPlanningAgent sends user feedback to active planning session
func (s *SpecDrivenTaskService) SendFeedbackToPlanningAgent(
    ctx context.Context,
    taskID string,
    feedback string,
    userID string,
) error {
    // Get task
    task, err := s.store.GetSpecTask(ctx, taskID)

    // Verify task is in design phase
    if task.Status != types.TaskStatusSpecGeneration &&
       task.Status != types.TaskStatusSpecReview {
        return fmt.Errorf("task not in design phase")
    }

    // Get planning session
    session, err := s.store.GetSession(ctx, task.SpecSessionID)

    // Create new interaction with user feedback
    // This reuses the existing session!
    interaction := &types.Interaction{
        SessionID: session.ID,
        Creator:   types.CreatorTypeUser,
        Message:   feedback,
        State:     types.InteractionStateWaiting,
    }

    // Controller processes the interaction
    // Planning agent receives feedback and continues designing
    return s.controller.AddInteraction(ctx, interaction)
}
```

#### 5. Sync Design Docs to Git Worktree

```go
// api/pkg/services/design_docs_worktree_manager.go

// SyncDesignDocsToWorktree writes database specs to git worktree files
func (m *DesignDocsWorktreeManager) SyncDesignDocsToWorktree(
    worktreePath string,
    task *types.SpecTask,
) error {
    // Write requirements.md
    err := os.WriteFile(
        filepath.Join(worktreePath, "requirements.md"),
        []byte(task.RequirementsSpec),
        0644,
    )

    // Write design.md
    err = os.WriteFile(
        filepath.Join(worktreePath, "design.md"),
        []byte(task.TechnicalDesign),
        0644,
    )

    // Write implementation-plan.md
    err = os.WriteFile(
        filepath.Join(worktreePath, "implementation-plan.md"),
        []byte(task.ImplementationPlan),
        0644,
    )

    // Commit changes
    return m.commitChanges(worktreePath, ".", "Update design docs from database")
}
```

### Frontend Changes

#### 1. Shareable Link Button

```typescript
// frontend/src/components/tasks/SpecTaskReviewCard.tsx (new)

const SpecTaskReviewCard: FC<{task: SpecTask}> = ({ task }) => {
  const [shareLink, setShareLink] = useState<string | null>(null)

  const generateShareLink = async () => {
    const response = await api.post(`/api/v1/spec-tasks/${task.id}/share-link`)
    setShareLink(response.share_url)

    // Copy to clipboard
    navigator.clipboard.writeText(response.share_url)
    snackbar.success('Link copied to clipboard!')
  }

  return (
    <Card>
      <CardHeader title="Design Documents Ready for Review" />
      <CardContent>
        <Button
          startIcon={<ShareIcon />}
          onClick={generateShareLink}
        >
          Get Shareable Link
        </Button>

        {shareLink && (
          <Alert severity="info" sx={{ mt: 2 }}>
            <Typography variant="body2">
              Share this link to review on any device:
            </Typography>
            <Typography variant="code" sx={{ wordBreak: 'break-all' }}>
              {shareLink}
            </Typography>
          </Alert>
        )}
      </CardContent>
    </Card>
  )
}
```

#### 2. Interactive Feedback Interface

```typescript
// frontend/src/components/tasks/SpecTaskFeedbackPanel.tsx (new)

const SpecTaskFeedbackPanel: FC<{task: SpecTask}> = ({ task }) => {
  const [feedback, setFeedback] = useState('')
  const [sending, setSending] = useState(false)

  const sendFeedback = async () => {
    setSending(true)
    try {
      await api.post(`/api/v1/spec-tasks/${task.id}/feedback`, {
        feedback: feedback
      })

      setFeedback('')
      snackbar.success('Feedback sent to planning agent!')
    } catch (err) {
      snackbar.error('Failed to send feedback')
    } finally {
      setSending(false)
    }
  }

  return (
    <Card>
      <CardHeader
        title="Provide Feedback to Planning Agent"
        subheader="Agent is actively designing - your feedback helps refine the specs"
      />
      <CardContent>
        <TextField
          multiline
          rows={4}
          fullWidth
          value={feedback}
          onChange={(e) => setFeedback(e.target.value)}
          placeholder="E.g., 'Also add password reset functionality' or 'Use PostgreSQL instead of MongoDB'"
        />

        <Button
          variant="contained"
          onClick={sendFeedback}
          disabled={!feedback.trim() || sending}
          sx={{ mt: 2 }}
        >
          Send Feedback to Agent
        </Button>

        <Alert severity="info" sx={{ mt: 2 }}>
          The planning agent will incorporate your feedback and update the design.
          You can provide feedback multiple times during the design phase.
        </Alert>
      </CardContent>
    </Card>
  )
}
```

#### 3. Design Review Page with Both Features

```typescript
// frontend/src/pages/SpecTaskReview.tsx (new)

const SpecTaskReview: FC = () => {
  const { taskId } = useParams()
  const [task, setTask] = useState<SpecTask | null>(null)

  return (
    <Container maxWidth="lg">
      <Grid container spacing={3}>
        {/* Left: Design docs preview */}
        <Grid item xs={12} md={8}>
          <Card>
            <CardHeader title="Design Documents" />
            <CardContent>
              <Tabs>
                <Tab label="Requirements" />
                <Tab label="Technical Design" />
                <Tab label="Implementation Plan" />
              </Tabs>

              {/* Markdown preview of specs */}
              <ReactMarkdown>{task.requirements_spec}</ReactMarkdown>
            </CardContent>
          </Card>
        </Grid>

        {/* Right: Interactive feedback + share */}
        <Grid item xs={12} md={4}>
          <Stack spacing={2}>
            {/* Share link */}
            <SpecTaskReviewCard task={task} />

            {/* Interactive feedback */}
            {task.status === 'spec_generation' && (
              <SpecTaskFeedbackPanel task={task} />
            )}

            {/* Approval buttons */}
            {task.status === 'spec_review' && (
              <SpecTaskApprovalPanel task={task} />
            )}
          </Stack>
        </Grid>
      </Grid>
    </Container>
  )
}
```

### Mobile-Optimized HTML Viewer

```go
// api/pkg/server/spec_driven_task_handlers.go

const designDocsViewerHTML = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Design Document - {{.TaskName}}</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
            line-height: 1.6;
            max-width: 800px;
            margin: 0 auto;
            padding: 20px;
            background: #f5f5f5;
        }
        .container {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
        }
        h1, h2, h3 { color: #333; }
        code {
            background: #f4f4f4;
            padding: 2px 6px;
            border-radius: 3px;
            font-family: 'Courier New', monospace;
        }
        pre {
            background: #f4f4f4;
            padding: 15px;
            border-radius: 5px;
            overflow-x: auto;
        }
        .section {
            margin-bottom: 40px;
        }
        .badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: bold;
            background: #e3f2fd;
            color: #1976d2;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>{{.TaskName}}</h1>
        <p><span class="badge">{{.Status}}</span></p>

        <div class="section">
            <h2>Requirements</h2>
            {{.RequirementsHTML}}
        </div>

        <div class="section">
            <h2>Technical Design</h2>
            {{.TechnicalDesignHTML}}
        </div>

        <div class="section">
            <h2>Implementation Plan</h2>
            {{.ImplementationPlanHTML}}
        </div>

        <p style="color: #666; font-size: 14px; margin-top: 60px;">
            Generated by Helix AI • Last updated: {{.UpdatedAt}}
        </p>
    </div>
</body>
</html>
`

func (s *HelixAPIServer) viewDesignDocsPublic(w http.ResponseWriter, r *http.Request) {
    // Validate share token
    // Get task
    // Convert markdown to HTML
    // Render template
    // Mobile-optimized, no login required
}
```

---

## Answer to Your Questions

### Q1: How/where does Helix read design documents out of git?

**Current Answer**: It doesn't!

Design documents are currently stored **in the database** (`SpecTask.RequirementsSpec`, `TechnicalDesign`, `ImplementationPlan` text fields), not read from git.

**What I Built**:
- I created the git worktree infrastructure (DesignDocsWorktreeManager)
- But the current flow is: Database → Git (one-way sync for agent access)
- Not: Git → Database (which you're asking about)

**To Fix**: Need to implement `SyncDesignDocsToWorktree()` method that:
1. Takes database specs
2. Writes to git worktree files
3. Commits to helix-design-docs branch
4. Agents then read from worktree

### Q2: Can we present shareable link AND allow feedback during design phase?

**Answer**: Yes! Here's how:

**Shareable Link** (view on phone):
- Generate token-based URL
- No login required
- Mobile-optimized HTML rendering
- User can review anywhere

**Interactive Feedback** (talk to planning agent):
- User sends feedback message
- Goes to SAME Helix session planning agent is using (task.SpecSessionID)
- Agent sees feedback as new interaction
- Agent revises specs based on feedback
- Continues conversation until user approves

**UI Flow**:
```
┌────────────────────────────────────┐
│ Design Documents Ready             │
│                                    │
│ [Get Shareable Link] 📱           │ ← View on phone
│                                    │
│ ┌────────────────────────────────┐ │
│ │ Feedback to Planning Agent     │ │
│ │                                │ │
│ │ [Text area for feedback]       │ │ ← Interactive feedback
│ │                                │ │
│ │ [Send to Agent] 💬            │ │
│ └────────────────────────────────┘ │
│                                    │
│ When satisfied:                    │
│ [Approve] [Request Changes]        │
└────────────────────────────────────┘
```

**Key Insight**: Planning session stays open during spec_generation AND spec_review phases. User can keep chatting with planning agent until they're happy, THEN formally approve.

---

## Implementation Needed

To fully implement this enhancement:

### Backend
- [ ] Add JWT-based share token generation
- [ ] Add public design docs viewer endpoint (HTML template)
- [ ] Add interactive feedback endpoint
- [ ] Enhance SpecDrivenTaskService.SendFeedbackToPlanningAgent
- [ ] Add SyncDesignDocsToWorktree bidirectional sync
- [ ] Keep planning session alive during review

### Frontend
- [ ] SpecTaskReviewCard with share link button
- [ ] SpecTaskFeedbackPanel for interactive feedback
- [ ] SpecTaskReview page combining both features
- [ ] Add to router and navigation
- [ ] Copy to clipboard functionality
- [ ] QR code generation (optional, for easy phone access)

### Integration
- [ ] Planning session lifecycle management
- [ ] Database ↔ Git worktree sync
- [ ] Token validation and expiry
- [ ] Markdown → HTML rendering
- [ ] Mobile-responsive CSS

---

## Benefits

✅ **Review on Phone**: Shareable link works on any device
✅ **Interactive Design**: Conversation with planning agent during design
✅ **Iterative Refinement**: Multiple rounds of feedback before approval
✅ **Better Specs**: User involvement improves design quality
✅ **Faster Approval**: Address concerns during design, not after
✅ **Mobile-First**: Engineers can review while mobile
✅ **No Friction**: Token-based, no login required for viewing

---

## Next Steps

Would you like me to implement these enhancements now?

1. Shareable design doc links (token-based, mobile-optimized)
2. Interactive feedback to planning agent (reuses planning session)
3. Bidirectional database ↔ git sync
4. Complete review UI with both features
