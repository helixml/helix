import { describe, expect, it } from 'vitest'
import {
  chatRailAction,
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

describe('chatRailAction', () => {
  it('navigates and closes the drawer on a wide screen, as every rail entry does', () => {
    expect(chatRailAction(false, false)).toEqual({ navigate: true, keepDrawerOpen: false })
    expect(chatRailAction(false, true)).toEqual({ navigate: true, keepDrawerOpen: false })
  })

  it('keeps the list open on a phone — the drawer is the destination', () => {
    expect(chatRailAction(true, false).keepDrawerOpen).toBe(true)
    expect(chatRailAction(true, true).keepDrawerOpen).toBe(true)
  })

  it('brings a phone to the chat section when it is somewhere else', () => {
    expect(chatRailAction(true, false).navigate).toBe(true)
  })

  it('leaves a thread the phone is already reading alone', () => {
    expect(chatRailAction(true, true).navigate).toBe(false)
  })
})
