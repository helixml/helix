import { describe, expect, it } from 'vitest'
import {
  isNavigationRouteActive,
  isOrgProjectSettingsRoute,
} from './UserOrgSelector.logic'

describe('UserOrgSelector navigation state', () => {
  it('keeps Chat active for direct and spec-task conversations', () => {
    const chatRoutes = ['chat', 'session']

    expect(isNavigationRouteActive('org_chat', chatRoutes)).toBe(true)
    expect(isNavigationRouteActive('org_chat-task', chatRoutes)).toBe(true)
    expect(isNavigationRouteActive('org_session', chatRoutes)).toBe(true)
  })

  it('does not treat a project session as top-level Chat', () => {
    expect(isNavigationRouteActive('org_project-session', ['chat', 'session'])).toBe(false)
  })

  it.each(['repositories', 'guidelines'])(
    'treats the %s tab as organization settings',
    (tab) => {
      expect(isOrgProjectSettingsRoute('org_projects', tab)).toBe(true)
    },
  )

  it('keeps the projects tab in top-level Projects', () => {
    expect(isOrgProjectSettingsRoute('org_projects', 'projects')).toBe(false)
  })

  it('does not affect other routes with the same tab parameter', () => {
    expect(isOrgProjectSettingsRoute('projects', 'repositories')).toBe(false)
  })
})
