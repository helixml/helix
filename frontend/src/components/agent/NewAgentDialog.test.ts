import { describe, expect, it } from 'vitest'

import { preferredSubscriptionRuntimeConfig } from './NewAgentDialog'

const catalogue = { defaultClaudeModel: 'claude-opus-5-5', defaultCodexModel: 'gpt-6-sol' }

describe('preferredSubscriptionRuntimeConfig', () => {
  it('prefers Codex when both subscriptions are available', () => {
    expect(preferredSubscriptionRuntimeConfig(1, 1, catalogue)).toMatchObject({
      runtime: 'codex_cli',
      credentials: 'subscription',
      provider: '',
      model: 'gpt-6-sol',
    })
  })

  it('selects Claude when it is the only subscription available', () => {
    expect(preferredSubscriptionRuntimeConfig(0, 1, catalogue)).toMatchObject({
      runtime: 'claude_code',
      credentials: 'subscription',
      provider: '',
      model: 'claude-opus-5-5',
    })
  })

  it('keeps the default runtime when no subscription is available', () => {
    expect(preferredSubscriptionRuntimeConfig(0, 0, catalogue)).toBeUndefined()
  })
})
