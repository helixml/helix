import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  TypesCodeAgentCredentialType,
  TypesCodeAgentRuntime,
  TypesProject,
} from '../api/api'
import { useSeedProjectCodeAgentConfig } from './useSeedProjectCodeAgentConfig'

const mutateAsync = vi.hoisted(() => vi.fn())
const updateProject = vi.hoisted(() => vi.fn())

vi.mock('../services/projectService', () => ({
  useUpdateProject: (projectId: string) => {
    updateProject(projectId)
    return { mutateAsync }
  },
}))

const PICK = {
  runtime: TypesCodeAgentRuntime.CodeAgentRuntimeClaudeCode,
  credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription,
  model: 'claude-opus-5',
}

const PROJECT: TypesProject = { id: 'prj_1', name: 'demo' }

describe('useSeedProjectCodeAgentConfig', () => {
  beforeEach(() => {
    mutateAsync.mockReset()
    mutateAsync.mockResolvedValue({})
    updateProject.mockClear()
  })

  it('remembers the first explicit pick as the project default', () => {
    const { result } = renderHook(() => useSeedProjectCodeAgentConfig(PROJECT))

    result.current(PICK, 'user')

    expect(updateProject).toHaveBeenCalledWith('prj_1')
    expect(mutateAsync).toHaveBeenCalledWith({ code_agent_config: PICK })
  })

  it('ignores the picker choosing a default on its own', () => {
    const { result } = renderHook(() => useSeedProjectCodeAgentConfig(PROJECT))

    result.current(PICK, 'auto')

    expect(mutateAsync).not.toHaveBeenCalled()
  })

  it('leaves an existing project default alone', () => {
    const configured: TypesProject = {
      ...PROJECT,
      code_agent_config: {
        runtime: TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI,
        credential_type: TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey,
        provider_ref: 'openai',
        model: 'gpt-5.6-sol',
      },
    }
    const { result } = renderHook(() => useSeedProjectCodeAgentConfig(configured))

    result.current(PICK, 'user')

    expect(mutateAsync).not.toHaveBeenCalled()
  })

  it('writes once while the project query is still reporting the old config', () => {
    const { result } = renderHook(() => useSeedProjectCodeAgentConfig(PROJECT))

    result.current(PICK, 'user')
    result.current({ ...PICK, model: 'claude-fable-5' }, 'user')

    expect(mutateAsync).toHaveBeenCalledTimes(1)
  })

  it('does nothing without a project', () => {
    const { result } = renderHook(() => useSeedProjectCodeAgentConfig(undefined))

    result.current(PICK, 'user')

    expect(mutateAsync).not.toHaveBeenCalled()
  })
})
