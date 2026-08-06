import { describe, expect, it } from 'vitest'
import {
  buildNewChatTaskRequest,
  chooseProjectChatAgentId,
  modelSupportsReasoningEffort,
  newChatHeading,
  projectChatAgentStorageKey,
  readNewChatReasoningEffort,
} from './newChatLogic'

describe('new chat project mode', () => {
  it('uses the normal-chat heading without project context', () => {
    expect(newChatHeading()).toBe('What would you like to know?')
  })

  it('uses the project name in task mode', () => {
    expect(newChatHeading('Payments')).toBe('What should we build in Payments?')
  })

  it('creates Plan tasks in backlog so attachments can upload before start', () => {
    expect(buildNewChatTaskRequest({
      appId: 'app_1',
      mode: 'plan',
      projectId: 'prj_1',
      prompt: 'Add billing',
    })).toEqual({
      app_id: 'app_1',
      auto_start: false,
      just_do_it_mode: false,
      priority: 'medium',
      project_id: 'prj_1',
      prompt: 'Add billing',
    })
  })

  it('marks Build tasks to skip planning', () => {
    expect(buildNewChatTaskRequest({
      mode: 'build',
      projectId: 'prj_1',
      prompt: 'Fix the tests',
    }).just_do_it_mode).toBe(true)
  })

  it('only exposes effort for a selected model that supports it', () => {
    const providers = [{
      id: 'provider-1',
      name: 'openai',
      available_models: [
        { id: 'gpt-basic', model_info: { supports_reasoning_effort: false } },
        { id: 'gpt-reasoning', model_info: { supports_reasoning_effort: true } },
      ],
    }]

    expect(modelSupportsReasoningEffort(providers, 'provider-1', 'gpt-reasoning')).toBe(true)
    expect(modelSupportsReasoningEffort(providers, 'openai', 'gpt-basic')).toBe(false)
    expect(modelSupportsReasoningEffort(providers, 'another-provider', 'gpt-reasoning')).toBe(false)
  })

  it('falls back to medium for an invalid stored effort', () => {
    expect(readNewChatReasoningEffort('high')).toBe('high')
    expect(readNewChatReasoningEffort('ultra')).toBe('medium')
  })

  it('keeps project agent preferences isolated by organization', () => {
    expect(projectChatAgentStorageKey('org_one')).toBe('helix_project_chat_agent:org_one')
    expect(projectChatAgentStorageKey('org_two')).toBe('helix_project_chat_agent:org_two')
  })

  it('restores an eligible remembered agent and rejects stale choices', () => {
    const availableIds = ['app_claude', 'app_codex']
    expect(chooseProjectChatAgentId(availableIds, 'app_codex')).toBe('app_codex')
    expect(chooseProjectChatAgentId(availableIds, 'app_org_worker')).toBe('app_claude')
    expect(chooseProjectChatAgentId([], 'app_codex')).toBe('')
  })
})
