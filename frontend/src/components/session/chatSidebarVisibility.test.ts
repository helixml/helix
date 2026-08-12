import { describe, expect, it } from 'vitest'

import {
  chatSidebarCollapsedStorageKey,
  parseChatSidebarCollapsed,
} from './chatSidebarVisibility'

describe('chat sidebar visibility storage', () => {
  it('scopes the stored state to the organization', () => {
    expect(chatSidebarCollapsedStorageKey('unmanned-org'))
      .toBe('helix:project-chat-sidebar:hidden:unmanned-org')
  })

  it('only treats an explicit true value as collapsed', () => {
    expect(parseChatSidebarCollapsed('true')).toBe(true)
    expect(parseChatSidebarCollapsed('false')).toBe(false)
    expect(parseChatSidebarCollapsed(null)).toBe(false)
  })
})
