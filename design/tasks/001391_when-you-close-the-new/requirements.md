# Requirements: SpectTask Form Draft Persistence

## Problem Statement

When the user closes the "New Task" panel in the SpectTasks page, all form data is lost. If the user was composing a detailed task prompt, selecting an agent, or configuring branch settings, all that work disappears when they close the panel (even accidentally). This is frustrating and causes data loss.

## User Stories

### US-1: Draft Auto-Save
**As a** user composing a new spectask  
**I want** my in-progress form data to persist when I close the panel  
**So that** I don't lose my work if I close it accidentally or need to do something else

### US-2: Draft History / Previous Prompts
**As a** user who creates many spectasks  
**I want** access to my previous task prompts (both sent and drafts)  
**So that** I can reuse or refine similar prompts without retyping

### US-3: Multiple Drafts
**As a** user working on several ideas  
**I want** to save multiple draft prompts  
**So that** I can switch between different task ideas before committing

## Acceptance Criteria

### AC-1: Form State Persists on Close
- [ ] Closing the panel preserves: prompt text, selected agent, priority, dependencies, branch settings, just-do-it mode
- [ ] Reopening the panel restores all persisted state
- [ ] Persistence is scoped per-project (different projects = different drafts)

### AC-2: Draft Auto-Save
- [ ] Draft saves automatically on every change (debounced, like `RobustPromptInput`)
- [ ] Draft persists across browser refresh
- [ ] Draft persists across browser sessions (localStorage)

### AC-3: Prompt History Integration
- [ ] Successfully submitted prompts are saved to history
- [ ] User can browse previous prompts from the form
- [ ] User can select a previous prompt to populate the form
- [ ] History is scoped per-project

### AC-4: Clear Draft
- [ ] User can explicitly clear the current draft
- [ ] Successful task creation clears the draft (existing `resetForm` behavior)

## Out of Scope
- Backend sync of drafts (localStorage only for now)
- Sharing drafts between users
- Draft versioning / undo history