import { describe, it, expect, vi } from 'vitest'

import {
  fetchExistingWorkerSession,
  ensureRunningWorkerSession,
} from './workerChatSession'

// These two helpers split the worker chat-session resolution into the
// side-effect-free read (used to auto-show the inline transcript on the
// worker detail page) and the infra-spinning ensure (used by "Open Human
// Desktop"). Keeping them pure + injected makes the ensure→create→resume
// branching unit-testable without an axios client.

describe('fetchExistingWorkerSession', () => {
  it('returns the existing session id without creating anything', async () => {
    const getExploratorySession = vi.fn().mockResolvedValue({ id: 'ses_existing' })
    const id = await fetchExistingWorkerSession('prj_1', { getExploratorySession })
    expect(id).toBe('ses_existing')
    expect(getExploratorySession).toHaveBeenCalledWith('prj_1')
  })

  it('returns null when no session exists yet', async () => {
    const getExploratorySession = vi.fn().mockResolvedValue(null)
    expect(
      await fetchExistingWorkerSession('prj_1', { getExploratorySession }),
    ).toBeNull()
  })

  it('returns null when the session has no id', async () => {
    const getExploratorySession = vi.fn().mockResolvedValue({ config: {} })
    expect(
      await fetchExistingWorkerSession('prj_1', { getExploratorySession }),
    ).toBeNull()
  })
})

describe('ensureRunningWorkerSession', () => {
  it('returns a running session id without creating or resuming', async () => {
    const api = {
      getExploratorySession: vi
        .fn()
        .mockResolvedValue({ id: 'ses_run', config: { external_agent_status: 'running' } }),
      createExploratorySession: vi.fn(),
      resumeSession: vi.fn(),
    }
    const id = await ensureRunningWorkerSession('prj_1', api)
    expect(id).toBe('ses_run')
    expect(api.createExploratorySession).not.toHaveBeenCalled()
    expect(api.resumeSession).not.toHaveBeenCalled()
  })

  it('creates a session when none exists', async () => {
    const api = {
      getExploratorySession: vi.fn().mockResolvedValue(null),
      createExploratorySession: vi.fn().mockResolvedValue({ id: 'ses_new' }),
      resumeSession: vi.fn(),
    }
    const id = await ensureRunningWorkerSession('prj_1', api)
    expect(id).toBe('ses_new')
    expect(api.createExploratorySession).toHaveBeenCalledWith('prj_1')
    expect(api.resumeSession).not.toHaveBeenCalled()
  })

  it('resumes a stopped (paused) session before returning it', async () => {
    const api = {
      getExploratorySession: vi
        .fn()
        .mockResolvedValue({ id: 'ses_paused', config: { external_agent_status: 'stopped' } }),
      createExploratorySession: vi.fn(),
      resumeSession: vi.fn().mockResolvedValue(undefined),
    }
    const id = await ensureRunningWorkerSession('prj_1', api)
    expect(id).toBe('ses_paused')
    expect(api.resumeSession).toHaveBeenCalledWith('ses_paused')
    expect(api.createExploratorySession).not.toHaveBeenCalled()
  })

  it('throws when create yields no session id', async () => {
    const api = {
      getExploratorySession: vi.fn().mockResolvedValue(null),
      createExploratorySession: vi.fn().mockResolvedValue({}),
      resumeSession: vi.fn(),
    }
    await expect(ensureRunningWorkerSession('prj_1', api)).rejects.toThrow()
  })
})
