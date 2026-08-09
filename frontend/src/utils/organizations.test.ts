import { describe, expect, it } from 'vitest'

import { orgLandingRoute } from './organizations'

describe('orgLandingRoute', () => {
  it('lands in organization chat', () => {
    expect(orgLandingRoute()).toBe('org_chat')
  })
})
