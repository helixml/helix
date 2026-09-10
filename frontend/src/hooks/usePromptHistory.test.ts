import { beforeEach, describe, it, expect } from 'vitest'
import { appendPromptDraft, reconcileEntry, type PromptHistoryEntry } from './usePromptHistory'

const make = (over: Partial<PromptHistoryEntry> = {}): PromptHistoryEntry => ({
  id: 'p1',
  content: 'hello',
  timestamp: 1,
  status: 'pending',
  ...over,
})

describe('reconcileEntry — the local↔backend dirty-flag invariant', () => {
  it('PULL preserves an un-pushed local edit (the interrupt-promotion bug)', () => {
    // User promoted a queued prompt to interrupt; not yet pushed → dirty.
    const local = make({ interrupt: true, syncedToBackend: false })
    // Backend still has the pre-promotion value.
    const backend = make({ interrupt: false, syncedToBackend: true, status: 'pending' })

    const r = reconcileEntry(local, backend, /* pushed */ false)

    // Must stay dirty so syncToBackend actually pushes interrupt=true.
    expect(r.syncedToBackend).toBe(false)
    // The local edit must survive.
    expect(r.interrupt).toBe(true)
  })

  it('PULL confirms a clean entry as synced', () => {
    expect(reconcileEntry(make({ syncedToBackend: true }), make(), false).syncedToBackend).toBe(true)
    expect(reconcileEntry(make({ syncedToBackend: undefined }), make(), false).syncedToBackend).toBe(true)
  })

  it('PUSH clears the dirty flag (the entry was just acknowledged)', () => {
    const r = reconcileEntry(make({ interrupt: true, syncedToBackend: false }), make(), /* pushed */ true)
    expect(r.syncedToBackend).toBe(true)
  })

  it('always reflects backend-owned status/retry/error, even on a dirty entry', () => {
    const local = make({ status: 'pending', retryCount: 0, syncedToBackend: false })
    const backend = make({ status: 'failed', retryCount: 2, errorMessage: 'bounced', nextRetryAt: 99 })

    const r = reconcileEntry(local, backend, false)

    expect(r.status).toBe('failed')
    expect(r.retryCount).toBe(2)
    expect(r.errorMessage).toBe('bounced')
    expect(r.nextRetryAt).toBe(99)
    // Backend status reflected, but the pending local edit is STILL preserved.
    expect(r.syncedToBackend).toBe(false)
  })

  it('never lets the backend overwrite a frontend-owned field (interrupt/content)', () => {
    const local = make({ interrupt: true, content: 'local', syncedToBackend: false })
    const backend = make({ interrupt: false, content: 'stale-backend' })

    const r = reconcileEntry(local, backend, false)

    expect(r.interrupt).toBe(true)
    expect(r.content).toBe('local')
  })
})

describe('appendPromptDraft', () => {
  beforeEach(() => localStorage.clear())

  it('seeds an empty session draft', () => {
    appendPromptDraft('ses-new', 'I would like to create a new bot')

    expect(JSON.parse(localStorage.getItem('helix_prompt_draft_ses-new') ?? '{}')).toEqual({
      content: 'I would like to create a new bot',
      sessionId: 'ses-new',
    })
  })

  it('preserves and appends to an existing draft', () => {
    localStorage.setItem('helix_prompt_draft_ses-existing', JSON.stringify({
      content: 'Existing thought',
      sessionId: 'ses-existing',
    }))

    appendPromptDraft('ses-existing', 'I would like to create a new bot')

    expect(JSON.parse(localStorage.getItem('helix_prompt_draft_ses-existing') ?? '{}').content)
      .toBe('Existing thought I would like to create a new bot')
  })
})
