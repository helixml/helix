import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'

const mockSync = vi.fn()
const mockList = vi.fn()

vi.mock('../services/promptHistoryService', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/promptHistoryService')>()),
  syncPromptHistory: (...args: unknown[]) => mockSync(...args),
  listPromptHistory: (...args: unknown[]) => mockList(...args),
}))

vi.mock('./useAccount', () => ({
  default: () => ({ user: { id: 'usr_1' } }),
}))

import { usePromptHistory } from './usePromptHistory'

const SESSION = 'ses_1'
const TASK = 'spt_1'
const PROJECT = 'prj_1'

function seedQueuedPrompt() {
  localStorage.setItem(`helix_prompt_history_${TASK}`, JSON.stringify([{
    id: 'p1',
    content: 'later please',
    timestamp: Date.now(),
    status: 'pending',
    sessionId: SESSION,
    specTaskId: TASK,
    interrupt: false,
    steer: false,
    syncedToBackend: true,
  }]))
}

describe('usePromptHistory.updateDeliveryMode', () => {
  beforeEach(() => {
    localStorage.clear()
    mockSync.mockReset().mockResolvedValue({ entries: [] })
    mockList.mockReset().mockResolvedValue({ entries: [] })
  })

  it.each([
    ['steer', { steer: true, interrupt: false }],
    ['interrupt', { steer: false, interrupt: true }],
    ['queue', { steer: false, interrupt: false }],
  ] as const)('pushes the %s flags at once, without waiting for the debounced sync', async (mode, flags) => {
    seedQueuedPrompt()
    const { result } = renderHook(() => usePromptHistory({
      sessionId: SESSION,
      specTaskId: TASK,
      projectId: PROJECT,
      apiClient: {} as never,
    }))
    await waitFor(() => expect(mockList).toHaveBeenCalled())
    await act(async () => {})
    mockSync.mockClear()

    act(() => result.current.updateDeliveryMode('p1', mode))

    // Synchronous: the debounced batch sync would only fire after a timer.
    expect(mockSync).toHaveBeenCalledTimes(1)
    const [, projectId, specTaskId, entries] = mockSync.mock.calls[0]
    expect(projectId).toBe(PROJECT)
    expect(specTaskId).toBe(TASK)
    expect(entries).toEqual([expect.objectContaining({ id: 'p1', ...flags })])

    const stored = JSON.parse(localStorage.getItem(`helix_prompt_history_${TASK}`) ?? '[]')
    expect(stored[0]).toEqual(expect.objectContaining(flags))
  })
})
