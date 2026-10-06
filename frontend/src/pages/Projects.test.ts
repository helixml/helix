import { describe, expect, it } from 'vitest'

import { parseCreateProjectConfig } from './Projects'
import { parseOrgDefaultRuntime } from './newChatLogic'

describe('project creation handoff', () => {
  it('accepts a complete execution config unchanged', () => {
    const config = {
      runtime: 'claude_code',
      credential_type: 'subscription',
      model: 'claude-fable-5',
    }

    expect(parseCreateProjectConfig(JSON.stringify(config))).toEqual(config)
  })

  it('rejects malformed and incomplete execution configs', () => {
    expect(parseCreateProjectConfig('{')).toBeUndefined()
    expect(parseCreateProjectConfig(JSON.stringify({ runtime: 'zed_agent' }))).toBeUndefined()
  })

  it('materializes the organization default runtime for project creation', () => {
    expect(parseOrgDefaultRuntime(JSON.stringify({
      code_agent_runtime: 'zed_agent',
      code_agent_credential_type: 'api_key',
      provider: 'pe_helix',
      model: 'helix-model',
      reasoning_effort: 'high',
    }))).toEqual({
      runtime: 'zed_agent',
      credential_type: 'api_key',
      provider_ref: 'pe_helix',
      model: 'helix-model',
      reasoning_effort: 'high',
    })
  })
})
