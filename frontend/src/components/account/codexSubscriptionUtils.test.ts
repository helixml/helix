import { describe, expect, it } from 'vitest'

import {
  formatCodexAccountIdentity,
  formatCodexAccountRef,
  formatCodexPlan,
} from './codexSubscriptionUtils'

describe('formatCodexPlan', () => {
  it('labels the run-together slugs OpenAI actually returns', () => {
    // "prolite" is verbatim from a verified id_token on this stack.
    expect(formatCodexPlan('prolite')).toBe('Pro Lite')
    expect(formatCodexPlan('pro')).toBe('Pro')
    expect(formatCodexPlan('plus')).toBe('Plus')
  })

  it('capitalises an unknown plan rather than dropping it', () => {
    expect(formatCodexPlan('startup')).toBe('Startup')
  })

  it('returns empty for missing input', () => {
    expect(formatCodexPlan(undefined)).toBe('')
    expect(formatCodexPlan('  ')).toBe('')
  })
})

describe('formatCodexAccountRef', () => {
  it('shortens the account id to its first segment', () => {
    expect(formatCodexAccountRef('d7a72b21-87b3-4249-a897-15662e3505d9')).toBe(
      'ChatGPT account d7a72b21',
    )
  })

  it('returns empty for missing input', () => {
    expect(formatCodexAccountRef(null)).toBe('')
  })
})

describe('formatCodexAccountIdentity', () => {
  it('renders the verified account and plan', () => {
    expect(
      formatCodexAccountIdentity({
        accountEmail: 'karolis.rusenas@gmail.com',
        plan: 'prolite',
      }),
    ).toBe('karolis.rusenas@gmail.com · Pro Lite')
  })

  it('names the account id when the id_token could not be verified', () => {
    expect(
      formatCodexAccountIdentity({
        accountId: 'd7a72b21-87b3-4249-a897-15662e3505d9',
        fallbackName: 'My Codex Subscription',
      }),
    ).toBe('ChatGPT account d7a72b21')
  })

  it('falls back to the subscription label when nothing is verified', () => {
    expect(formatCodexAccountIdentity({ fallbackName: 'My Codex Subscription' })).toBe(
      'My Codex Subscription',
    )
  })

  it('returns empty when nothing is known', () => {
    expect(formatCodexAccountIdentity({})).toBe('')
  })
})
