import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, fireEvent, waitFor } from '@testing-library/react'
import { PromptHistoryEntry } from '../../hooks/usePromptHistory'
import RobustPromptInput from './RobustPromptInput'

const updateInterrupt = vi.fn()
const saveToHistory = vi.fn()
const clearDraft = vi.fn()

let pendingPrompts: PromptHistoryEntry[] = []

vi.mock('../../hooks/usePromptHistory', async () => {
  const actual = await vi.importActual<typeof import('../../hooks/usePromptHistory')>(
    '../../hooks/usePromptHistory'
  )
  return {
    ...actual,
    usePromptHistory: () => ({
      draft: '',
      setDraft: vi.fn(),
      history: [],
      historyIndex: -1,
      navigateUp: () => false,
      navigateDown: () => false,
      saveToHistory,
      markAsSent: vi.fn(),
      markAsFailed: vi.fn(),
      updateContent: vi.fn(),
      updateInterrupt,
      removeFromQueue: vi.fn(),
      reorderQueue: vi.fn(),
      pendingPrompts,
      failedPrompts: [],
      clearDraft,
      pinPrompt: vi.fn(),
    }),
  }
})

const mkEntry = (id: string, ts: number, overrides: Partial<PromptHistoryEntry> = {}): PromptHistoryEntry => ({
  id,
  content: `msg ${id}`,
  timestamp: ts,
  status: 'pending',
  interrupt: false,
  ...overrides,
})

describe('RobustPromptInput empty-Enter promotes most-recent queued to interrupt', () => {
  beforeEach(() => {
    updateInterrupt.mockClear()
    saveToHistory.mockClear()
    clearDraft.mockClear()
    pendingPrompts = []
  })

  const renderInput = () =>
    render(
      <RobustPromptInput
        sessionId="ses_test"
        onSend={vi.fn()}
      />
    )

  const pressEnter = (container: HTMLElement) => {
    const textarea = container.querySelector('textarea[data-prompt-input], textarea')
    expect(textarea).toBeTruthy()
    fireEvent.keyDown(textarea!, { key: 'Enter', shiftKey: false })
  }

  it('flips the highest-timestamp pending non-interrupt entry to interrupt', () => {
    pendingPrompts = [
      mkEntry('a', 1000),
      mkEntry('b', 3000),
      mkEntry('c', 2000),
    ]
    const { container } = renderInput()
    pressEnter(container)
    expect(updateInterrupt).toHaveBeenCalledTimes(1)
    expect(updateInterrupt).toHaveBeenCalledWith('b', true)
    expect(saveToHistory).not.toHaveBeenCalled()
  })

  it('skips entries already in interrupt mode', () => {
    pendingPrompts = [
      mkEntry('a', 5000, { interrupt: true }),
      mkEntry('b', 1000),
    ]
    const { container } = renderInput()
    pressEnter(container)
    expect(updateInterrupt).toHaveBeenCalledWith('b', true)
  })

  it('skips deleted (tombstoned) entries', () => {
    pendingPrompts = [
      mkEntry('a', 5000, { deleted: true }),
      mkEntry('b', 1000),
    ]
    const { container } = renderInput()
    pressEnter(container)
    expect(updateInterrupt).toHaveBeenCalledWith('b', true)
  })

  it('is a silent no-op when there are no eligible entries', () => {
    pendingPrompts = [mkEntry('a', 1000, { interrupt: true })]
    const { container } = renderInput()
    pressEnter(container)
    expect(updateInterrupt).not.toHaveBeenCalled()
    expect(saveToHistory).not.toHaveBeenCalled()
  })

  it('is a silent no-op when the queue is empty', () => {
    pendingPrompts = []
    const { container } = renderInput()
    pressEnter(container)
    expect(updateInterrupt).not.toHaveBeenCalled()
    expect(saveToHistory).not.toHaveBeenCalled()
  })

})

// Regression for 53b336e01: the client-side queue pump was deleted, so any
// composer without a spec_task_id (project Human Desktop, org-worker chat)
// silently parked messages as "saved locally" and never dispatched them. The
// pump must run when backend queue processing is NOT enabled, and must stay
// out of the way when it is (spec tasks).
describe('RobustPromptInput client-side queue pump', () => {
  beforeEach(() => {
    saveToHistory.mockClear()
    pendingPrompts = []
  })

  it('dispatches a pending message via onSend when there is no spec task', async () => {
    pendingPrompts = [mkEntry('a', 1000, { content: 'hello worker' })]
    const onSend = vi.fn().mockResolvedValue(undefined)
    render(
      <RobustPromptInput sessionId="ses_test" projectId="prj_1" onSend={onSend} />
    )
    await waitFor(
      () => expect(onSend).toHaveBeenCalledWith('hello worker', false),
      { timeout: 2000 },
    )
  })

  it('does NOT pump the queue when the backend queue is enabled (spec task)', async () => {
    pendingPrompts = [mkEntry('a', 1000)]
    const onSend = vi.fn().mockResolvedValue(undefined)
    render(
      <RobustPromptInput
        sessionId="ses_test"
        specTaskId="task_1"
        projectId="prj_1"
        apiClient={{} as any}
        onSend={onSend}
      />
    )
    // Give the pump's 500ms/300ms timers room to (not) fire.
    await new Promise((r) => setTimeout(r, 800))
    expect(onSend).not.toHaveBeenCalled()
  })
})
