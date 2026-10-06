import { describe, expect, it } from 'vitest'

import {
  formatClaudeAccountIdentity,
  getClaudeLoginExpiry,
  formatClaudeOrganizationRef,
  formatRateLimitTier,
} from './claudeSubscriptionUtils'

describe('formatRateLimitTier', () => {
  it('reduces Anthropic tier slugs to the multiplier', () => {
    // Verbatim from a live GET /api/oauth/profile on a Max account.
    expect(formatRateLimitTier('default_claude_max_20x')).toBe('20x')
    expect(formatRateLimitTier('default_claude_max_5x')).toBe('5x')
  })

  it('drops slugs that carry no multiplier rather than leaking jargon', () => {
    expect(formatRateLimitTier('default_claude_pro')).toBe('')
    expect(formatRateLimitTier('default_claude_team')).toBe('')
  })

  it('passes through user-reported tiers from the setup-token form', () => {
    expect(formatRateLimitTier('20x')).toBe('20x')
    expect(formatRateLimitTier('  5x  ')).toBe('5x')
  })

  it('returns empty for missing input', () => {
    expect(formatRateLimitTier(undefined)).toBe('')
    expect(formatRateLimitTier(null)).toBe('')
    expect(formatRateLimitTier('   ')).toBe('')
  })
})

describe('formatClaudeOrganizationRef', () => {
  it('shortens the org uuid to its first segment', () => {
    expect(formatClaudeOrganizationRef('f2f721d7-f975-426f-bb19-b0b45a3a9d52')).toBe(
      'Claude org f2f721d7',
    )
  })

  it('returns empty for missing input', () => {
    expect(formatClaudeOrganizationRef(undefined)).toBe('')
    expect(formatClaudeOrganizationRef('  ')).toBe('')
  })
})

describe('formatClaudeAccountIdentity', () => {
  it('renders account, plan and multiplier from real profile values', () => {
    expect(
      formatClaudeAccountIdentity({
        accountEmail: 'phil@winder.ai',
        plan: 'max',
        tier: 'default_claude_max_20x',
      }),
    ).toBe('phil@winder.ai · Max · 20x')
  })

  it('falls back to the connecting Helix user when the Claude account is unknown', () => {
    expect(
      formatClaudeAccountIdentity({
        fallbackName: 'luke@helix.ml',
        plan: 'pro',
        tier: 'default_claude_pro',
      }),
    ).toBe('luke@helix.ml · Pro')
  })

  it('prefers the Claude account email over the Helix fallback', () => {
    expect(
      formatClaudeAccountIdentity({
        accountEmail: 'phil@winder.ai',
        accountName: 'Phil',
        fallbackName: 'luke@helix.ml',
      }),
    ).toBe('phil@winder.ai')
  })

  it('names the verified Claude org when a setup token has no email', () => {
    expect(
      formatClaudeAccountIdentity({
        organizationId: 'f2f721d7-f975-426f-bb19-b0b45a3a9d52',
        fallbackName: 'luke@helix.ml',
      }),
    ).toBe('Claude org f2f721d7')
  })

  it('prefers a profiled email over the org reference', () => {
    expect(
      formatClaudeAccountIdentity({
        accountEmail: 'phil@winder.ai',
        organizationId: 'f2f721d7-f975-426f-bb19-b0b45a3a9d52',
      }),
    ).toBe('phil@winder.ai')
  })

  it('returns empty when nothing is known', () => {
    expect(formatClaudeAccountIdentity({})).toBe('')
  })
})

describe('getClaudeLoginExpiry', () => {
  const inHours = (h: number) => new Date(Date.now() + h * 3600_000).toISOString()

  it('says nothing when the login has more than a day left', () => {
    // The common case: warning here would be noise on every render.
    expect(getClaudeLoginExpiry(inHours(72))).toBeNull()
  })

  it('says nothing when no deadline is recorded', () => {
    // Setup tokens carry no refresh token, so there is no deadline to read.
    expect(getClaudeLoginExpiry(undefined)).toBeNull()
    expect(getClaudeLoginExpiry(null)).toBeNull()
    expect(getClaudeLoginExpiry('not-a-date')).toBeNull()
  })

  it('treats Go\'s zero time as no deadline, not a deadline in year 1', () => {
    // Verbatim from GET /api/v1/claude-subscriptions for a setup token: an
    // unset time.Time serialises as a real, parseable date. Reading it as a
    // deadline told a user with a working login "Expired 739850d ago".
    expect(getClaudeLoginExpiry('0001-01-01T00:00:00Z')).toBeNull()
  })

  it('warns once the login dies within a day', () => {
    // Not a whole number of hours: inHours(5) lands microseconds under 5h and
    // floors to 4, which made this assertion flake under load.
    const expiry = getClaudeLoginExpiry(inHours(5.5))
    expect(expiry).toMatchObject({ isExpired: false, isExpiringToday: true })
    expect(expiry?.label).toBe('Expires in 5h')
  })

  it('reports an already-dead login', () => {
    const expiry = getClaudeLoginExpiry(inHours(-2))
    expect(expiry).toMatchObject({ isExpired: true, isExpiringToday: false })
    expect(expiry?.label).toBe('Expired 2h ago')
  })

  it('never rounds a live login down to zero minutes', () => {
    expect(getClaudeLoginExpiry(new Date(Date.now() + 20_000).toISOString())?.label).toBe('Expires in 1m')
  })
})
