import { describe, expect, it } from 'vitest'

import { TypesOrganization } from '../api/api'
import {
  firstAccessibleOrgSlug,
  isEmbedRouteName,
  isOrgAccessDeniedError,
  orgLandingRoute,
  resolveOrgAccess,
} from './organizations'

const org = (name: string, extra: Partial<TypesOrganization> = {}): TypesOrganization => ({
  id: `org_${name}`,
  name,
  member: true,
  ...extra,
})

describe('orgLandingRoute', () => {
  it('lands in organization chat', () => {
    expect(orgLandingRoute()).toBe('org_chat')
  })
})

describe('resolveOrgAccess', () => {
  it('is pending until the org list has loaded', () => {
    // The list starts empty. Acting on it before it loads would evict every
    // user from their org on every page load.
    expect(
      resolveOrgAccess({ orgID: 'acme', organizations: [], orgListLoaded: false })
    ).toEqual({ status: 'pending' })
  })

  // The hook's `initialized` flag is set in a `finally`, so it is true even
  // when GET /organizations failed — leaving an empty list that means "we
  // don't know", not "you have no orgs". Acting on that would boot a user out
  // of an org they can fully access whenever the list request blips.
  it('stays pending when the list request failed, even though we tried', () => {
    expect(
      resolveOrgAccess({ orgID: 'acme', organizations: [], orgListLoaded: false })
    ).toEqual({ status: 'pending' })
  })

  it('is ok when there is no org in the URL', () => {
    expect(
      resolveOrgAccess({ orgID: '', organizations: [], orgListLoaded: true })
    ).toEqual({ status: 'ok' })
  })

  it('is ok when the URL org matches by slug', () => {
    expect(
      resolveOrgAccess({ orgID: 'acme', organizations: [org('acme')], orgListLoaded: true })
    ).toEqual({ status: 'ok' })
  })

  it('is ok when the URL org matches by id', () => {
    expect(
      resolveOrgAccess({ orgID: 'org_acme', organizations: [org('acme')], orgListLoaded: true })
    ).toEqual({ status: 'ok' })
  })

  // The regression this whole change exists for: a `selected_org` written by a
  // previously signed-in user on the same browser pointed at an org the current
  // user has no membership in, and nothing moved them off it.
  it('reports an org the user cannot see as inaccessible, with a fallback', () => {
    expect(
      resolveOrgAccess({
        orgID: 'unmanned-org',
        organizations: [org('mr-tester-org1')],
        orgListLoaded: true,
      })
    ).toEqual({ status: 'inaccessible', fallbackOrgSlug: 'mr-tester-org1' })
  })

  it('reports inaccessible with no fallback when the user has no orgs at all', () => {
    expect(
      resolveOrgAccess({ orgID: 'unmanned-org', organizations: [], orgListLoaded: true })
    ).toEqual({ status: 'inaccessible', fallbackOrgSlug: undefined })
  })

  // Admins are listed every org, with member:false on the ones they don't
  // belong to, and the API authorizes them as members — so being in the list is
  // enough. Treating member:false as inaccessible would boot admins out of
  // orgs they are legitimately administering.
  it('treats an admin-visible non-member org as accessible', () => {
    expect(
      resolveOrgAccess({
        orgID: 'unmanned-org',
        organizations: [org('unmanned-org', { member: false }), org('mine')],
        orgListLoaded: true,
      })
    ).toEqual({ status: 'ok' })
  })

  it('never evicts from an embed route', () => {
    // A scoped embed key sees an empty org list by design.
    expect(
      resolveOrgAccess({
        orgID: 'unmanned-org',
        organizations: [],
        orgListLoaded: true,
        routeName: 'embed_task',
      })
    ).toEqual({ status: 'ok' })
  })
})

describe('firstAccessibleOrgSlug', () => {
  it('prefers an org the user is actually a member of', () => {
    expect(
      firstAccessibleOrgSlug([org('not-mine', { member: false }), org('mine')])
    ).toBe('mine')
  })

  it('returns undefined when nothing is accessible', () => {
    expect(firstAccessibleOrgSlug([org('not-mine', { member: false })])).toBeUndefined()
    expect(firstAccessibleOrgSlug([])).toBeUndefined()
  })
})

describe('isOrgAccessDeniedError', () => {
  it.each([403, 404])('treats %i as a permanent access answer', (status) => {
    expect(isOrgAccessDeniedError({ response: { status } })).toBe(true)
  })

  // Transient failures must not evict the user — they should be able to retry
  // or wait for auth to recover.
  it.each([401, 429, 500, 502])('treats %i as transient', (status) => {
    expect(isOrgAccessDeniedError({ response: { status } })).toBe(false)
  })

  it('treats a network error with no response as transient', () => {
    expect(isOrgAccessDeniedError(new Error('Network Error'))).toBe(false)
    expect(isOrgAccessDeniedError(undefined)).toBe(false)
  })
})

describe('isEmbedRouteName', () => {
  it('matches embed routes only', () => {
    expect(isEmbedRouteName('embed_task')).toBe(true)
    expect(isEmbedRouteName('org_chat')).toBe(false)
    expect(isEmbedRouteName(undefined)).toBe(false)
  })
})

describe('firstAccessibleOrgSlug exclusions', () => {
  // Recovery from a 403 must not land back on the org that just refused us,
  // and with two failing orgs must not ping-pong between them.
  it('skips every excluded org, by id or by slug', () => {
    const orgs = [org('a'), org('b'), org('c')]
    expect(firstAccessibleOrgSlug(orgs, ['a'])).toBe('b')
    expect(firstAccessibleOrgSlug(orgs, ['org_a', 'b'])).toBe('c')
    expect(firstAccessibleOrgSlug(orgs, ['a', 'b', 'c'])).toBeUndefined()
  })
})
