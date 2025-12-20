# Prompt Library and Unified Search

## Overview

This document covers two interconnected features:
1. **Prompt Library** - A robust prompt editing, queueing, and reuse system for offline-first experience
2. **Unified Search** - Search across all prompts and sessions in all projects

## Prompt Library

### Goals
- Perfect prompt editing experience for intermittent connectivity (train with 10-20% internet)
- Backend-driven queue processing (not frontend)
- Prompt reuse and templates
- Cross-device sync

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│ Frontend (RobustPromptInput)                                    │
│  - Draft auto-save (localStorage)                               │
│  - Message queue UI with drag-and-drop reordering               │
│  - History navigation (↑/↓ keys)                                │
│  - Sync to backend on connectivity                              │
│  - NO sending logic when backend queue enabled                  │
└───────────────────────┬─────────────────────────────────────────┘
                        │ POST /api/v1/prompt-history/sync
                        ▼
┌─────────────────────────────────────────────────────────────────┐
│ Backend (HelixAPIServer)                                        │
│  - Stores prompt history per user/spec_task                     │
│  - Interrupt prompts: sent immediately on sync                  │
│  - Non-interrupt: queued until message_completed                │
│  - processPromptQueue() called after message_completed          │
└───────────────────────┬─────────────────────────────────────────┘
                        │ WebSocket
                        ▼
┌─────────────────────────────────────────────────────────────────┐
│ External Agent (Zed + Qwen Code)                                │
│  - Receives chat_message commands                               │
│  - Sends message_completed when done                            │
└─────────────────────────────────────────────────────────────────┘
```

### PromptHistoryEntry Schema

```go
type PromptHistoryEntry struct {
    ID         string `gorm:"primaryKey"`
    UserID     string `gorm:"index"`
    ProjectID  string `gorm:"index"`
    SpecTaskID string `gorm:"index"`
    SessionID  string `gorm:"index"` // Which session this was sent to

    Content    string // The prompt content
    Status     string // "pending", "sent", "failed"
    Interrupt  bool   // If true, interrupts current conversation
    QueuePosition *int // For drag-and-drop ordering

    // Library features
    Pinned     bool      // User pinned this prompt
    UsageCount int       // How many times reused
    LastUsedAt *time.Time // Last time reused
    Tags       []string  // User-defined tags (stored as JSON)
    IsTemplate bool      // Saved as a reusable template

    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

### UI Components

1. **RobustPromptInput** (existing, enhanced)
   - Message queue with drag-and-drop
   - Interrupt mode toggle (⚡ vs 📋)
   - Sync status indicators (✓ = local, ✓✓ = synced)
   - History navigation with ↑/↓

2. **Enhanced History Dropdown** (new)
   - Pinned prompts section at top
   - Search/filter prompts
   - Pin/unpin toggle
   - Frequency sort option

3. **PromptLibrarySidebar** (new)
   - Full prompt library panel
   - Search and filter
   - Tag management
   - Template management

4. **PromptViewerModal** (new)
   - View full prompt content
   - Edit and resend
   - Pin/unpin, tag management

### Keyboard Shortcuts
- `Cmd+/` - Open prompt library sidebar
- `Cmd+P` - Quick search prompts
- `↑/↓` in empty input - Navigate history

## Unified Search

### Goals
- Search across all prompts and sessions in all projects
- Rich search results with context
- Link prompts to the sessions they were sent in

### Architecture Options

#### Option 1: Postgres Full-Text Search
- Use `tsvector` and `tsquery` for text search
- Simple, no additional infrastructure
- Good for moderate data sizes

#### Option 2: RAG with Embeddings
- Use vector embeddings for semantic search
- Better for "find similar prompts" use cases
- Requires embedding model integration

**Recommendation**: Start with Postgres FTS, add RAG later for semantic features.

### Search Schema

```sql
-- Add full-text search columns
ALTER TABLE prompt_history_entry
ADD COLUMN content_tsvector tsvector
GENERATED ALWAYS AS (to_tsvector('english', content)) STORED;

CREATE INDEX idx_prompt_history_fts ON prompt_history_entry USING GIN(content_tsvector);

-- Sessions already have name and can search interactions
ALTER TABLE session
ADD COLUMN name_tsvector tsvector
GENERATED ALWAYS AS (to_tsvector('english', COALESCE(name, ''))) STORED;

-- Interactions contain the actual conversation
ALTER TABLE interaction
ADD COLUMN prompt_tsvector tsvector
GENERATED ALWAYS AS (to_tsvector('english', COALESCE(prompt_message, '') || ' ' || COALESCE(response_message, ''))) STORED;
```

### Prompt-Session Links

Prompts are already linked to sessions via `session_id` field. When searching:

1. Search prompts → show which session(s) they were used in
2. Search sessions → show prompts that were sent to them
3. Search interactions → find by content in either prompt or response

### Search UI

**Giant Search Bar** on Project List page:
- Searches across all spec tasks in all projects
- Rich results showing:
  - Matching prompts (with session context)
  - Matching sessions (with interaction previews)
  - Matching spec tasks (by title, description)

### API Endpoints

```
GET /api/v1/search
  ?q=<query>
  &type=prompts,sessions,tasks  # Filter by type
  &project_id=<optional>        # Scope to project
  &limit=50
```

Response:
```json
{
  "prompts": [
    {
      "id": "...",
      "content": "...",
      "highlight": "...matching **text**...",
      "session_id": "ses_...",
      "session_name": "Planning Session",
      "spec_task_id": "...",
      "spec_task_title": "Implement feature X"
    }
  ],
  "sessions": [...],
  "tasks": [...]
}
```

## Implementation Plan

### Phase 1: Prompt Library (Current)
1. ✅ Backend-driven queue processing
2. ✅ Interrupt vs non-interrupt modes
3. ✅ Sync status indicators
4. ✅ Drag-and-drop reordering
5. 🔄 Add pinned, usageCount, lastUsedAt, tags, isTemplate fields
6. ⏳ Pin/unpin API and UI
7. ⏳ Enhanced history dropdown with pinning and search
8. ⏳ PromptLibrarySidebar component
9. ⏳ PromptViewerModal component
10. ⏳ Keyboard shortcuts

### Phase 2: Unified Search
11. ⏳ Add Postgres FTS indexes
12. ⏳ Search API endpoint
13. ⏳ Giant search bar on project list
14. ⏳ Rich search results with context
15. ⏳ Prompt-session cross-references in UI

### Phase 3: Advanced Features (Future)
- RAG-based semantic search
- "Find similar prompts" feature
- Auto-tagging with AI
- Prompt effectiveness analytics
