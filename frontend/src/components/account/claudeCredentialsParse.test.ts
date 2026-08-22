import { describe, expect, it } from 'vitest'

import { parseClaudeCredentials } from './ClaudeSubscriptionConnect'

describe('parseClaudeCredentials', () => {
  it('accepts the whole ~/.claude/.credentials.json file', () => {
    // Field names verbatim from a real credentials file written by `claude login`.
    const parsed = parseClaudeCredentials(
      JSON.stringify({
        claudeAiOauth: {
          accessToken: 'sk-ant-oat01-aaa',
          refreshToken: 'sk-ant-ort01-bbb',
          expiresAt: 1787174321857,
          scopes: ['user:profile', 'user:inference'],
          subscriptionType: 'max',
          rateLimitTier: 'default_claude_max_20x',
        },
      }),
    )
    expect(parsed.accessToken).toBe('sk-ant-oat01-aaa')
    expect(parsed.refreshToken).toBe('sk-ant-ort01-bbb')
    expect(parsed.subscriptionType).toBe('max')
  })

  it('accepts just the claudeAiOauth object, since people copy either one', () => {
    const parsed = parseClaudeCredentials(
      JSON.stringify({ accessToken: 'sk-ant-oat01-aaa', refreshToken: 'sk-ant-ort01-bbb' }),
    )
    expect(parsed.accessToken).toBe('sk-ant-oat01-aaa')
  })

  it('rejects a file missing the refresh token', () => {
    expect(() =>
      parseClaudeCredentials(JSON.stringify({ claudeAiOauth: { accessToken: 'sk-ant-oat01-aaa' } })),
    ).toThrow(/accessToken and refreshToken/)
  })

  it('rejects a pasted setup token, which is not JSON', () => {
    expect(() => parseClaudeCredentials('sk-ant-oat01-not-json')).toThrow(/not valid JSON/)
  })
})
