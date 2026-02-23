# Design: Image Attachments for SpecTask Creation

## Architecture Overview

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│ NewSpecTaskForm │────▶│   Filestore     │────▶│    SpecTask     │
│ (drag & drop)   │     │ (temp storage)  │     │  (references)   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                                        ▼
                              ┌─────────────────────────────────────┐
                              │          Session Start              │
                              │  (copies to sandbox filesystem)     │
                              └─────────────────────────────────────┘
                                                        │
                                                        ▼
                              ┌─────────────────────────────────────┐
                              │   Agent Prompt includes paths to    │
                              │   /home/retro/work/attachments/     │
                              └─────────────────────────────────────┘
```

## Key Decisions

### Decision 1: Where to Store Images

**Chosen:** Use existing Helix Filestore (`apps/{app_id}/task-attachments/{task_id}/`)

**Rationale:** 
- Filestore already exists with upload/list/delete APIs
- Consistent with knowledge file storage pattern
- No new infrastructure needed

### Decision 2: When to Copy to Sandbox

**Chosen:** Copy images when sandbox session starts (in `StartSpecGeneration` or `StartJustDoIt`)

**Rationale:**
- Single copy operation per session
- Images available immediately when agent starts
- Reuses existing file transfer pattern (similar to how design docs are copied)

### Decision 3: Sandbox Path for Attachments

**Chosen:** `/home/retro/work/attachments/{task_id}/`

**Rationale:**
- Separate from `incoming/` (ad-hoc uploads) and `helix-specs/` (design docs)
- Clear naming convention
- Task-scoped directory prevents collision

## Component Changes

### 1. Frontend: `NewSpecTaskForm.tsx`

Add image attachment zone using existing `FileUpload` component pattern:
- Accept drag & drop and click-to-browse
- Upload immediately to filestore via `FilestoreUpload` API
- Store file paths in component state
- Include paths in `CreateTaskRequest`

### 2. API Types: `simple_spec_task.go`

Add `Attachments` field to `CreateTaskRequest` and `SpecTask`:
```go
type CreateTaskRequest struct {
    // ... existing fields ...
    Attachments []string `json:"attachments,omitempty"` // Filestore paths
}

type SpecTask struct {
    // ... existing fields ...
    Attachments []string `json:"attachments,omitempty" gorm:"type:json"` // JSON array
}
```

### 3. Task Service: `spec_driven_task_service.go`

In `StartSpecGeneration` and `StartJustDoItMode`:
- Read attachment paths from task
- Copy files from filestore to sandbox via existing file transfer mechanism
- Include attachment info in agent system prompt

### 4. Agent Instructions: `agent_instruction_service.go`

Add attachments section to implementation/planning prompts:
```
## Attached Images

The user attached the following images for reference:
- /home/retro/work/attachments/{task_id}/screenshot1.png
- /home/retro/work/attachments/{task_id}/diagram.jpg

Use Zed's image viewing capability or describe what you see.
```

## Existing Patterns Found

- **File Upload:** `uploadFileToSandbox()` in `external_agent_handlers.go` uploads to `/home/retro/work/incoming/`
- **Filestore API:** `FilestoreUpload`, `FilestoreList`, `FilestoreDelete` in client
- **Drag & Drop UI:** `FileUpload` component used in `KnowledgeEditor.tsx`
- **JSON arrays in GORM:** `DependsOn` field in SpecTask uses `gorm:"type:json"`
