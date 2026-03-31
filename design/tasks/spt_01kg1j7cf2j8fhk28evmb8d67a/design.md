# Design: Persist New SpecTask Draft in localStorage

## Key Files

- **Form:** `frontend/src/components/tasks/NewSpecTaskForm.tsx`
  - `taskPrompt` state (line 91): `const [taskPrompt, setTaskPrompt] = useState("")`
  - `resetForm()` (lines 257-276): clears form state on cancel/submit
  - TextField onChange (line 516): fires on every keystroke
- **Inspiration:** `frontend/src/hooks/usePromptHistory.ts`
  - `DRAFT_STORAGE_KEY = 'helix_prompt_draft'` (line 27)
  - `loadDraft(sessionId)` (lines 138-152): reads with 24-hour TTL check
  - `saveDraft(sessionId, content)` (lines 154-165): debounced 300ms write
  - `clearDraftStorage(sessionId)` (lines 167-173): deletes on send

## Approach

Add localStorage draft persistence **directly inside `NewSpecTaskForm.tsx`** — no new hook needed, just mirror the pattern from `usePromptHistory`.

### Storage key

```
helix_new_spectask_draft_{projectId}
```

`projectId` is already a required prop on `NewSpecTaskForm`, so this is always available. One draft per project, no cross-contamination.

### Draft shape (same as usePromptHistory)

```json
{ "content": "...", "timestamp": 1234567890 }
```

### Implementation sketch

```typescript
const DRAFT_KEY = `helix_new_spectask_draft_${projectId}`
const DRAFT_TTL = 24 * 60 * 60 * 1000 // 24 hours

// On mount — load draft
const [taskPrompt, setTaskPrompt] = useState<string>(() => {
  try {
    const raw = localStorage.getItem(DRAFT_KEY)
    if (!raw) return ""
    const { content, timestamp } = JSON.parse(raw)
    if (Date.now() - timestamp > DRAFT_TTL) { localStorage.removeItem(DRAFT_KEY); return "" }
    return content || ""
  } catch { return "" }
})

// On change — debounced save (useRef for timer)
const draftTimer = useRef<ReturnType<typeof setTimeout>>()
const handlePromptChange = (value: string) => {
  setTaskPrompt(value)
  clearTimeout(draftTimer.current)
  draftTimer.current = setTimeout(() => {
    if (value) localStorage.setItem(DRAFT_KEY, JSON.stringify({ content: value, timestamp: Date.now() }))
    else localStorage.removeItem(DRAFT_KEY)
  }, 300)
}

// On successful submit — clear draft
localStorage.removeItem(DRAFT_KEY)
```

### Cancel behaviour

On cancel (`resetForm` + `onClose`): **clear the draft**. Rationale: the user explicitly dismissed the form, so the draft should not surprise them next time. This diverges slightly from the prompt editor (which clears on send, not cancel) but matches user expectation for a form that was intentionally closed.

If the user re-opens to a blank form after cancel, that is correct behaviour.

## Decision: Why not reuse `usePromptHistory`?

`usePromptHistory` is tied to a `sessionId` (an existing spectask's conversation). For a *new* task that hasn't been created yet, there is no session. Using the hook would require significant refactoring. A self-contained inline implementation is simpler and keeps the form independent.

## Codebase Notes

- `NewSpecTaskForm` already persists labels to `helix_last_task_labels` using a simple `localStorage.setItem`/`getItem` pattern (lines 95, 334) — this draft feature follows the exact same style.
- The debounce timer ref must be cleaned up in a `useEffect` return to avoid writes after unmount.
