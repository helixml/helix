import { describe, expect, it } from 'vitest'

import { getUserInitials } from './user'

describe('getUserInitials', () => {
  it('uses the first and last name initials when a full name is available', () => {
    expect(getUserInitials({ full_name: 'Test User', email: 'test@helix.ml' })).toBe('TU')
  })

  it('uses email for resolved users without a profile name', () => {
    expect(getUserInitials({ email: 'test@helix.ml' })).toBe('T')
  })

  it('uses a question mark only when the user is unresolved', () => {
    expect(getUserInitials()).toBe('?')
  })
})
