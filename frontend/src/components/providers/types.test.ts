import { describe, expect, it } from 'vitest'

import { PROVIDERS } from './types'

describe('provider presets', () => {
  it('creates Anthropic endpoints with the canonical provider name', () => {
    expect(PROVIDERS.find((provider) => provider.id === 'user/anthropic')).toMatchObject({
      endpoint_name: 'anthropic',
    })
  })
})
