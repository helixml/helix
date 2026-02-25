# Design: SpectTask Form Draft Persistence

## Overview

Implement draft auto-save for the `NewSpecTaskForm` component using the same patterns already established by `RobustPromptInput` and `usePromptHistory`. The form state will persist to localStorage, scoped by project ID.

## Architecture Decision

**Approach: Reuse `usePromptHistory` pattern with a new `useSpecTaskFormDraft` hook**

The codebase already has a proven pattern in `usePromptHistory`:
- Auto-save to localStorage on every change (debounced)
- Load draft on mount
- Clear draft on successful submit
- Scoped storage keys (`helix_prompt_draft_${sessionId}`)

We'll create a similar hook specifically for the spectask form draft.

## Key Components

### 1. `useSpecTaskFormDraft` Hook (New)

Location: `frontend/src/hooks/useSpecTaskFormDraft.ts`

```typescript
interface SpecTaskFormDraft {
  taskPrompt: string
  taskPriority: string
  selectedHelixAgent: string
  selectedDependencyTaskIds: string[]
  justDoItMode: boolean
  branchMode: TypesBranchMode
  baseBranch: string
  branchPrefix: string
  workingBranch: string
  showBranchCustomization: boolean
  timestamp: number
}

function useSpecTaskFormDraft(projectId: string): {
  draft: SpecTaskFormDraft | null
  saveDraft: (draft: Partial<SpecTaskFormDraft>) => void
  clearDraft: () => void
  promptHistory: string[]  // Previous submitted prompts for this project
  saveToPromptHistory: (prompt: string) => void
}
```

Storage key: `helix_spectask_draft_${projectId}`

### 2. Modify `NewSpecTaskForm`

- Import and use `useSpecTaskFormDraft(projectId)`
- Initialize form state from `draft` on mount
- Call `saveDraft()` when any form field changes (debounced)
- Call `clearDraft()` in `resetForm()` after successful submission
- Add UI for browsing/selecting from `promptHistory`

### 3. Prompt History Dropdown

Add a small history icon/dropdown near the prompt textarea (similar to `RobustPromptInput`'s history feature) that:
- Shows recent submitted prompts for this project
- Clicking a prompt populates the textarea
- Maximum 20 prompts stored per project

## Data Flow

```
User types → useState updates → debounced saveDraft() → localStorage
Panel closes → component unmounts (state lost, but localStorage persists)
Panel reopens → useSpecTaskFormDraft loads from localStorage → useState initialized
```

## Storage Schema

```typescript
// Key: helix_spectask_draft_${projectId}
{
  taskPrompt: "Build a feature that...",
  taskPriority: "high",
  selectedHelixAgent: "app_123",
  selectedDependencyTaskIds: ["task_456"],
  justDoItMode: false,
  branchMode: "new",
  baseBranch: "main",
  branchPrefix: "",
  workingBranch: "",
  showBranchCustomization: false,
  timestamp: 1704067200000
}

// Key: helix_spectask_prompts_${projectId}
["Previous prompt 1", "Previous prompt 2", ...]
```

## Draft Expiry

Drafts older than 7 days are discarded on load (same pattern as `usePromptHistory` which uses 24 hours for session drafts, but task drafts are more valuable so longer retention).

## UI Changes

1. **History dropdown** - Small history icon button next to the prompt textarea
2. **"Clear draft" button** - Optional, in the form footer near Cancel
3. **Visual indicator** - Subtle "Draft saved" text after auto-save (optional, low priority)

## Patterns to Follow

From `usePromptHistory.ts`:
- `loadDraft()` / `saveDraft()` functions with try/catch
- Debouncing with `useRef` for the save timeout
- Storage key generation functions
- Timestamp-based expiry checks