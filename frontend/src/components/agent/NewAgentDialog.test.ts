import { describe, expect, it } from 'vitest'

import { preferredSubscriptionRuntimeConfig } from './NewAgentDialog'

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
      model: 'opus[1m]',
    })
  })

  it('keeps the default runtime when no subscription is available', () => {
    expect(preferredSubscriptionRuntimeConfig(0, 0)).toBeUndefined()
  })
})
