import { describe, expect, it } from 'vitest'

import type { TypesOrganizationMembership, TypesUser } from '../../api/api'
import { resolveOrganizationUser } from './OrganizationUserAvatar'

describe('resolveOrganizationUser', () => {
  const currentUser: TypesUser = { id: 'current', full_name: 'Current User' }
  const members: TypesOrganizationMembership[] = [
    { user_id: 'other', user: { id: 'other', full_name: 'Other User' } },
  ]

  it('uses the current account even before its membership has loaded', () => {
    expect(resolveOrganizationUser('current', members, currentUser)).toBe(currentUser)
  })

  it('resolves another organization member and leaves unknown users unresolved', () => {
    expect(resolveOrganizationUser('other', members, currentUser)?.full_name).toBe('Other User')
    expect(resolveOrganizationUser('missing', members, currentUser)).toBeUndefined()
    expect(resolveOrganizationUser(undefined, members, currentUser)).toBeUndefined()
  })
})
