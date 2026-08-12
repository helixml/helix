import { describe, expect, it } from 'vitest'

import { preferredSubscriptionRuntimeConfig } from './NewAgentDialog'
import { CLAUDE_SUBSCRIPTION_MODELS } from './CodingAgentForm'

describe('Claude subscription models', () => {
  it('uses explicit flagship model versions', () => {
    expect(CLAUDE_SUBSCRIPTION_MODELS).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'claude-opus-5', label: expect.stringContaining('Opus 5') }),
      expect.objectContaining({ id: 'claude-fable-5', label: expect.stringContaining('Fable 5') }),
      expect.objectContaining({ id: 'claude-opus-4-8', label: expect.stringContaining('Opus 4.8') }),
    ]))
  })
})

describe('preferredSubscriptionRuntimeConfig', () => {
  it('prefers Codex when both subscriptions are available', () => {
    expect(preferredSubscriptionRuntimeConfig(1, 1)).toMatchObject({
      runtime: 'codex_cli',
      credentials: 'subscription',
      provider: '',
      model: 'gpt-5.6-sol',
    })
  })

  it('selects Claude when it is the only subscription available', () => {
    expect(preferredSubscriptionRuntimeConfig(0, 1)).toMatchObject({
      runtime: 'claude_code',
      credentials: 'subscription',
      provider: '',
      model: 'claude-opus-5',
    })
  })

  it('keeps the default runtime when no subscription is available', () => {
    expect(preferredSubscriptionRuntimeConfig(0, 0)).toBeUndefined()
  })
})
