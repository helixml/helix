# Implementation Tasks

## Core Hook
- [ ] Create `frontend/src/hooks/useSpecTaskFormDraft.ts` hook
  - [ ] Define `SpecTaskFormDraft` interface with all form fields
  - [ ] Implement `loadDraft(projectId)` from localStorage
  - [ ] Implement `saveDraft(projectId, draft)` to localStorage (debounced)
  - [ ] Implement `clearDraft(projectId)` 
  - [ ] Add 7-day expiry check on load
  - [ ] Implement prompt history array storage (max 20 items)
  - [ ] Export `saveToPromptHistory(projectId, prompt)` function

## Integrate with NewSpecTaskForm
- [ ] Import `useSpecTaskFormDraft` in `NewSpecTaskForm.tsx`
- [ ] Initialize all form useState from loaded draft (taskPrompt, taskPriority, selectedHelixAgent, etc.)
- [ ] Add useEffect to call `saveDraft` on any form field change (debounced ~300ms)
- [ ] Call `clearDraft()` in `resetForm()` after successful task creation
- [ ] Call `saveToPromptHistory()` with the prompt after successful task creation

## Prompt History UI
- [ ] Add history icon button next to prompt textarea
- [ ] Create dropdown/menu showing previous prompts for this project
- [ ] Clicking a history item populates the prompt textarea
- [ ] Show truncated preview with timestamp in dropdown items

## Testing
- [ ] Verify draft persists when closing and reopening panel
- [ ] Verify draft persists across browser refresh
- [ ] Verify draft clears after successful task submission
- [ ] Verify prompt history shows previously submitted prompts
- [ ] Verify selecting from history populates form
- [ ] Verify drafts are scoped per-project (different projects don't share)