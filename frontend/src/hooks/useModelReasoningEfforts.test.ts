import { describe, expect, it } from 'vitest'

import { TypesCodeAgentRuntime, TypesReasoningEffortProfile } from '../api/api'
import { effortsForRuntime } from './useModelReasoningEfforts'

const gpt6Sol = {
  family: 'gpt-6-sol',
  supported: ['none', 'low', 'medium', 'high', 'xhigh'],
  responses_only: ['max'],
} as TypesReasoningEffortProfile

describe('effortsForRuntime', () => {
  it('offers responses-only values to Codex, which calls /v1/responses', () => {
    expect(effortsForRuntime(gpt6Sol, TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI))
      .toEqual(['none', 'low', 'medium', 'high', 'xhigh', 'max'])
  })

  it('withholds responses-only values from chat-completions harnesses', () => {
    for (const runtime of [TypesCodeAgentRuntime.CodeAgentRuntimeQwenCode, TypesCodeAgentRuntime.CodeAgentRuntimeZedAgent]) {
      expect(effortsForRuntime(gpt6Sol, runtime)).toEqual(['none', 'low', 'medium', 'high', 'xhigh'])
    }
  })

  it('reports unknown when the model has no profile', () => {
    expect(effortsForRuntime(undefined, TypesCodeAgentRuntime.CodeAgentRuntimeCodexCLI)).toBeUndefined()
  })
})
